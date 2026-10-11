package httpapi

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"
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
	// 单条客户端消息的大小上限，防止被灌爆内存。
	terminalReadLimit = 1 << 20
)

// taskTerminal 把任务的原始输出通过 WebSocket 推给前端。
//
// 协议：
//   - 服务端 → 客户端：二进制帧，内容是进程 stdout/stderr 的原始字节，
//     前端直接丢给 xterm 的 write()。\r 覆盖与 ANSI 转义都由 xterm 处理。
//   - 客户端 → 服务端：二进制帧写进进程 stdin；文本帧是 JSON 控制消息（暂未使用）。
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
		writeTerminal(conn, replay, chunks)
	}()

	readTerminal(conn, sess)

	// 读循环退出说明客户端断开或出错，关掉连接让写协程也结束。
	_ = conn.Close()
	<-writerDone

	return nil
}

// writeTerminal 负责所有对连接的写操作：先重放历史，再持续推送新输出。
func writeTerminal(conn *websocket.Conn, replay []byte, chunks <-chan []byte) {
	if len(replay) > 0 {
		if err := conn.WriteMessage(websocket.BinaryMessage, replay); err != nil {
			return
		}
	}

	ping := time.NewTicker(terminalPingInterval)
	defer ping.Stop()

	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				// 进程结束，通道被关闭。
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

		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}
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
