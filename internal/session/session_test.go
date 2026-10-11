package session

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRingBufferKeepsTail(t *testing.T) {
	const max = 64
	r := &ringBuffer{max: max}

	// 灌入远超上限的数据。
	var all bytes.Buffer
	for i := 0; i < 100; i++ {
		chunk := []byte("0123456789")
		all.Write(chunk)
		r.write(chunk)
	}

	got := r.snapshot()
	if len(got) > max {
		t.Fatalf("缓冲长度 %d 超过上限 %d", len(got), max)
	}

	// 保留的必须是最近写入的那一段（后缀），不能是开头。
	if !bytes.HasSuffix(all.Bytes(), got) {
		t.Errorf("缓冲内容不是写入流的后缀\n保留: %q", got)
	}
	// 最新的字节一定要在。
	if !bytes.HasSuffix(got, []byte("0123456789")) {
		t.Errorf("最新的数据被丢掉了: %q", got)
	}
}

func TestRingBufferSnapshotIsCopy(t *testing.T) {
	r := &ringBuffer{max: 64}
	r.write([]byte("abc"))

	snap := r.snapshot()
	snap[0] = 'X'

	if got := string(r.snapshot()); got != "abc" {
		t.Errorf("snapshot 应该是拷贝，改动它不该影响缓冲，实际 %q", got)
	}
}

// runShell 起一个 sh 进程来验证进程管理，避免测试依赖真实的 BaiduPCS-Go。
func runShell(t *testing.T, script string, bufBytes int) *Session {
	t.Helper()

	sess, err := start(startOptions{
		ID:          "t_test",
		Name:        "test",
		RemotePath:  "/x",
		SaveDir:     "/tmp",
		CLIPath:     "/bin/sh",
		Args:        []string{"-c", script},
		BufferBytes: bufBytes,
	})
	if err != nil {
		t.Fatalf("启动进程失败: %v", err)
	}
	return sess
}

// drain 把通道里的内容收全，直到通道关闭。
func drain(ch <-chan []byte) []byte {
	var out bytes.Buffer
	for chunk := range ch {
		out.Write(chunk)
	}
	return out.Bytes()
}

func TestSessionCapturesOutput(t *testing.T) {
	sess := runShell(t, "printf 'hello\\n'; printf 'world\\n'", 4096)

	select {
	case <-sess.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("进程没有在预期时间内结束")
	}

	task := sess.Task()
	if task.Status != StatusExited {
		t.Errorf("状态应为 %q，实际 %q", StatusExited, task.Status)
	}
	if task.ExitCode != 0 {
		t.Errorf("退出码应为 0，实际 %d", task.ExitCode)
	}

	// 进程结束后再订阅：应当能拿到完整重放，且通道已经关闭。
	replay, ch, cancel := sess.Subscribe()
	defer cancel()

	out := string(replay)
	if !strings.Contains(out, "hello") || !strings.Contains(out, "world") {
		t.Errorf("重放内容不完整: %q", out)
	}
	if extra := drain(ch); len(extra) != 0 {
		t.Errorf("进程已结束，通道不该再有数据: %q", extra)
	}
}

func TestSessionSubscribeBeforeOutput(t *testing.T) {
	// 先订阅再产出，验证订阅者能实时收到。
	sess := runShell(t, "sleep 0.3; printf 'streamed'", 4096)

	replay, ch, cancel := sess.Subscribe()
	defer cancel()

	got := append([]byte{}, replay...)
	got = append(got, drain(ch)...)

	if !strings.Contains(string(got), "streamed") {
		t.Errorf("订阅者没收到实时输出: %q", got)
	}
}

func TestSessionKill(t *testing.T) {
	sess := runShell(t, "sleep 30", 4096)

	if got := sess.Task().Status; got != StatusRunning {
		t.Fatalf("刚启动时状态应为 %q，实际 %q", StatusRunning, got)
	}

	// Kill 会等进程真的退出，所以这里不该超时。
	done := make(chan struct{})
	go func() {
		_ = sess.Kill()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Kill 没有在预期时间内让进程退出")
	}

	select {
	case <-sess.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done 通道没有被关闭")
	}

	if got := sess.Task().Status; got != StatusKilled {
		t.Errorf("状态应为 %q，实际 %q", StatusKilled, got)
	}
}

func TestSessionSlowSubscriberIsDropped(t *testing.T) {
	// 让进程产出远超订阅者缓冲上限的数据，且订阅者完全不消费。
	sess := runShell(t, "for i in $(seq 1 2000); do echo 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'; done", 1<<20)

	_, ch, cancel := sess.Subscribe()
	defer cancel()

	select {
	case <-sess.Done():
	case <-time.After(20 * time.Second):
		t.Fatal("进程没有结束")
	}

	// 慢订阅者会被踢掉，通道被关闭。能走到这里（drain 能返回）就说明关闭了。
	deadline := time.After(5 * time.Second)
	finished := make(chan struct{})
	go func() {
		drain(ch)
		close(finished)
	}()

	select {
	case <-finished:
	case <-deadline:
		t.Fatal("慢订阅者的通道没有被关闭")
	}
}

func TestTaskSnapshotIsIndependent(t *testing.T) {
	sess := runShell(t, "sleep 5", 4096)
	defer func() { _ = sess.Kill() }()

	task := sess.Task()
	if len(task.Args) == 0 {
		t.Fatal("Args 不该为空")
	}
	task.Args[0] = "被改坏了"

	if sess.Task().Args[0] == "被改坏了" {
		t.Error("Task() 应该返回 Args 的拷贝，外部改动不该影响内部状态")
	}
}

// 进程输出里的进度行应当被解析进 Task.Progress。
func TestSessionParsesProgress(t *testing.T) {
	sess := runShell(t,
		"printf '[1] 准备下载: /我的资源/big.bin\\n'; "+
			"printf '[1] ↓ 1.00MB/2.00MB 100.00KB/s in 10s, left 10s ............\\r'; "+
			"printf '[1] 下载完成, 保存位置: /tmp/big.bin\\n'; "+
			"printf '下载结束, 时间: 10s, 数据总量: 2.00MB\\n'",
		4096)

	select {
	case <-sess.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("进程没有在预期时间内结束")
	}

	prog := sess.Task().Progress
	if prog == nil {
		t.Fatal("应该从输出里解析出进度")
	}
	if prog.CurrentName != "big.bin" {
		t.Errorf("当前文件名错误: %q", prog.CurrentName)
	}
	if prog.TotalBytes != 2<<20 {
		t.Errorf("总量应为 %d，实际 %d", 2<<20, prog.TotalBytes)
	}
	if !prog.Finished {
		t.Error("输出里有「下载结束」，应该标记为 Finished")
	}
	if prog.Percent != 100 {
		t.Errorf("看到「下载完成」后应为 100%%，实际 %v", prog.Percent)
	}
}

// 没有进度行的任务，Progress 应该保持 nil，前端据此不显示进度条。
func TestSessionWithoutProgressHasNilProgress(t *testing.T) {
	sess := runShell(t, "printf 'nothing to see here\\n'", 4096)

	select {
	case <-sess.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("进程没有在预期时间内结束")
	}

	if prog := sess.Task().Progress; prog != nil {
		t.Errorf("没有进度行时不该有进度，实际 %+v", prog)
	}
}

// 进度行会被采进速度曲线。
//
// 这里直接调 appendOutput 喂数据，而不是起一个 sh 进程打印——采样是「读到内容有变化
// 就采一次」，结果取决于输出到达时怎么分块。走真实进程的话分块由管道调度决定，
// 本地两条 printf 落成两次读、CI 上可能并成一次，测试就飘了（确实飘过一次）。
// 自己控制分块才能确定地断言。
func TestSessionRecordsSpeedHistory(t *testing.T) {
	sess := runShell(t, "sleep 5", 4096)
	defer func() { _ = sess.Kill() }()

	// 两条进度行分两次喂 → 两个点。
	sess.appendOutput([]byte("[1] ↓ 1.00MB/2.00MB 100.00KB/s in 1s, left 10s ............\r"))
	sess.appendOutput([]byte("[1] ↓ 2.00MB/2.00MB 200.00KB/s in 2s, left 5s ............\r"))

	times, speeds := sess.SpeedHistory()
	if len(times) != len(speeds) {
		t.Fatalf("时间与速度长度不一致: %d vs %d", len(times), len(speeds))
	}
	if len(speeds) != 2 {
		t.Fatalf("两条进度行应采出 2 个点，实际 %d 个", len(speeds))
	}
	if got, want := speeds[0], int64(100*1024); got != want {
		t.Errorf("第 1 个速度应为 %d，实际 %d", want, got)
	}
	if got, want := speeds[1], int64(200*1024); got != want {
		t.Errorf("第 2 个速度应为 %d，实际 %d", want, got)
	}
	for i := 1; i < len(times); i++ {
		if times[i] < times[i-1] {
			t.Errorf("时间戳没有递增: %v", times)
			break
		}
	}
}

// 一次读到多条进度行时只会采一个点：采样跟着「内容有变化」走，不是跟着行数走。
//
// 单文件下载的进度行是每秒一条、各自到达，所以实际不会触发；这里把这个已知行为
// 固定下来，免得以后有人以为它是 bug 又改回去。（真要在意，可以把采样挪进解析器、
// 每解析一条进度行采一次，但目前没有收益。）
func TestSessionCoalescedProgressSamplesOnce(t *testing.T) {
	sess := runShell(t, "sleep 5", 4096)
	defer func() { _ = sess.Kill() }()

	sess.appendOutput([]byte(
		"[1] ↓ 1.00MB/2.00MB 100.00KB/s in 1s, left 10s ............\r" +
			"[1] ↓ 2.00MB/2.00MB 200.00KB/s in 2s, left 5s ............\r"))

	_, speeds := sess.SpeedHistory()
	if len(speeds) != 1 {
		t.Fatalf("合并在一次读取里的多条进度行只应采 1 个点，实际 %d 个", len(speeds))
	}
	if got, want := speeds[0], int64(200*1024); got != want {
		t.Errorf("应采到最后一条的速度 %d，实际 %d", want, got)
	}
}

// snapshot 返回的必须是拷贝，调用方改它不该影响内部状态。
func TestSpeedHistorySnapshotIsCopy(t *testing.T) {
	sess := runShell(t, "sleep 5", 4096)
	defer func() { _ = sess.Kill() }()

	sess.mu.Lock()
	sess.speeds.append(1000, 111)
	sess.speeds.append(2000, 222)
	sess.mu.Unlock()

	times, speeds := sess.SpeedHistory()
	times[0] = 99999
	speeds[0] = 99999

	again, againSpeeds := sess.SpeedHistory()
	if again[0] == 99999 || againSpeeds[0] == 99999 {
		t.Error("SpeedHistory 应该返回拷贝，外部改动不该影响内部状态")
	}
}

// 采样环要有上限，长时间下载不能把内存养肥。
func TestSpeedHistoryIsBounded(t *testing.T) {
	var h speedHistory
	for i := 0; i < maxSpeedSamples*3; i++ {
		h.append(int64(i), int64(i))
	}

	times, speeds := h.snapshot()
	if len(times) != maxSpeedSamples || len(speeds) != maxSpeedSamples {
		t.Fatalf("应保留 %d 个点，实际 %d/%d", maxSpeedSamples, len(times), len(speeds))
	}
	// 保留的必须是最新的那一段。
	if got := times[len(times)-1]; got != int64(maxSpeedSamples*3-1) {
		t.Errorf("最后一条应该是最新的，实际 %d", got)
	}
}
