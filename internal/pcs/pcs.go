// Package pcs 封装对 BaiduPCS-Go 可执行文件的调用与文本输出解析。
//
// 关于这个 CLI 的几个实测结论，解析逻辑都建立在这上面：
//
//  1. 所有输出都写 stdout，stderr 始终为空。
//  2. 退出码恒为 0，出错时也是 0。判断成败只能扫描文本里的 "遇到错误"。
//  3. 非 TTY 环境下不输出 ANSI 颜色，但进度条用 \r 覆盖刷新。
//  4. ls 的列是按显示宽度对齐的，中文算两个宽度，所以不能按固定列位置切分，
//     只能用「日期时间」做锚点。目录名以 "/" 结尾，目录的大小列是 "-"。
//  5. 文件大小是 1024 进制：145961465 字节显示为 139.199700MB。
package pcs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Entry 是 ls 返回的一个目录项。
//
// 同时给出「结构化值」和「CLI 原样文本」两套字段：前端要跟 CLI 显示得一模一样，
// 而 CLI 的大小列有自己的精度（例如 465.96MB），从字节数反推不保证能还原，
// 所以直接把原始文本透传出去，由前端原样显示。
type Entry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`

	// Size 是解析出来的字节数，目录为 0。
	Size int64 `json:"size"`
	// SizeText 是 CLI 大小列的原样文本，目录为 "-"。
	SizeText string `json:"sizeText"`

	// ModTime 是 RFC3339 格式的修改时间。
	ModTime string `json:"modTime"`
	// ModTimeText 是 CLI 日期列的原样文本，形如 2017-07-20 17:03:36。
	ModTimeText string `json:"modTimeText"`
}

// Meta 是 meta 命令返回的元信息。
type Meta struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	MD5     string `json:"md5"`
	ModTime string `json:"modTime"`
}

// Client 是对 BaiduPCS-Go 的一次性调用封装。它是无状态的，可并发使用。
type Client struct {
	CLIPath string
}

// ls 行格式：`  0      465.96MB  2026-10-11 20:09:43  name.ext`
var lsRowRe = regexp.MustCompile(`^\s*(\d+)\s+(\S+)\s+(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\s+(.*?)\s*$`)

// errMarker 是 CLI 报告错误时固定出现的中文片段。
const errMarker = "遇到错误"

// Ls 列出网盘目录。remotePath 为空时视为根目录。
func (c *Client) Ls(ctx context.Context, remotePath string) ([]Entry, error) {
	remotePath = normalizeRemote(remotePath)

	out, err := c.run(ctx, "ls", remotePath)
	if err != nil {
		return nil, err
	}
	if msg := extractError(out); msg != "" {
		return nil, fmt.Errorf("列出 %s 失败: %s", remotePath, msg)
	}

	return parseLs(out, remotePath), nil
}

// parseLs 解析 ls 的表格输出。拆出来是为了能脱离真实 CLI 做单元测试。
func parseLs(out, remotePath string) []Entry {
	var entries []Entry
	for _, line := range strings.Split(out, "\n") {
		m := lsRowRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		rawSize, stamp, name := m[2], m[3], m[4]
		name = strings.TrimRight(name, " \t")
		if name == "" {
			continue
		}

		// 目录在 ls 输出里以 "/" 结尾，大小列是 "-"。
		isDir := strings.HasSuffix(name, "/")
		name = strings.TrimSuffix(name, "/")

		entries = append(entries, Entry{
			Name:        name,
			Path:        path.Join(remotePath, name),
			IsDir:       isDir,
			Size:        parseSize(rawSize),
			SizeText:    rawSize,
			ModTime:     parseTime(stamp),
			ModTimeText: stamp,
		})
	}
	return entries
}

// Meta 获取单个文件/目录的元信息。
func (c *Client) Meta(ctx context.Context, remotePath string) (*Meta, error) {
	remotePath = normalizeRemote(remotePath)

	out, err := c.run(ctx, "meta", remotePath)
	if err != nil {
		return nil, err
	}
	if msg := extractError(out); msg != "" {
		return nil, fmt.Errorf("获取 %s 的元信息失败: %s", remotePath, msg)
	}

	return parseMeta(out, remotePath), nil
}

// parseMeta 解析 meta 的「键  值」输出。
func parseMeta(out, remotePath string) *Meta {
	fields := parseTwoColumn(out)
	m := &Meta{
		Path: remotePath,
		MD5:  fields["md5"],
	}
	for k, v := range fields {
		if strings.HasPrefix(k, "md5") {
			m.MD5 = v
		}
	}

	m.IsDir = fields["类型"] == "目录"
	m.Name = fields["文件名称"]
	if m.Name == "" {
		m.Name = fields["目录名称"]
	}
	if m.Name == "" {
		m.Name = path.Base(remotePath)
	}

	// 文件大小形如 "2018, 1.970703KB"，逗号前是精确字节数。
	if v := fields["文件大小"]; v != "" {
		if i := strings.IndexByte(v, ','); i > 0 {
			if n, err := strconv.ParseInt(strings.TrimSpace(v[:i]), 10, 64); err == nil {
				m.Size = n
			}
		} else {
			m.Size = parseSize(v)
		}
	}
	m.ModTime = parseTime(fields["修改日期"])

	return m
}

// DownloadArgs 构造 download 子命令的参数列表（不含可执行文件本身）。
func DownloadArgs(remotePath, saveDir, mode string, threads int) []string {
	args := []string{"download", "--saveto", saveDir}
	if mode != "" {
		args = append(args, "--mode", mode)
	}
	if threads > 0 {
		args = append(args, "-p", strconv.Itoa(threads))
	}
	return append(args, normalizeRemote(remotePath))
}

// CommandLine 把参数列表拼成给人看的命令行，仅用于展示。
func CommandLine(cliPath string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, cliPath)
	for _, a := range args {
		if strings.ContainsAny(a, " \t\"'") {
			parts = append(parts, strconv.Quote(a))
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}

// run 执行一次 CLI 调用并返回合并后的输出。
func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	if c.CLIPath == "" {
		return "", errors.New("未配置 BaiduPCS-Go 路径")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.CLIPath, args...)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	out := buf.String()

	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("执行 %s 超时", strings.Join(args, " "))
	}
	// CLI 出错时也返回退出码 0，所以只有真正的启动失败（比如找不到可执行文件）
	// 才需要当成错误上报；业务错误交给调用方扫描输出文本。
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return out, fmt.Errorf("执行 %s: %w", c.CLIPath, err)
		}
	}
	return out, nil
}

// extractError 从输出里摘出 CLI 的错误描述，没有错误时返回空串。
func extractError(out string) string {
	i := strings.Index(out, errMarker)
	if i < 0 {
		return ""
	}
	msg := strings.TrimSpace(out[i:])
	if j := strings.IndexByte(msg, '\n'); j >= 0 {
		msg = strings.TrimSpace(msg[:j])
	}
	return msg
}

// parseTwoColumn 解析 meta 那种「键  值」两列的表格。
// 键本身可能含单个空格（例如 "md5 (截图请打码)"），所以按两个及以上空格切分。
func parseTwoColumn(out string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		// 必须先去掉行首缩进：否则第一段连续空格就是缩进本身，
		// 按它切分会把键切成空串，整行都被丢掉。
		line = strings.TrimLeft(line, " \t")
		if line == "" || strings.HasPrefix(line, "[") {
			continue
		}

		idx := multiSpaceRe.FindStringIndex(line)
		if idx == nil {
			continue
		}
		key := strings.TrimSpace(line[:idx[0]])
		val := strings.TrimSpace(line[idx[1]:])
		if key != "" && val != "" {
			fields[key] = val
		}
	}
	return fields
}

var multiSpaceRe = regexp.MustCompile(`\s{2,}`)

// parseSize 解析 "465.96MB" / "-" / "2018" 这类大小文本。单位是 1024 进制。
func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}

	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	if i == 0 {
		return 0
	}

	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0
	}

	var mult float64
	switch strings.ToUpper(strings.TrimSpace(s[i:])) {
	case "", "B":
		mult = 1
	case "KB":
		mult = 1 << 10
	case "MB":
		mult = 1 << 20
	case "GB":
		mult = 1 << 30
	case "TB":
		mult = 1 << 40
	default:
		return 0
	}
	// CLI 显示的大小是四舍五入过的（例如 2018 字节显示成 1.970703KB，
	// 乘回去是 2017.999872），所以这里也要四舍五入而不是直接截断。
	return int64(math.Round(num * mult))
}

// parseTime 把 "2015-12-12 18:46:12" 转成 RFC3339，失败时原样返回。
func parseTime(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local); err == nil {
		return t.Format(time.RFC3339)
	}
	return s
}

// NormalizeRemote 统一网盘路径：空串视为根目录，且总是以 "/" 开头。
func NormalizeRemote(p string) string { return normalizeRemote(p) }

// normalizeRemote 统一网盘路径：空串视为根目录，且总是以 "/" 开头。
func normalizeRemote(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}

// ParentRemote 返回网盘路径的上级目录，根目录返回它自己。
func ParentRemote(p string) string {
	p = normalizeRemote(p)
	if p == "/" {
		return "/"
	}
	return path.Dir(p)
}
