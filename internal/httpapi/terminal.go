package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"

	"github.com/hungtcs/baidupcs-console/internal/pcs"
	"github.com/hungtcs/baidupcs-console/internal/session"
)

// 局域网自用、不鉴权，因此不做来源校验。若将来要暴露到公网，
// 这里必须改成白名单，否则任意站点都能连上这个终端。
var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(*http.Request) bool { return true },
}

const (
	// 客户端静默多久算掉线。浏览器会自动回 pong，所以这个值可以放得比较宽。
	terminalReadTimeout = 3 * time.Minute
	// 服务端主动 ping 的间隔。
	terminalPingInterval = 30 * time.Second
	// 进度推送间隔。CLI 自己的进度条也是每秒刷新，对上即可。
	progressInterval = 1 * time.Second
	// 单条客户端消息的大小上限，防止被灌爆内存。
	terminalReadLimit = 1 << 20
)

// taskTerminal 把任务的原始输出通过 WebSocket 推给前端。
//
// 协议：
//   - 服务端 → 客户端，**二进制帧**：进程 stdout/stderr 的原始字节，
//     前端直接丢给 xterm 的 write()。\r 覆盖与 ANSI 转义都由 xterm 处理。
//   - 服务端 → 客户端，**文本帧**：JSON，目前只有解析出来的进度
//     `{"type":"progress","progress":{...}}`。
//   - 客户端 → 服务端：二进制帧写进进程 stdin；文本帧预留作控制消息。
//
// 连接建立时会先把已积累的输出整体重放一遍，所以刷新页面或中途连上都能看到完整历史。
func (s *Server) taskTerminal(c *echo.Context) error {
	sess, err := s.manager.Get(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		// Upgrade 失败时它自己已经写过响应了，这里不能再写。
		return nil
	}

	replay, chunks, unsubscribe := sess.Subscribe()
	defer unsubscribe()

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		writeTerminal(conn, sess, replay, chunks)
	}()

	readTerminal(conn, sess)

	// 读循环退出说明客户端断开或出错，关掉连接让写协程也结束。
	_ = conn.Close()
	<-writerDone

	return nil
}

// progressMessage 是文本帧里推给前端的进度消息。
//
// T 是这条采样产生的时间（Unix 毫秒）。前端把 (T, speed) 追加到速度曲线尾巴上，
// 这样时间轴和连接时下发的历史用的是同一个时钟，不会错位。
type progressMessage struct {
	Type     string        `json:"type"`
	Progress *pcs.Progress `json:"progress"`
	T        int64         `json:"t"`
}

// speedHistoryMessage 在连接建立时下发一份已有的速度采样，用列式存放
// （uPlot 的数据格式本来就是 [xs, ys] 两列）。
type speedHistoryMessage struct {
	Type   string  `json:"type"`
	Times  []int64 `json:"times"`
	Speeds []int64 `json:"speeds"`
}

// writeTerminal 负责所有对连接的写操作：先重放历史，再持续推送新输出与进度。
//
// 进度是每秒从任务快照里读一次、有变化才发。CLI 自己的进度条也是每秒刷新，
// 所以这个节奏刚好对上，不会比终端内容更慢。
func writeTerminal(conn *websocket.Conn, sess *session.Session, replay []byte, chunks <-chan []byte) {
	if len(replay) > 0 {
		if err := conn.WriteMessage(websocket.BinaryMessage, replay); err != nil {
			return
		}
	}

	// 连上先把已有的速度采样整体下发，之后每秒的进度推送会往尾巴上追加。
	// 这样中途切回来、刷新页面，曲线都是接得上的。
	if !sendSpeedHistory(conn, sess) {
		return
	}

	var sent *pcs.Progress
	// 连上就先发一次当前进度，中途连进来的客户端不用等下一拍。
	if !sendProgress(conn, sess, &sent) {
		return
	}

	ping := time.NewTicker(terminalPingInterval)
	progressTick := time.NewTicker(progressInterval)
	defer ping.Stop()
	defer progressTick.Stop()

	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				// 进程结束，通道被关闭。收尾前再发一次最终进度。
				sendProgress(conn, sess, &sent)
				_ = conn.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, "进程已结束"),
					time.Now().Add(time.Second),
				)
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, chunk); err != nil {
				return
			}

		case <-progressTick.C:
			if !sendProgress(conn, sess, &sent) {
				return
			}

		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}
}

// sendProgress 在进度有变化时发一条文本帧。返回 false 表示连接已不可用。
func sendProgress(conn *websocket.Conn, sess *session.Session, sent **pcs.Progress) bool {
	prog := sess.Task().Progress
	if prog == nil {
		return true
	}
	if *sent != nil && **sent == *prog {
		return true
	}
	*sent = prog

	data, err := json.Marshal(progressMessage{
		Type:     "progress",
		Progress: prog,
		T:        time.Now().UnixMilli(),
	})
	if err != nil {
		return true
	}
	return conn.WriteMessage(websocket.TextMessage, data) == nil
}

// sendSpeedHistory 把已有的速度采样一次性发过去。返回 false 表示连接已不可用。
func sendSpeedHistory(conn *websocket.Conn, sess *session.Session) bool {
	times, speeds := sess.SpeedHistory()
	if len(times) == 0 {
		return true
	}

	// 空的列要序列化成 [] 而不是 null，否则前端得处处判空。
	data, err := json.Marshal(speedHistoryMessage{
		Type:   "speedHistory",
		Times:  times,
		Speeds: speeds,
	})
	if err != nil {
		return true
	}
	return conn.WriteMessage(websocket.TextMessage, data) == nil
}

// readTerminal 处理客户端发来的内容，直到连接断开。
func readTerminal(conn *websocket.Conn, sess interface {
	WriteStdin([]byte) error
}) {
	conn.SetReadLimit(terminalReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(terminalReadTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(terminalReadTimeout))
	})

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		// 只有二进制帧会被当成 stdin。文本帧留给后续的 JSON 控制消息。
		if msgType == websocket.BinaryMessage {
			if err := sess.WriteStdin(data); err != nil {
				return
			}
		}
	}
}
