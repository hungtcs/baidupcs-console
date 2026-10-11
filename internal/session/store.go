package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Store 把任务记录持久化到磁盘，一个任务一个 JSON 文件：
//
//	<dir>/tasks/<任务ID>.json
//
// 一个任务一个文件（而不是一个总文件）的好处是：状态变化只重写它自己那份，
// 不必全量重写；某一份写坏了也只影响那一个任务，不会把所有任务一起带走。
type Store struct {
	dir string
}

const taskFileSuffix = ".json"

// NewStore 准备数据目录，目录不存在时创建。
func NewStore(dir string) (*Store, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("数据目录不能为空")
	}

	tasksDir := filepath.Join(dir, "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建数据目录 %s: %w", tasksDir, err)
	}
	return &Store{dir: tasksDir}, nil
}

// Dir 返回存放任务文件的目录。
func (s *Store) Dir() string { return s.dir }

func (s *Store) path(id string) (string, error) {
	// ID 由本程序生成（t_ + 十六进制），但恢复出来的 ID 来自文件名，
	// 所以仍然挡一下，避免任何形式的路径穿越。
	if id == "" || strings.ContainsAny(id, `/\`) || id == "." || id == ".." {
		return "", fmt.Errorf("非法的任务 ID: %q", id)
	}
	return filepath.Join(s.dir, id+taskFileSuffix), nil
}

// Save 原子地写入任务文件：先写 .tmp 再 rename，避免读到写了一半的内容。
func (s *Store) Save(t Task) error {
	path, err := s.path(t.ID)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化任务 %s: %w", t.ID, err)
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("写入任务 %s: %w", t.ID, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("提交任务 %s: %w", t.ID, err)
	}
	return nil
}

// Delete 删除任务文件。文件本来就不存在时不算错误。
func (s *Store) Delete(id string) error {
	path, err := s.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除任务 %s: %w", id, err)
	}
	return nil
}

// LoadAll 读出全部任务。单个文件损坏只跳过它并记一条日志，不影响其余任务。
func (s *Store) LoadAll() ([]Task, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取数据目录 %s: %w", s.dir, err)
	}

	var tasks []Task
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, taskFileSuffix) {
			continue
		}
		// 跳过写了一半的临时文件。
		if strings.HasSuffix(name, ".tmp") {
			continue
		}

		path := filepath.Join(s.dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("读取任务文件失败，已跳过", "file", path, "error", err)
			continue
		}

		var t Task
		if err := json.Unmarshal(data, &t); err != nil {
			slog.Warn("任务文件解析失败，已跳过", "file", path, "error", err)
			continue
		}
		if t.ID == "" {
			t.ID = strings.TrimSuffix(name, taskFileSuffix)
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}
