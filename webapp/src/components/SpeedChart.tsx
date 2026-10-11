import { useEffect, useRef } from 'react'
import uPlot from 'uplot'
import 'uplot/dist/uPlot.min.css'
import { formatSpeed } from '../format'

interface SpeedChartProps {
  /** Unix 毫秒，与 speeds 一一对应 */
  times: number[]
  /** 字节/秒 */
  speeds: number[]
  height?: number
}

/** 只有两个点以上才值得画——一个点连不成线。 */
const MIN_POINTS = 2

/**
 * uPlot 的 React 包装。
 *
 * uPlot 没有官方 React 封装，但它就是「一个容器 + new uPlot(opts, data, el)」，
 * 包装起来很薄：实例建一次，数据变了调 setData，容器尺寸变了调 setSize。
 *
 * 数据要转成 uPlot 的格式：x 轴用 **秒**（不是毫秒，uPlot 的时间轴按秒算），
 * y 轴直接用字节/秒。
 */
export function SpeedChart({ times, speeds, height = 132 }: SpeedChartProps) {
  const hostRef = useRef<HTMLDivElement | null>(null)
  const plotRef = useRef<uPlot | null>(null)

  // 建实例。只跟 height 有关，数据变化走下面的 setData。
  useEffect(() => {
    const host = hostRef.current
    if (host === null) return

    const plot = new uPlot(buildOptions(host.clientWidth, height), [[], []], host)
    plotRef.current = plot

    // 容器宽度会随窗口和侧栏变化，用 ResizeObserver 跟着调。
    const observer = new ResizeObserver(() => {
      const width = host.clientWidth
      if (width > 0) plot.setSize({ width, height })
    })
    observer.observe(host)

    return () => {
      observer.disconnect()
      plot.destroy()
      plotRef.current = null
    }
  }, [height])

  // 数据变化时只更新数据，不重建实例（重建会丢缩放状态、也会闪）。
  useEffect(() => {
    const plot = plotRef.current
    if (plot === null) return
    plot.setData([times.map((ms) => ms / 1000), speeds])
  }, [times, speeds])

  if (times.length < MIN_POINTS) {
    return (
      <div className="speed-chart-empty" style={{ height }}>
        还没有足够的速度数据
      </div>
    )
  }

  return <div className="speed-chart" style={{ height }} ref={hostRef} />
}

function buildOptions(width: number, height: number): uPlot.Options {
  return {
    width: Math.max(width, 1),
    height,
    padding: [8, 8, 0, 0],
    legend: { show: false },
    // 曲线是「看当下」的，不需要十字光标和缩放，省得和终端抢注意力。
    cursor: { show: false },
    scales: {
      x: { time: true },
      // 速度不会为负，下界钉死 0；上界留空让 uPlot 自适应峰值。
      y: { range: (_u, _min, max) => [0, max > 0 ? max * 1.15 : 1] },
    },
    axes: [
      {
        stroke: '#6b7788',
        grid: { stroke: 'rgba(35, 43, 54, 0.7)', width: 1 },
        ticks: { stroke: 'rgba(35, 43, 54, 0.7)' },
        font: '11px system-ui, sans-serif',
        values: (_u, ticks) =>
          ticks.map((t) => new Date(t * 1000).toLocaleTimeString('zh-CN', { hour12: false })),
      },
      {
        stroke: '#6b7788',
        grid: { stroke: 'rgba(35, 43, 54, 0.7)', width: 1 },
        ticks: { stroke: 'rgba(35, 43, 54, 0.7)' },
        font: '11px system-ui, sans-serif',
        size: 56,
        // 轴上的数字走和别处一样的格式化，免得出现 1.2e+6 这种。
        values: (_u, ticks) => ticks.map((v) => formatSpeed(v)),
      },
    ],
    series: [
      {},
      {
        stroke: '#4d8dff',
        width: 2,
        fill: 'rgba(77, 141, 255, 0.14)',
        points: { show: false },
      },
    ],
  }
}
