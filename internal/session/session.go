// Package session 负责把 BaiduPCS-Go 当成后台进程跑起来，并把它的原始输出
// 原封不动地广播给订阅者（浏览器里的 xterm）。
//
// 这里刻意不做任何输出解析：进度条的 \r 覆盖、ANSI 转义都交给终端模拟器处理，
// 后端只保证字节流不丢、不乱序、能被后来者从头重放。
package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/hungtcs/baidupcs-console/internal/pcs"
)

// Status 是任务的生命周期状态。
type Status string

const (
	// StatusRunning 表示进程正在运行。
	StatusRunning Status = "running"
	// StatusExited 表示进程已自然退出（注意 CLI 出错时退出码也是 0）。
	StatusExited Status = "exited"
	// StatusKilled 表示进程是被我们主动杀掉的。
	StatusKilled Status = "killed"
)

// Task 是暴露给 HTTP API 的任务描述，不含输出内容本身。
type Task struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	RemotePath  string     `json:"remotePath"`
	SaveDir     string     `json:"saveDir"`
	Args        []string   `json:"args"`
	CommandLine string     `json:"commandLine"`
	Status      Status     `json:"status"`
	ExitCode    int        `json:"exitCode"`
	PID         int        `json:"pid"`
	StartedAt   time.Time  `json:"startedAt"`
	FinishedAt  *time.Time `json:"finishedAt"`
}

// subBuffer 是单个订阅者的待发队列长度。慢到这个数还没被消费掉，
// 说明浏览器已经跟不上了，直接踢掉让它重连后重放。
const subBuffer = 512

// Session 是一个正在（或曾经）运行的 BaiduPCS-Go 进程。
type Session struct {
	task Task

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	mu     sync.Mutex
	buf    *ringBuffer
	subs   map[chan []byte]struct{}
	killed bool
	done   chan struct{}
	// onExit 在进程结束后、done 关闭前被调用，用来把最终状态落盘。
	// 调用它时锁已释放。
	onExit func(Task)
}

// ringBuffer 只保留最近 max 字节的输出，超出后丢弃最旧的部分。
type ringBuffer struct {
	max int
	buf []byte
}

func (r *ringBuffer) write(p []byte) {
	r.buf = append(r.buf, p...)
	if len(r.buf) <= r.max {
		return
	}
	// 一次多丢一些，避免每次写入都搬一次内存。留 3/4 是为了
	// 让重放时能保留尽量多的历史，同时又不必每次写都搬内存。
	keep := r.max * 3 / 4
	r.buf = append(r.buf[:0], r.buf[len(r.buf)-keep:]...)
}

func (r *ringBuffer) snapshot() []byte {
	out := make([]byte, len(r.buf))
	copy(out, r.buf)
	return out
}

// newID 生成任务 ID，形如 t_1a2b3c4d。
func newID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "t_" + hex.EncodeToString([]byte(time.Now().Format("150405.000")))
	}
	return "t_" + hex.EncodeToString(b[:])
}

// startOptions 是启动一个任务进程所需的全部输入。
type startOptions struct {
	ID         string
	Name       string
	RemotePath string
	SaveDir    string
	CLIPath    string
	Args       []string

	// BufferBytes 是输出环形缓冲的上限。
	BufferBytes int
	// OnExit 在进程结束后被调用，用于把最终状态落盘。可为 nil。
	OnExit func(Task)
}

// start 启动进程并开始收集输出。调用方需保证参数已校验。
func start(opts startOptions) (*Session, error) {
	cmd := exec.Command(opts.CLIPath, opts.Args...)
	// 独立进程组，便于连同子进程一起杀掉；父进程意外退出时子进程也一起走。
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	s := &Session{
		task: Task{
			ID:          opts.ID,
			Name:        opts.Name,
			RemotePath:  opts.RemotePath,
			SaveDir:     opts.SaveDir,
			Args:        opts.Args,
			CommandLine: pcs.CommandLine(opts.CLIPath, opts.Args),
			Status:      StatusRunning,
			StartedAt:   time.Now(),
		},
		cmd:    cmd,
		stdin:  stdin,
		buf:    &ringBuffer{max: opts.BufferBytes},
		subs:   make(map[chan []byte]struct{}),
		done:   make(chan struct{}),
		onExit: opts.OnExit,
	}

	// stdout 与 stderr 都喂给同一个写入器，顺序即进程真实的输出顺序。
	cmd.Stdout = writerFunc(s.appendOutput)
	cmd.Stderr = writerFunc(s.appendOutput)

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// PID 必须在锁下写：会话一旦返回给管理器，别的 goroutine 就可能通过
	// Task()/List() 并发读到 s.task。
	s.mu.Lock()
	s.task.PID = cmd.Process.Pid
	s.mu.Unlock()

	go s.wait()
	return s, nil
}

// wait 等待进程结束并落定状态。
func (s *Session) wait() {
	err := s.cmd.Wait()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
	}

	now := time.Now()

	s.mu.Lock()
	s.task.ExitCode = code
	s.task.FinishedAt = &now
	if s.killed {
		s.task.Status = StatusKilled
	} else {
		s.task.Status = StatusExited
	}
	// 进程没了，订阅者可以收尾了。
	for ch := range s.subs {
		close(ch)
		delete(s.subs, ch)
	}
	final := s.task
	onExit := s.onExit
	s.mu.Unlock()

	// 落盘要在 close(done) 之前完成：Kill/Remove/Restart 都靠 done 判断
	// "这个进程的收尾工作已经做完了"，提前关闭会让它们读到还没写完的状态。
	if onExit != nil {
		onExit(final)
	}

	close(s.done)
}

// newRestoredSession 为从磁盘恢复出来的任务造一个"已经结束"的会话。
// 它没有进程，终端只回放空内容，但任务记录、状态与操作入口都在。
func newRestoredSession(t Task) *Session {
	done := make(chan struct{})
	close(done)

	return &Session{
		task: t,
		buf:  &ringBuffer{max: 0},
		subs: make(map[chan []byte]struct{}),
		done: done,
	}
}

// appendOutput 把进程输出写进环形缓冲并广播给所有订阅者。
func (s *Session) appendOutput(p []byte) (int, error) {
	// 读取方复用缓冲区，必须先拷一份再分发。
	chunk := make([]byte, len(p))
	copy(chunk, p)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.buf.write(chunk)
	for ch := range s.subs {
		select {
		case ch <- chunk:
		default:
			// 订阅者跟不上了：断开它，前端重连后会拿到完整重放。
			close(ch)
			delete(s.subs, ch)
		}
	}
	return len(p), nil
}

// Task 返回任务当前状态的快照。
func (s *Session) Task() Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.task
	t.Args = append([]string(nil), s.task.Args...)
	return t
}

// Done 在进程结束后关闭。
func (s *Session) Done() <-chan struct{} { return s.done }

// Subscribe 注册一个订阅者，同时返回当前已积累的全部输出用于重放。
// 注册与快照在同一把锁下完成，所以不会重复也不会漏。
func (s *Session) Subscribe() (replay []byte, ch <-chan []byte, cancel func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	replay = s.buf.snapshot()

	// 进程已经结束了：只回放，给一个立刻关闭的空通道。
	if s.task.FinishedAt != nil {
		closed := make(chan []byte)
		close(closed)
		return replay, closed, func() {}
	}

	c := make(chan []byte, subBuffer)
	s.subs[c] = struct{}{}

	return replay, c, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.subs[c]; ok {
			close(c)
			delete(s.subs, c)
		}
	}
}

// WriteStdin 把前端发来的字节写进进程标准输入。
func (s *Session) WriteStdin(p []byte) error {
	// 从磁盘恢复出来的任务没有进程，自然也没有 stdin。
	if s.stdin == nil {
		return errors.New("任务没有运行中的进程")
	}
	_, err := s.stdin.Write(p)
	return err
}

// Kill 杀掉整个进程组，先 SIGTERM 再留一点时间，必要时 SIGKILL。
func (s *Session) Kill() error {
	s.mu.Lock()
	if s.task.FinishedAt != nil {
		s.mu.Unlock()
		return nil
	}
	s.killed = true
	pid := s.task.PID
	s.mu.Unlock()

	if pid <= 0 {
		return nil
	}

	// 负号表示整个进程组。BaiduPCS-Go 收到 SIGTERM 会正常收尾，
	// 已下载的分片和断点续传元数据都保留，之后重跑同一命令即可续传。
	if err := signalGroup(pid, syscall.SIGTERM); err != nil {
		return err
	}

	select {
	case <-s.done:
		return nil
	case <-time.After(5 * time.Second):
	}

	return signalGroup(pid, syscall.SIGKILL)
}

// signalGroup 给整个进程组发信号。进程已经自己退出时 kill 会返回 ESRCH，
// 那不是错误，忽略掉即可，否则会冒成一次没意义的 500。
func signalGroup(pid int, sig syscall.Signal) error {
	if err := syscall.Kill(-pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

// writerFunc 让普通函数满足 io.Writer。
type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
