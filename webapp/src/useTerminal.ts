import { useEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import type { Progress } from './api'
import { appendSpeed, EMPTY_SPEED } from './speedSeries'
import type { SpeedSeries } from './speedSeries'
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
  /** 服务端推送的进度快照；还没解析到进度时为 null */
  progress: Progress | null
  /** 这个任务的下载速度曲线，连接时会带回后端已积累的历史 */
  speed: SpeedSeries
  /**
   * 有没有收到过任何终端输出。
   *
   * 用来区分「进程跑过、终端里有日志可看」和「本来就没输出」——后者（典型是服务重启后
   * 恢复出来的任务）不该白占一块黑窗口。
   */
  hasOutput: boolean
}

/**
 * 为指定任务维护一个 xterm 终端与它的 WebSocket。
 *
 * - 服务端发来的是二进制帧，用流式 TextDecoder 解码后再 write，避免多字节
 *   UTF-8 字符被拆帧时乱码。
 * - 连接建立时服务端会整体重放历史输出，所以切换任务 / 刷新页面都能看到完整内容。
 * - 进程结束时服务端主动关闭（code 1000），这是正常结束，不重连；只有异常断开才重连。
 * - taskId 或 runId 变化时销毁旧终端并新建，天然完成清屏。
 *
 * runId 用来区分「同一个任务的不同轮次」：点「继续」时后端会用同一个任务 ID 起一个
 * 新进程并替换掉旧的会话，旧连接早就以 1000 正常关闭了。只看 taskId 的话 effect 不会
 * 重跑，前端就还挂在那个死连接上——必须刷新页面才有内容。传 startedAt 进来当 runId
 * 就能在重启的瞬间自动重连。
 */
export function useTerminal(taskId: string | null, runId: string | null): UseTerminalResult {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const [state, setState] = useState<TerminalState>('idle')
  const [progress, setProgress] = useState<Progress | null>(null)
  const [speed, setSpeed] = useState<SpeedSeries>(EMPTY_SPEED)
  const [hasOutput, setHasOutput] = useState(false)

  useEffect(() => {
    const container = containerRef.current
    if (taskId === null || container === null) {
      setState('idle')
      setProgress(null)
      setSpeed(EMPTY_SPEED)
      setHasOutput(false)
      return
    }

    // 切换任务时先清掉上一个任务的进度，别让状态栏短暂显示错的数据。
    // 服务端在连接建立后会立刻推一次当前进度，很快就会补上。
    setProgress(null)
    setSpeed(EMPTY_SPEED)
    setHasOutput(false)

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
        // 和 index.css 的 --ink-0 / --fg-2 对齐，终端是全局最深的一层
        background: '#0a0d12',
        foreground: '#c8d3e2',
        cursor: '#35b8e0',
        selectionBackground: 'rgba(53, 184, 224, 0.28)',
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

        // 二进制帧是进程的原始输出，交给 xterm。
        if (event.data instanceof ArrayBuffer) {
          if (event.data.byteLength > 0) setHasOutput(true)
          term.write(decoder.decode(new Uint8Array(event.data), { stream: true }))
          return
        }

        // 文本帧是 JSON 控制消息：进度快照与速度历史。
        if (typeof event.data === 'string') {
          handleControlMessage(event.data, { setProgress, setSpeed })
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
  }, [taskId, runId])

  return { containerRef, state, progress, speed, hasOutput }
}

interface ControlHandlers {
  setProgress: (value: Progress | null) => void
  setSpeed: (updater: (prev: SpeedSeries) => SpeedSeries) => void
}

/**
 * 处理服务端发来的文本帧：
 *
 *   {"type":"progress","progress":{...},"t":1728660000000}
 *   {"type":"speedHistory","times":[...],"speeds":[...]}
 *
 * 解析失败就当普通文本忽略掉，绝不抛出去打断终端。
 */
function handleControlMessage(raw: string, handlers: ControlHandlers): void {
  let msg: unknown
  try {
    msg = JSON.parse(raw)
  } catch {
    return
  }

  if (msg === null || typeof msg !== 'object') return

  const { type } = msg as { type?: unknown }

  if (type === 'speedHistory') {
    const { times, speeds } = msg as { times?: unknown; speeds?: unknown }
    if (Array.isArray(times) && Array.isArray(speeds)) {
      handlers.setSpeed(() => ({ times: times as number[], speeds: speeds as number[] }))
    }
    return
  }

  if (type === 'progress') {
    const { progress, t } = msg as { progress?: unknown; t?: unknown }
    if (progress === null || typeof progress !== 'object') return

    const value = progress as Progress
    handlers.setProgress(value)

    // t 是后端采样时间，缺了就不往曲线上加点，免得时间轴错位。
    if (typeof t === 'number') {
      handlers.setSpeed((prev) => appendSpeed(prev, t, value.speed))
    }
  }
}
