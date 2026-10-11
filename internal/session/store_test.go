package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hungtcs/baidupcs-console/internal/config"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("创建 Store 失败: %v", err)
	}
	return store
}

func sampleTask(id string) Task {
	started := time.Date(2026, 10, 11, 21, 0, 0, 0, time.Local)
	finished := started.Add(90 * time.Second)
	return Task{
		ID:          id,
		Name:        "备份文档图片视频音频归档素材测试示例报告.pdf",
		RemotePath:  "/示例资料/备份文档图片视频音频归档素材测试示例报告.pdf",
		SaveDir:     "/home/user/Downloads",
		Args:        []string{"download", "--saveto", "/home/user/Downloads", "/示例资料/..."},
		CommandLine: "BaiduPCS-Go download --saveto /home/user/Downloads /示例资料/...",
		Status:      StatusExited,
		ExitCode:    0,
		PID:         100742,
		StartedAt:   started,
		FinishedAt:  &finished,
	}
}

func TestStoreRoundTrip(t *testing.T) {
	store := newTestStore(t)
	want := sampleTask("t_abcd1234")

	if err := store.Save(want); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	tasks, err := store.LoadAll()
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("期望 1 个任务，实际 %d 个", len(tasks))
	}

	got := tasks[0]
	if got.ID != want.ID || got.Name != want.Name || got.RemotePath != want.RemotePath {
		t.Errorf("基本字段不一致:\n得到 %+v\n期望 %+v", got, want)
	}
	if got.Status != want.Status || got.ExitCode != want.ExitCode || got.PID != want.PID {
		t.Errorf("状态字段不一致: %+v", got)
	}
	// 中文路径与带空格的参数要能原样往返。
	if len(got.Args) != len(want.Args) || got.Args[3] != want.Args[3] {
		t.Errorf("Args 不一致: %v", got.Args)
	}
	if !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("StartedAt 不一致: %v vs %v", got.StartedAt, want.StartedAt)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(*want.FinishedAt) {
		t.Errorf("FinishedAt 不一致: %v", got.FinishedAt)
	}
}

// 一个任务一个文件：改一个任务不该动到别的任务的文件。
func TestStoreOneFilePerTask(t *testing.T) {
	store := newTestStore(t)

	for _, id := range []string{"t_aaa", "t_bbb", "t_ccc"} {
		if err := store.Save(sampleTask(id)); err != nil {
			t.Fatalf("保存 %s 失败: %v", id, err)
		}
	}

	entries, err := os.ReadDir(store.Dir())
	if err != nil {
		t.Fatalf("读取目录失败: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("期望 3 个文件，实际 %d 个", len(entries))
	}

	tasks, err := store.LoadAll()
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("期望读回 3 个任务，实际 %d 个", len(tasks))
	}
}

func TestStoreDelete(t *testing.T) {
	store := newTestStore(t)
	if err := store.Save(sampleTask("t_aaa")); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	if err := store.Delete("t_aaa"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	tasks, _ := store.LoadAll()
	if len(tasks) != 0 {
		t.Errorf("删除后不该还有任务: %+v", tasks)
	}

	// 删两次不该报错。
	if err := store.Delete("t_aaa"); err != nil {
		t.Errorf("重复删除不该报错: %v", err)
	}
}

// 写入必须是原子的：不能留下 .tmp 残骸，也不能出现半个文件。
func TestStoreSaveIsAtomic(t *testing.T) {
	store := newTestStore(t)

	path, err := store.path("t_aaa")
	if err != nil {
		t.Fatalf("计算路径失败: %v", err)
	}
	if err := store.Save(sampleTask("t_aaa")); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("不该留下 .tmp 文件")
	}

	// 覆盖写也要正常。
	task := sampleTask("t_aaa")
	task.Status = StatusKilled
	if err := store.Save(task); err != nil {
		t.Fatalf("覆盖保存失败: %v", err)
	}
	tasks, _ := store.LoadAll()
	if len(tasks) != 1 || tasks[0].Status != StatusKilled {
		t.Errorf("覆盖保存后状态不对: %+v", tasks)
	}
}

// 单个文件坏掉只跳过它，不该让整个列表读不出来。
func TestStoreSkipsCorruptFile(t *testing.T) {
	store := newTestStore(t)

	if err := store.Save(sampleTask("t_good")); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	bad := filepath.Join(store.Dir(), "t_bad.json")
	if err := os.WriteFile(bad, []byte("{ 这不是合法 JSON"), 0o644); err != nil {
		t.Fatalf("写入坏文件失败: %v", err)
	}

	tasks, err := store.LoadAll()
	if err != nil {
		t.Fatalf("读取不该整体失败: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "t_good" {
		t.Errorf("应该只读回那个完好的任务，实际 %+v", tasks)
	}
}

func TestStoreRejectsUnsafeID(t *testing.T) {
	store := newTestStore(t)

	for _, id := range []string{"", "..", "../escape", "a/b", `a\b`} {
		if err := store.Save(Task{ID: id}); err == nil {
			t.Errorf("ID %q 应该被拒绝", id)
		}
	}
}

// 服务重启后，上次还在跑的任务应该以「已停止」的形态恢复出来。
func TestManagerRestoresTasks(t *testing.T) {
	dataDir := t.TempDir()

	store, err := NewStore(dataDir)
	if err != nil {
		t.Fatalf("创建 Store 失败: %v", err)
	}

	// 模拟"上次服务退出时这个任务还在跑"。
	running := sampleTask("t_running")
	running.Status = StatusRunning
	running.FinishedAt = nil
	if err := store.Save(running); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	done := sampleTask("t_done")
	if err := store.Save(done); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	cfg := config.Config{
		CLIPath:           "BaiduPCS-Go",
		SaveRoot:          t.TempDir(),
		DataDir:           dataDir,
		OutputBufferBytes: 1 << 20,
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("创建 Manager 失败: %v", err)
	}

	list := m.List()
	if len(list) != 2 {
		t.Fatalf("期望恢复 2 个任务，实际 %d 个: %+v", len(list), list)
	}

	byID := make(map[string]Task, len(list))
	for _, task := range list {
		byID[task.ID] = task
	}

	restored := byID["t_running"]
	if restored.Status != StatusKilled {
		t.Errorf("上次在跑的任务应恢复成 %q，实际 %q", StatusKilled, restored.Status)
	}
	if restored.FinishedAt == nil {
		t.Error("恢复出来的任务必须有 FinishedAt，否则前端会当成还在跑")
	}

	// 已经结束的任务状态不该被改动。
	if byID["t_done"].Status != StatusExited {
		t.Errorf("已结束的任务状态被改坏了: %q", byID["t_done"].Status)
	}

	// 恢复出来的任务没有进程，终端订阅应当立刻拿到空重放并关闭。
	sess, err := m.Get("t_done")
	if err != nil {
		t.Fatalf("取任务失败: %v", err)
	}
	replay, ch, cancel := sess.Subscribe()
	defer cancel()
	if len(replay) != 0 {
		t.Errorf("恢复出来的任务不该有终端历史: %q", replay)
	}
	if _, ok := <-ch; ok {
		t.Error("恢复出来的任务，订阅通道应当已经关闭")
	}

	// 删除恢复出来的任务，对应文件也要消失。
	if err := m.Remove("t_done"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if tasks, _ := store.LoadAll(); len(tasks) != 1 {
		t.Errorf("删除后磁盘上应只剩 1 个任务文件，实际 %d 个", len(tasks))
	}
}
