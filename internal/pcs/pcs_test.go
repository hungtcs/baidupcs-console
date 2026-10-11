package pcs

import (
	"math"
	"strings"
	"testing"
)

// 下面这些输出都是从真实的 BaiduPCS-Go v4.0.2 上原样抄下来的，
// 包括中文对齐留下的尾随空格。改解析逻辑时这些用例必须继续通过。

const realLsRoot = `
当前目录: /
----
  #    文件大小         修改日期                     文件(目录)
  0             -  2017-07-20 17:03:36  @hungtcs/
  1             -  2014-06-10 01:20:15  apps/
  2             -  2024-03-19 23:08:28  Games/
  3             -  2016-01-12 22:15:04  Programmer/
  4             -  2026-09-16 20:58:40  Temp/
  5             -  2020-04-16 16:44:43  电子书/
  6             -  2026-09-11 00:50:24  我的资源/
  7      465.96MB  2026-10-11 20:09:43  YX57599-STM8S103FP6系统板-190508.zip
     总: 465.96MB                       文件总数: 1, 目录总数: 7
----
`

// 名字里有空格、顿号、全角字符，且列宽被中文撑开。
const realLsWithSpaces = `
当前目录: /我的资源/STM32F103RCT6   RBT6开发板 开发板资料
----
  #   文件大小        修改日期                           文件(目录)
  0           -  2019-02-07 12:17:22  1、程序开发工具/
  1           -  2019-02-07 12:17:22  2、STM32程序ＩＳＰ串口下载工具/
  2    950.64MB  2019-02-07 12:17:22  3、其他STM32开发板资料.rar
  3      4.13MB  2019-02-07 12:17:22  4、STM32_OLED_NRF(本店板子测试过).rar
----
`

const realMetaFile = `
[0] - [/我的资源/Document/Notes/Chrome运行Android.txt] --------------

  类型              文件
  文件路径          /我的资源/Document/Notes/Chrome运行Android.txt
  文件名称          Chrome运行Android.txt
  文件大小          2018, 1.970703KB
  md5 (截图请打码)  db009d99412a402b031561f8f153e6d7
  app_id            250528
  fs_id             739793078800285
  创建日期          2015-04-15 16:58:00
  修改日期          2015-12-12 18:46:12

`

const realMetaDir = `
[0] - [/我的资源/Document] --------------

  类型            目录
  目录路径        /我的资源/Document
  目录名称        Document
  app_id          250528
  fs_id           898837518163087
  创建日期        2014-05-09 17:54:14
  修改日期        2015-12-12 18:40:20
  是否含有子目录  true

`

// 大小换算的辅助函数。Go 不允许把非整数的常量表达式直接转成 int64，
// 所以走运行时计算，保证与 parseSize 用同一套 1024 进制。
func kb(f float64) int64 { return int64(math.Round(f * 1024)) }
func mb(f float64) int64 { return int64(math.Round(f * 1024 * 1024)) }
func gb(f float64) int64 { return int64(math.Round(f * 1024 * 1024 * 1024)) }
func tb(f float64) int64 { return int64(math.Round(f * 1024 * 1024 * 1024 * 1024)) }

func TestParseLsRoot(t *testing.T) {
	entries := parseLs(realLsRoot, "/")

	if len(entries) != 8 {
		t.Fatalf("期望 8 个条目，实际 %d 个: %+v", len(entries), entries)
	}

	// 目录：大小列是 "-"，名字以 "/" 结尾。
	first := entries[0]
	if first.Name != "@hungtcs" || !first.IsDir || first.Size != 0 {
		t.Errorf("第 0 项解析错误: %+v", first)
	}
	if first.Path != "/@hungtcs" {
		t.Errorf("路径应为 /@hungtcs，实际 %q", first.Path)
	}
	if first.ModTime != "2017-07-20T17:03:36+08:00" && !strings.HasPrefix(first.ModTime, "2017-07-20T17:03:36") {
		t.Errorf("修改时间解析错误: %q", first.ModTime)
	}

	// 前端要跟 CLI 显示得一模一样，所以原样文本必须保留。
	if first.SizeText != "-" {
		t.Errorf("目录的大小原样文本应为 \"-\"，实际 %q", first.SizeText)
	}
	if first.ModTimeText != "2017-07-20 17:03:36" {
		t.Errorf("日期的原样文本错误: %q", first.ModTimeText)
	}

	// 中文目录名不能被尾随空格污染。
	if entries[5].Name != "电子书" || !entries[5].IsDir {
		t.Errorf("中文目录名解析错误: %+v", entries[5])
	}

	// 文件：大小 465.96MB，单位是 1024 进制。
	last := entries[7]
	if last.IsDir {
		t.Errorf("第 7 项不该被判定为目录: %+v", last)
	}
	want := mb(465.96)
	if last.Size != want {
		t.Errorf("大小应为 %d，实际 %d", want, last.Size)
	}
	if last.Name != "YX57599-STM8S103FP6系统板-190508.zip" {
		t.Errorf("文件名解析错误: %q", last.Name)
	}
	// CLI 显示的是 465.96MB；从字节数反推不保证能还原，所以必须是原样文本。
	if last.SizeText != "465.96MB" {
		t.Errorf("大小原样文本应为 \"465.96MB\"，实际 %q", last.SizeText)
	}
	if last.ModTimeText != "2026-10-11 20:09:43" {
		t.Errorf("日期原样文本错误: %q", last.ModTimeText)
	}

	// 汇总行「总: ... 文件总数: ...」不能被当成条目。
	for _, e := range entries {
		if strings.Contains(e.Name, "文件总数") || strings.HasPrefix(e.Name, "总:") {
			t.Errorf("汇总行被误解析成条目: %+v", e)
		}
	}
}

func TestParseLsNamesWithSpaces(t *testing.T) {
	entries := parseLs(realLsWithSpaces, "/我的资源/STM32F103RCT6   RBT6开发板 开发板资料")

	if len(entries) != 4 {
		t.Fatalf("期望 4 个条目，实际 %d 个: %+v", len(entries), entries)
	}

	if entries[0].Name != "1、程序开发工具" || !entries[0].IsDir {
		t.Errorf("第 0 项解析错误: %+v", entries[0])
	}
	if entries[2].Name != "3、其他STM32开发板资料.rar" {
		t.Errorf("第 2 项文件名解析错误: %q", entries[2].Name)
	}
	if entries[2].Size != mb(950.64) {
		t.Errorf("第 2 项大小解析错误: %d", entries[2].Size)
	}
	// 括号不能被当成路径分隔或截断。
	if entries[3].Name != "4、STM32_OLED_NRF(本店板子测试过).rar" {
		t.Errorf("第 3 项文件名解析错误: %q", entries[3].Name)
	}
}

func TestParseLsEmpty(t *testing.T) {
	out := `
当前目录: /empty
----
  #    文件大小         修改日期                     文件(目录)
     总: 0B                             文件总数: 0, 目录总数: 0
----
`
	if entries := parseLs(out, "/empty"); len(entries) != 0 {
		t.Errorf("空目录应解析出 0 个条目，实际 %+v", entries)
	}
}

func TestParseMetaFile(t *testing.T) {
	m := parseMeta(realMetaFile, "/我的资源/Document/Notes/Chrome运行Android.txt")

	if m.IsDir {
		t.Error("不该被判定为目录")
	}
	if m.Name != "Chrome运行Android.txt" {
		t.Errorf("文件名错误: %q", m.Name)
	}
	// "文件大小 2018, 1.970703KB" 里逗号前是精确字节数。
	if m.Size != 2018 {
		t.Errorf("大小应为 2018，实际 %d", m.Size)
	}
	if m.MD5 != "db009d99412a402b031561f8f153e6d7" {
		t.Errorf("md5 错误: %q", m.MD5)
	}
	// 键 "md5 (截图请打码)" 里有一个单空格，不能把键切坏。
	if !strings.HasPrefix(m.ModTime, "2015-12-12T18:46:12") {
		t.Errorf("修改时间错误: %q", m.ModTime)
	}
}

func TestParseMetaDir(t *testing.T) {
	m := parseMeta(realMetaDir, "/我的资源/Document")

	if !m.IsDir {
		t.Error("应被判定为目录")
	}
	if m.Name != "Document" {
		t.Errorf("目录名错误: %q", m.Name)
	}
	if m.Size != 0 {
		t.Errorf("目录大小应为 0，实际 %d", m.Size)
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"-", 0},
		{"", 0},
		{"0B", 0},
		{"2018", 2018},
		{"1.970703KB", 2018},
		{"64.16KB", kb(64.16)},
		{"465.96MB", mb(465.96)},
		{"1.39GB", gb(1.39)},
		{"2.008789TB", tb(2.008789)},
		{"垃圾数据", 0},
	}
	for _, c := range cases {
		if got := parseSize(c.in); got != c.want {
			t.Errorf("parseSize(%q) = %d, 期望 %d", c.in, got, c.want)
		}
	}
}

func TestExtractError(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "ls 路径不存在",
			in:   "获取目录下的文件列表: 遇到错误, 远端服务器返回错误, 代码: -9, 消息: 文件不存在",
			want: "遇到错误, 远端服务器返回错误, 代码: -9, 消息: 文件不存在",
		},
		{
			name: "meta 路径不存在",
			in:   "[0] - [/nope-xyz] --------------\n获取文件/目录的元信息: 遇到错误, 远端服务器返回错误, 代码: 31066, 消息: 文件或目录不存在",
			want: "遇到错误, 远端服务器返回错误, 代码: 31066, 消息: 文件或目录不存在",
		},
		{name: "正常输出", in: "当前目录: /\n----\n  0  -  2017-07-20 17:03:36  apps/", want: ""},
		{name: "空输出", in: "", want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractError(c.in); got != c.want {
				t.Errorf("extractError() = %q, 期望 %q", got, c.want)
			}
		})
	}
}

// CLI 出错时退出码也是 0，所以「没有错误标记」必须被当成成功，
// 否则正常输出会被误判。这里顺带守住这个前提。
func TestExtractErrorDoesNotFireOnNormalOutput(t *testing.T) {
	if msg := extractError(realLsRoot); msg != "" {
		t.Errorf("正常 ls 输出不该被判定为错误，却得到 %q", msg)
	}
	if msg := extractError(realMetaFile); msg != "" {
		t.Errorf("正常 meta 输出不该被判定为错误，却得到 %q", msg)
	}
}

func TestNormalizeRemote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "/"},
		{"/", "/"},
		{"我的资源", "/我的资源"},
		{"/我的资源/", "/我的资源"},
		{"/我的资源//Document", "/我的资源/Document"},
		{"/a/./b", "/a/b"},
	}
	for _, c := range cases {
		if got := NormalizeRemote(c.in); got != c.want {
			t.Errorf("NormalizeRemote(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}

func TestParentRemote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/", "/"},
		{"/我的资源", "/"},
		{"/我的资源/Document", "/我的资源"},
		{"/我的资源/Document/", "/我的资源"},
	}
	for _, c := range cases {
		if got := ParentRemote(c.in); got != c.want {
			t.Errorf("ParentRemote(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}

func TestDownloadArgs(t *testing.T) {
	got := DownloadArgs("/我的资源/a.pdf", "/data/dl", "locate", 0)
	want := []string{"download", "--saveto", "/data/dl", "--mode", "locate", "/我的资源/a.pdf"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("DownloadArgs() = %v, 期望 %v", got, want)
	}

	// MaxThreads 为 0 时不该出现 -p。
	got = DownloadArgs("/a.pdf", "/dl", "pcs", 8)
	want = []string{"download", "--saveto", "/dl", "--mode", "pcs", "-p", "8", "/a.pdf"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("DownloadArgs() = %v, 期望 %v", got, want)
	}

	// 路径带空格时必须作为一个独立参数传，不能靠 shell 拼接。
	got = DownloadArgs("/我的资源/a b.pdf", "/dl", "", 0)
	if got[len(got)-1] != "/我的资源/a b.pdf" {
		t.Errorf("带空格的路径应保持为单个参数: %v", got)
	}
}

func TestCommandLineQuotesSpaces(t *testing.T) {
	line := CommandLine("BaiduPCS-Go", []string{"download", "/我的资源/a b.pdf"})
	if !strings.Contains(line, `"/我的资源/a b.pdf"`) {
		t.Errorf("含空格的参数应被引号包起来，实际: %s", line)
	}
}
