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
