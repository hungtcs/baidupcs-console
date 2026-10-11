import { useEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

/** 终端连接状态，用于工具栏上的小指示点。 */
export type TerminalState = 'idle' | 'connecting' | 'open' | 'reconnecting' | 'closed'

/** 异常断开后的重连退避上限。 */
const MAX_RECONNECT_DELAY = 10_000
/** 窗口 / 容器尺寸变化后的 fit 防抖。 */
const FIT_DEBOUNCE = 120

export interface UseTerminalResult {
  /** 挂载 xterm 的容器，直接绑到 <div ref={containerRef} /> 上 */
  containerRef: RefObject<HTMLDivElement | null>
  state: TerminalState
}

/**
 * 为指定任务维护一个 xterm 终端与它的 WebSocket。
 *
 * - 服务端发来的是二进制帧，用流式 TextDecoder 解码后再 write，避免多字节
 *   UTF-8 字符被拆帧时乱码。
 * - 连接建立时服务端会整体重放历史输出，所以切换任务 / 刷新页面都能看到完整内容。
 * - 进程结束时服务端主动关闭（code 1000），这是正常结束，不重连；只有异常断开才重连。
 * - taskId 变化时销毁旧终端并新建，天然完成清屏。
 */
export function useTerminal(taskId: string | null): UseTerminalResult {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const [state, setState] = useState<TerminalState>('idle')

  useEffect(() => {
    const container = containerRef.current
    if (taskId === null || container === null) {
      setState('idle')
      return
    }

    const term = new Terminal({
      convertEol: false,
      cursorBlink: false,
      // 界面没有输入入口，不让用户误输入。
      disableStdin: true,
      fontFamily:
        'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace',
      fontSize: 12.5,
      lineHeight: 1.25,
      scrollback: 10_000,
      theme: {
        background: '#0a0d12',
        foreground: '#c8d3e2',
        cursor: '#4d8dff',
        selectionBackground: 'rgba(77, 141, 255, 0.3)',
        black: '#0a0d12',
        red: '#f87171',
        green: '#34d399',
        yellow: '#fbbf24',
        blue: '#4d8dff',
        magenta: '#a78bfa',
        cyan: '#38bdf8',
        white: '#c8d3e2',
        brightBlack: '#6b7788',
        brightRed: '#fca5a5',
        brightGreen: '#6ee7b7',
        brightYellow: '#fcd34d',
        brightBlue: '#7caeff',
        brightMagenta: '#c4b5fd',
        brightCyan: '#7dd3fc',
        brightWhite: '#e7ecf3',
      },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(container)

    let fitRaf = 0
    const doFit = () => {
      try {
        fit.fit()
      } catch {
        // 容器尺寸为 0（例如刚挂载）时忽略，后续 resize 会补上。
      }
    }
    // 等浏览器完成一次布局后再 fit，否则可能量到 0 尺寸。
    fitRaf = window.requestAnimationFrame(doFit)

    let fitTimer: number | null = null
    const scheduleFit = () => {
      if (fitTimer !== null) window.clearTimeout(fitTimer)
      fitTimer = window.setTimeout(() => {
        fitTimer = null
        doFit()
      }, FIT_DEBOUNCE)
    }
    window.addEventListener('resize', scheduleFit)
    // 左栏宽度变化、窗口变化都会触发；observe 时还会立刻回调一次。
    const observer = new ResizeObserver(scheduleFit)
    observer.observe(container)

    let socket: WebSocket | null = null
    let disposed = false
    let attempt = 0
    let reconnectTimer: number | null = null

    const scheduleReconnect = () => {
      if (disposed) return
      setState('reconnecting')
      const delay = Math.min(1000 * 2 ** attempt, MAX_RECONNECT_DELAY)
      attempt += 1
      reconnectTimer = window.setTimeout(connect, delay)
    }

    const connect = () => {
      if (disposed) return
      setState(attempt === 0 ? 'connecting' : 'reconnecting')

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const url = `${protocol}//${window.location.host}/api/tasks/${encodeURIComponent(taskId)}/terminal`

      // 每条连接都用新的解码器：重连时服务端会从头重放，若沿用上一条连接
      // 残留的半截多字节 UTF-8，会把重放内容的最前面几个字解码坏。
      const decoder = new TextDecoder('utf-8')

      let ws: WebSocket
      try {
        ws = new WebSocket(url)
      } catch {
        scheduleReconnect()
        return
      }
      ws.binaryType = 'arraybuffer'
      socket = ws

      ws.onopen = () => {
        if (disposed) return
        attempt = 0
        setState('open')
        doFit()
      }

      ws.onmessage = (event: MessageEvent) => {
        if (disposed) return
        if (event.data instanceof ArrayBuffer) {
          term.write(decoder.decode(new Uint8Array(event.data), { stream: true }))
        }
      }

      ws.onerror = () => {
        // 出错后浏览器会继续触发 onclose，交给它处理重连。
      }

      ws.onclose = (event: CloseEvent) => {
        if (disposed) return
        if (event.code === 1000) {
          // 进程正常结束，服务端主动关闭，不重连。
          setState('closed')
          return
        }
        scheduleReconnect()
      }
    }

    connect()

    return () => {
      disposed = true
      window.cancelAnimationFrame(fitRaf)
      if (fitTimer !== null) window.clearTimeout(fitTimer)
      if (reconnectTimer !== null) window.clearTimeout(reconnectTimer)
      window.removeEventListener('resize', scheduleFit)
      observer.disconnect()
      if (socket !== null) {
        socket.onopen = null
        socket.onmessage = null
        socket.onerror = null
        socket.onclose = null
        if (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING) {
          socket.close()
        }
      }
      term.dispose()
    }
  }, [taskId])

  return { containerRef, state }
}
