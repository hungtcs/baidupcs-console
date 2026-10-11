package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hungtcs/baidupcs-console/internal/config"
)

func newTestManager(t *testing.T, saveRoot string) *Manager {
	t.Helper()

	m, err := NewManager(config.Config{
		CLIPath:           "BaiduPCS-Go",
		SaveRoot:          saveRoot,
		DataDir:           t.TempDir(),
		OutputBufferBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("创建 Manager 失败: %v", err)
	}
	return m
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入 %s 失败: %v", path, err)
	}
}

// 界面上「继续」还是「重新开始」完全取决于这个判断，所以四种情况都要守住。
func TestIsResumable(t *testing.T) {
	saveRoot := t.TempDir()
	m := newTestManager(t, saveRoot)

	t.Run("单文件：目标不存在", func(t *testing.T) {
		if m.IsResumable(Task{Name: "a.bin"}) {
			t.Error("什么都没下载过，不该判定为可续传")
		}
	})

	t.Run("单文件：已下完（没有未完成标记）", func(t *testing.T) {
		writeFile(t, filepath.Join(saveRoot, "done.bin"), "complete")
		if m.IsResumable(Task{Name: "done.bin"}) {
			t.Error("文件已下完时 CLI 会删掉标记，不该判定为可续传")
		}
	})

	t.Run("单文件：下到一半（有未完成标记）", func(t *testing.T) {
		target := filepath.Join(saveRoot, "partial.bin")
		writeFile(t, target, "half")
		writeFile(t, target+downloadingSuffix, "resume-meta")

		if !m.IsResumable(Task{Name: "partial.bin"}) {
			t.Error("有未完成标记时应该判定为可续传")
		}
	})

	t.Run("目录：里面的文件都下完了", func(t *testing.T) {
		dir := filepath.Join(saveRoot, "videos")
		writeFile(t, filepath.Join(dir, "a.mp4"), "complete")
		writeFile(t, filepath.Join(dir, "sub", "b.mp4"), "complete")

		if m.IsResumable(Task{Name: "videos", IsDir: true}) {
			t.Error("目录里没有未完成标记时不该判定为可续传")
		}
	})

	t.Run("目录：嵌套很深的地方有未完成标记", func(t *testing.T) {
		dir := filepath.Join(saveRoot, "videos")
		writeFile(t, filepath.Join(dir, "sub", "deep", "b.mp4")+downloadingSuffix, "resume-meta")

		if !m.IsResumable(Task{Name: "videos", IsDir: true}) {
			t.Error("目录里只要有任意一个未完成标记，就应该判定为可续传")
		}
	})
}
