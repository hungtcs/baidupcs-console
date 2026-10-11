package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hungtcs/baidupcs-console/internal/config"
	"github.com/hungtcs/baidupcs-console/internal/pcs"
)

// ErrNotFound 表示指定的任务不存在。
var ErrNotFound = errors.New("任务不存在")

// Manager 持有全部任务会话，并把任务记录持久化到数据目录。
type Manager struct {
	cfg   config.Config
	cli   *pcs.Client
	store *Store

	mu       sync.RWMutex
	sessions map[string]*Session
	order    []string
}

// NewManager 创建任务管理器，并把上次保存的任务读回内存。
//
// 上次运行时进程是随服务一起退出的，所以恢复出来的 running 任务一律标成
// killed（进程确实被杀了，状态没失真），由用户决定是否「重新开始」。
func NewManager(cfg config.Config) (*Manager, error) {
	store, err := NewStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	m := &Manager{
		cfg:      cfg,
		cli:      &pcs.Client{CLIPath: cfg.CLIPath},
		store:    store,
		sessions: make(map[string]*Session),
	}
	m.restore()
	return m, nil
}

// restore 把磁盘上的任务读回内存。
func (m *Manager) restore() {
	tasks, err := m.store.LoadAll()
	if err != nil {
		slog.Error("读取已保存的任务失败，本次以空列表启动", "error", err)
		return
	}
	if len(tasks) == 0 {
		return
	}

	// 按开始时间排序，保证恢复后的列表顺序稳定（List 会再倒过来）。
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].StartedAt.Before(tasks[j].StartedAt) })

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, t := range tasks {
		if t.Status == StatusRunning {
			t.Status = StatusKilled
		}
		if t.FinishedAt == nil {
			now := time.Now()
			t.FinishedAt = &now
		}
		m.sessions[t.ID] = newRestoredSession(t)
		m.order = append(m.order, t.ID)
	}

	slog.Info("已恢复任务列表", "count", len(tasks), "dir", m.store.Dir())
}

// persist 把任务状态写盘。写失败只记日志，不影响下载本身。
func (m *Manager) persist(t Task) {
	if err := m.store.Save(t); err != nil {
		slog.Error("保存任务失败", "id", t.ID, "error", err)
	}
}

// StartDownload 校验远端路径后，把下载命令作为后台进程跑起来。
func (m *Manager) StartDownload(ctx context.Context, remotePath string) (Task, error) {
	remotePath = strings.TrimSpace(remotePath)
	if remotePath == "" {
		return Task{}, errors.New("网盘路径不能为空")
	}
	if !strings.HasPrefix(remotePath, "/") {
		remotePath = "/" + remotePath
	}

	// 先探一次元信息：CLI 在路径不存在时会静默地什么都不做并返回 0，
	// 提前校验能把这种情况变成一条明确的报错。
	meta, err := m.cli.Meta(ctx, remotePath)
	if err != nil {
		return Task{}, err
	}

	if err := os.MkdirAll(m.cfg.SaveRoot, 0o755); err != nil {
		return Task{}, fmt.Errorf("创建下载目录 %s: %w", m.cfg.SaveRoot, err)
	}

	name := meta.Name
	if name == "" {
		name = path.Base(remotePath)
	}

	args := pcs.DownloadArgs(remotePath, m.cfg.SaveRoot, m.cfg.Mode, m.cfg.MaxThreads)

	sess, err := m.launch(startOptions{
		ID:          newID(),
		Name:        name,
		RemotePath:  remotePath,
		SaveDir:     m.cfg.SaveRoot,
		CLIPath:     m.cfg.CLIPath,
		Args:        args,
		BufferBytes: m.cfg.OutputBufferBytes,
		OnExit:      m.persist,
	})
	if err != nil {
		return Task{}, fmt.Errorf("启动 %s: %w", m.cfg.CLIPath, err)
	}

	// 立刻落盘，这样服务中途挂掉也不会丢掉这个任务。
	task := sess.Task()
	m.persist(task)

	return task, nil
}

// launch 启动进程并把它登记进任务表。调用方保证 ID 是新的。
func (m *Manager) launch(opts startOptions) (*Session, error) {
	sess, err := start(opts)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.sessions[opts.ID] = sess
	m.order = append(m.order, opts.ID)
	m.mu.Unlock()

	return sess, nil
}

// Get 按 ID 取会话。
func (m *Manager) Get(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sess, ok := m.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	return sess, nil
}

// List 返回全部任务的快照，最新的在前。
func (m *Manager) List() []Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Task, 0, len(m.order))
	for i := len(m.order) - 1; i >= 0; i-- {
		if sess, ok := m.sessions[m.order[i]]; ok {
			out = append(out, sess.Task())
		}
	}
	return out
}

// Kill 停止任务进程，但保留任务记录与已下载的分片。
func (m *Manager) Kill(id string) (Task, error) {
	sess, err := m.Get(id)
	if err != nil {
		return Task{}, err
	}
	if err := sess.Kill(); err != nil {
		return Task{}, fmt.Errorf("结束进程: %w", err)
	}
	waitDone(sess)
	return sess.Task(), nil
}

// Restart 用同样的参数重新起一个进程，任务 ID 保持不变。
// BaiduPCS-Go 支持断点续传，所以这就是「继续下载」。
func (m *Manager) Restart(id string) (Task, error) {
	old, err := m.Get(id)
	if err != nil {
		return Task{}, err
	}
	prev := old.Task()

	// 先停掉旧进程，避免两个进程同时往同一批文件里写。
	_ = old.Kill()
	// 必须等旧进程的收尾（含落盘）彻底结束，否则它会把新任务的状态覆盖回去。
	waitDone(old)

	if err := os.MkdirAll(m.cfg.SaveRoot, 0o755); err != nil {
		return Task{}, fmt.Errorf("创建下载目录 %s: %w", m.cfg.SaveRoot, err)
	}

	sess, err := start(startOptions{
		ID:          id,
		Name:        prev.Name,
		RemotePath:  prev.RemotePath,
		SaveDir:     m.cfg.SaveRoot,
		CLIPath:     m.cfg.CLIPath,
		Args:        prev.Args,
		BufferBytes: m.cfg.OutputBufferBytes,
		OnExit:      m.persist,
	})
	if err != nil {
		return Task{}, fmt.Errorf("启动 %s: %w", m.cfg.CLIPath, err)
	}

	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()

	task := sess.Task()
	m.persist(task)

	return task, nil
}

// Remove 停止任务并连同它的记录一起删除。磁盘上已下载的文件不动。
func (m *Manager) Remove(id string) error {
	sess, err := m.Get(id)
	if err != nil {
		return err
	}

	_ = sess.Kill()
	// 等收尾结束再删文件，否则收尾时的落盘会把刚删掉的记录又写回来。
	waitDone(sess)

	m.mu.Lock()
	delete(m.sessions, id)
	for i, v := range m.order {
		if v == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.mu.Unlock()

	if err := m.store.Delete(id); err != nil {
		slog.Error("删除任务文件失败", "id", id, "error", err)
	}
	return nil
}

// KillAll 停掉所有还在运行的进程，用于程序退出时收尾。
// 每个进程的收尾都会把最终状态落盘，所以重启后能看到它们处于「已停止」。
func (m *Manager) KillAll() {
	m.mu.RLock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, s := range sessions {
		wg.Add(1)
		go func(s *Session) {
			defer wg.Done()
			_ = s.Kill()
			waitDone(s)
		}(s)
	}
	wg.Wait()
}

// waitDone 等会话的收尾工作结束（done 关闭发生在落盘之后），带一个兜底超时，
// 避免极端情况下把请求一直挂住。
func waitDone(sess *Session) {
	select {
	case <-sess.Done():
	case <-time.After(5 * time.Second):
	}
}
