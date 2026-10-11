package pcs

import (
	"bytes"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Progress 是从 download 输出里解析出来的进度快照。
//
// 目录下载时 CLI 会逐文件打印，且事先不告诉目录总大小，所以这里以
// 「当前正在下载的那个文件」为准，另外给出已完成/已知的文件数。
// 单文件任务下 FilesTotal 就是 1，可以当作普通进度显示。
//
// 所有字段都可比较（没有切片/映射），调用方可以直接用 == 判断有没有变化。
type Progress struct {
	// 当前正在下载的文件
	CurrentName string  `json:"currentName"`
	DoneText    string  `json:"doneText"`  // 原样，如 "1.88MB"
	TotalText   string  `json:"totalText"` // 原样，如 "139.20MB"
	DoneBytes   int64   `json:"doneBytes"`
	TotalBytes  int64   `json:"totalBytes"`
	Percent     float64 `json:"percent"` // 0-100；总量未知时为 0
	SpeedText   string  `json:"speedText"`
	Speed       int64   `json:"speed"` // 字节/秒
	ElapsedText string  `json:"elapsedText"`
	EtaText     string  `json:"etaText"` // 原样，未知时是 "-"

	// 目录下载时的文件计数
	FilesDone  int `json:"filesDone"`
	FilesTotal int `json:"filesTotal"`

	// Finished 表示看到过「下载结束」
	Finished bool `json:"finished"`
}

// 进度行原样长这样（实测 v4.0.2，圆点固定 12 个，不随终端宽度变化）：
//
//	[1] ↓ 128.00KB/139.20MB 213.21KB/s in 1s, left 11m7s ............
//	[1] ↓ 1.88MB/139.20MB 0B/s in 55s, left - ............
var progressLineRe = regexp.MustCompile(
	`^\[(\d+)\]\s+[↓↑]\s+([^/\s]+)/(\S+)\s+(\S+)\s+in\s+(\S+),\s+left\s+(\S+)`)

// [17] 准备下载: /我的资源/.../adx.gif
var prepareLineRe = regexp.MustCompile(`^\[(\d+)\]\s+准备下载:\s*(.+)$`)

// [17] 下载完成, 保存位置: /home/...
var fileDoneLineRe = regexp.MustCompile(`^\[(\d+)\]\s+下载完成`)

// maxPartialLine 限制「还没等到分隔符」的缓冲长度。进度行每秒都会以 \r 收尾，
// 正常不会积压；这个上限只是防止异常输入把内存撑爆。
const maxPartialLine = 64 << 10

// fileState 是单个文件（对应进度行里的 [N]）的进度。
type fileState struct {
	name        string
	doneText    string
	totalText   string
	speedText   string
	elapsedText string
	etaText     string
	doneBytes   int64
	totalBytes  int64
	done        bool
}

// ProgressParser 以流式方式解析 download 的输出。
//
// 输出是按块到达的，一块可能在半行处断开，所以内部会缓存未完成的那一段，
// 等下一个分隔符到了再处理。
//
// 本类型不是并发安全的，调用方需要保证串行访问（session 是在自己的锁下调用它）。
type ProgressParser struct {
	buf      []byte
	files    map[int]*fileState
	current  int // 最近一次出现进度行的文件索引
	hasData  bool
	finished bool
}

// NewProgressParser 创建一个解析器。
func NewProgressParser() *ProgressParser {
	return &ProgressParser{files: make(map[int]*fileState)}
}

// Write 喂入一段原始输出字节。
func (p *ProgressParser) Write(chunk []byte) {
	p.buf = append(p.buf, chunk...)

	for {
		i := bytes.IndexAny(p.buf, "\r\n")
		if i < 0 {
			if len(p.buf) > maxPartialLine {
				p.buf = p.buf[len(p.buf)-maxPartialLine:]
			}
			return
		}

		line := string(p.buf[:i])
		// 重新切片而不是重新分配：append 会在需要时自动搬走仍然存活的部分。
		p.buf = p.buf[i+1:]
		p.handleLine(line)
	}
}

func (p *ProgressParser) handleLine(line string) {
	line = strings.TrimRight(line, " \t")
	if line == "" {
		return
	}

	if m := progressLineRe.FindStringSubmatch(line); m != nil {
		p.applyProgressLine(m)
		return
	}
	if m := prepareLineRe.FindStringSubmatch(line); m != nil {
		if idx, err := strconv.Atoi(m[1]); err == nil {
			p.file(idx).name = path.Base(strings.TrimSpace(m[2]))
		}
		return
	}
	if m := fileDoneLineRe.FindStringSubmatch(line); m != nil {
		if idx, err := strconv.Atoi(m[1]); err == nil {
			f := p.file(idx)
			f.done = true
			// 最后一个进度行可能还没到 100%，这里直接按完成处理。
			if f.totalBytes > 0 {
				f.doneBytes = f.totalBytes
				f.doneText = f.totalText
			}
		}
		return
	}
	if strings.Contains(line, "下载结束") {
		p.finished = true
	}
}

func (p *ProgressParser) applyProgressLine(m []string) {
	idx, err := strconv.Atoi(m[1])
	if err != nil {
		return
	}

	f := p.file(idx)
	f.doneText = m[2]
	f.totalText = m[3]
	f.speedText = m[4]
	f.elapsedText = m[5]
	f.etaText = m[6]
	f.doneBytes = parseSize(m[2])
	f.totalBytes = parseSize(m[3])

	p.current = idx
	p.hasData = true
}

// applyPartial 处理还没等到分隔符的最后一段。
//
// 进度行是先写内容、下一条行首才补 \r，所以最新那条进度行在缓冲区里往往
// 一直没有收尾；只处理已终结的行会永远慢一拍。这里对它再试一次。
//
// 只按进度行正则试：那个正则要求尾部完整（"... in 1s, left 11m7s"），
// 半截的行匹配不上，值不会错。而「准备下载: <路径>」这类行如果被截断，
// 会得到一个错误的文件名，所以不在这里处理。
//
// 重复解析是安全的：同一行等分隔符到了会再走一次 handleLine，结果相同。
func (p *ProgressParser) applyPartial() {
	if len(p.buf) == 0 {
		return
	}
	if m := progressLineRe.FindStringSubmatch(strings.TrimRight(string(p.buf), " \t")); m != nil {
		p.applyProgressLine(m)
	}
}

// file 取（必要时创建）某个索引的状态。
func (p *ProgressParser) file(idx int) *fileState {
	f, ok := p.files[idx]
	if !ok {
		f = &fileState{}
		p.files[idx] = f
	}
	return f
}

// Snapshot 返回当前进度的快照。还没解析到任何进度时返回 nil。
func (p *ProgressParser) Snapshot() *Progress {
	p.applyPartial()

	if !p.hasData {
		return nil
	}

	f, ok := p.files[p.current]
	if !ok {
		return nil
	}

	done := 0
	for _, v := range p.files {
		if v.done {
			done++
		}
	}

	prog := &Progress{
		CurrentName: f.name,
		DoneText:    f.doneText,
		TotalText:   f.totalText,
		DoneBytes:   f.doneBytes,
		TotalBytes:  f.totalBytes,
		SpeedText:   f.speedText,
		Speed:       parseSpeed(f.speedText),
		ElapsedText: f.elapsedText,
		EtaText:     f.etaText,
		FilesDone:   done,
		FilesTotal:  len(p.files),
		Finished:    p.finished,
	}

	if f.totalBytes > 0 {
		prog.Percent = float64(f.doneBytes) / float64(f.totalBytes) * 100
		if prog.Percent > 100 {
			prog.Percent = 100
		}
	}

	return prog
}

// parseSpeed 解析 "100.99KB/s" / "0B/s" 这类速度文本。单位同样是 1024 进制。
func parseSpeed(s string) int64 {
	s = strings.TrimSuffix(strings.TrimSpace(s), "/s")
	return parseSize(s)
}
