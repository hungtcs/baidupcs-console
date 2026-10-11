package pcs

import (
	"os"
	"path/filepath"
	"testing"
)

// testdata 下是真实 BaiduPCS-Go v4.0.2 输出的原始字节（含 \r），
// 用 `> file 2>&1` 直接录下来的，没有做过任何加工。
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("读取夹具 %s 失败: %v", name, err)
	}
	return data
}

// 一次性喂进去（真实运行时输出是按块到达的，另有专门用例覆盖分块）。
func parseFixture(t *testing.T, name string) *Progress {
	t.Helper()

	p := NewProgressParser()
	p.Write(loadFixture(t, name))
	return p.Snapshot()
}

func TestProgressSingleFile(t *testing.T) {
	prog := parseFixture(t, "download_single_file.raw")
	if prog == nil {
		t.Fatal("应该解析出进度，实际是 nil")
	}
	t.Logf("单文件解析结果: %+v", *prog)

	// 夹具录的是 139.20MB 那个 PDF，被 timeout 在 60 秒时杀掉。
	if prog.TotalText != "139.20MB" {
		t.Errorf("总量原样文本错误: %q", prog.TotalText)
	}
	if prog.TotalBytes != mb(139.20) {
		t.Errorf("总量字节数错误: %d，期望 %d", prog.TotalBytes, mb(139.20))
	}
	if prog.DoneBytes <= 0 || prog.DoneBytes > prog.TotalBytes {
		t.Errorf("已完成字节数不合理: %d / %d", prog.DoneBytes, prog.TotalBytes)
	}
	if prog.Percent <= 0 || prog.Percent >= 100 {
		t.Errorf("百分比应该在 0 和 100 之间，实际 %v", prog.Percent)
	}

	// 单文件任务：只出现过一个索引。
	if prog.FilesTotal != 1 {
		t.Errorf("单文件任务的 FilesTotal 应为 1，实际 %d", prog.FilesTotal)
	}
	if prog.FilesDone != 0 {
		t.Errorf("没下载完，FilesDone 应为 0，实际 %d", prog.FilesDone)
	}
	if prog.Finished {
		t.Error("被 timeout 杀掉，不该标记为 Finished")
	}

	// 速度要么有值要么是 0（夹具里出现过 0B/s），但文本必须与 CLI 一致。
	if prog.SpeedText == "" || prog.SpeedText[len(prog.SpeedText)-2:] != "/s" {
		t.Errorf("速度原样文本错误: %q", prog.SpeedText)
	}
	if prog.EtaText == "" {
		t.Error("剩余时间文本不该为空")
	}
}

// 最后一行进度行必须是解析结果，不能被更早的行覆盖。
func TestProgressUsesLatestLine(t *testing.T) {
	prog := parseFixture(t, "download_single_file.raw")
	if prog == nil {
		t.Fatal("应该解析出进度")
	}

	// 夹具最后一行是 "in 56s"（被 timeout 杀掉那一刻）。
	if prog.ElapsedText != "56s" {
		t.Errorf("应该采用最后一行进度行的耗时，实际 %q", prog.ElapsedText)
	}
}

func TestProgressDirectory(t *testing.T) {
	prog := parseFixture(t, "download_directory.raw")
	if prog == nil {
		t.Fatal("应该解析出进度")
	}
	t.Logf("目录解析结果: %+v", *prog)

	// 目录下载会逐个文件打印，索引递增，所以 FilesTotal 应该大于 1。
	if prog.FilesTotal < 2 {
		t.Errorf("目录下载的 FilesTotal 应该大于 1，实际 %d", prog.FilesTotal)
	}
	if prog.FilesDone < 1 {
		t.Errorf("夹具里已经有文件下完了，FilesDone 应该 >= 1，实际 %d", prog.FilesDone)
	}
	if prog.FilesDone > prog.FilesTotal {
		t.Errorf("已完成数不该超过已知总数: %d / %d", prog.FilesDone, prog.FilesTotal)
	}

	// 当前文件名来自「准备下载」那一行。
	if prog.CurrentName == "" {
		t.Error("应该解析出当前文件名")
	}
	if filepath.Ext(prog.CurrentName) == "" {
		t.Errorf("当前文件名看起来不像文件名: %q", prog.CurrentName)
	}

	// 当前文件是 adx.gif，49 字节，应该已经下完。
	if prog.CurrentName == "adx.gif" {
		if prog.DoneBytes != 49 || prog.TotalBytes != 49 {
			t.Errorf("adx.gif 应为 49/49 字节，实际 %d/%d", prog.DoneBytes, prog.TotalBytes)
		}
		if prog.Percent != 100 {
			t.Errorf("已下完的文件百分比应为 100，实际 %v", prog.Percent)
		}
	}
}

func TestProgressFinished(t *testing.T) {
	prog := parseFixture(t, "download_completed.raw")
	if prog == nil {
		// 这个夹具是个 2KB 的小文件，下载太快可能一条进度行都没有，
		// 那种情况下没有进度是正确行为。
		t.Skip("小文件没有产生进度行，属于预期情况")
	}
	if !prog.Finished {
		t.Error("夹具里有「下载结束」，应该标记为 Finished")
	}
}

// 输出是按块到达的，一块可能在半行处断开——包括多字节 UTF-8 被切开的情况。
// 逐字节喂进去是最坏情况，结果必须和一次性喂完全一致。
func TestProgressChunkBoundaryAgnostic(t *testing.T) {
	for _, name := range []string{
		"download_single_file.raw",
		"download_directory.raw",
	} {
		t.Run(name, func(t *testing.T) {
			raw := loadFixture(t, name)

			whole := NewProgressParser()
			whole.Write(raw)
			want := whole.Snapshot()

			// 逐字节喂。
			byteWise := NewProgressParser()
			for i := range raw {
				byteWise.Write(raw[i : i+1])
			}
			got := byteWise.Snapshot()

			if (want == nil) != (got == nil) {
				t.Fatalf("分块喂入后结果从 %v 变成了 %v", want, got)
			}
			if want != nil && *want != *got {
				t.Errorf("分块喂入结果不一致:\n整块 %+v\n逐字节 %+v", *want, *got)
			}
		})
	}
}

// 半行缓存不能无限增长。
func TestProgressPartialLineIsBounded(t *testing.T) {
	p := NewProgressParser()

	// 一直喂没有分隔符的内容。
	chunk := make([]byte, 4096)
	for i := range chunk {
		chunk[i] = 'x'
	}
	for i := 0; i < 100; i++ {
		p.Write(chunk)
	}

	if len(p.buf) > maxPartialLine {
		t.Errorf("半行缓存 %d 超过上限 %d", len(p.buf), maxPartialLine)
	}
}

func TestProgressEmptyInput(t *testing.T) {
	p := NewProgressParser()
	if prog := p.Snapshot(); prog != nil {
		t.Errorf("没有任何输入时应该返回 nil，实际 %+v", prog)
	}

	// 只有与进度无关的输出，也不该凭空造出进度。
	p.Write([]byte("[1] 加入下载队列: /我的资源/a.pdf\n[1] 准备下载: /我的资源/a.pdf\n"))
	if prog := p.Snapshot(); prog != nil {
		t.Errorf("没有进度行时应该返回 nil，实际 %+v", prog)
	}
}

// 一个文件下载完成时，即使最后一个进度行没到 100%，也要按完成处理。
func TestProgressFileCompletionSnapsTo100(t *testing.T) {
	p := NewProgressParser()
	p.Write([]byte(
		"[1] 准备下载: /我的资源/a.bin\n" +
			"[1] ↓ 900B/1000B 100B/s in 9s, left 1s ............\r" +
			"[1] 下载完成, 保存位置: /tmp/a.bin\n" +
			"[1] 跳过文件有效性检验\n" +
			"下载结束, 时间: 9.1s, 数据总量: 1000B\n"))

	prog := p.Snapshot()
	if prog == nil {
		t.Fatal("应该解析出进度")
	}
	if prog.Percent != 100 {
		t.Errorf("看到「下载完成」后百分比应为 100，实际 %v", prog.Percent)
	}
	if prog.DoneBytes != 1000 {
		t.Errorf("看到「下载完成」后已完成字节应等于总量，实际 %d", prog.DoneBytes)
	}
	if !prog.Finished {
		t.Error("应该标记为 Finished")
	}
	if prog.FilesDone != 1 {
		t.Errorf("FilesDone 应为 1，实际 %d", prog.FilesDone)
	}
}

// 速度文本带 /s 后缀，要能正确转成字节/秒。
func TestParseSpeed(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"0B/s", 0},
		{"62B/s", 62},
		{"100.99KB/s", kb(100.99)},
		{"213.21KB/s", kb(213.21)},
		{"1.50MB/s", mb(1.5)},
	}
	for _, c := range cases {
		if got := parseSpeed(c.in); got != c.want {
			t.Errorf("parseSpeed(%q) = %d，期望 %d", c.in, got, c.want)
		}
	}
}
