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

interface PlotProps extends SpeedChartProps {
  height: number
}

/** 只有两个点以上才值得画——一个点连不成线。 */
const MIN_POINTS = 2

const DEFAULT_HEIGHT = 132

/**
 * 速度曲线。
 *
 * **数据不足时必须在挂载之前就决定不画**，所以这里拆成两层：外层判空，内层 `Plot`
 * 只在确定有数据时才被挂载。
 *
 * 不能把判断放在 `Plot` 里用条件 return：uPlot 实例是在 effect 里用 ref 建的，
 * 若组件首次渲染返回的是占位 div（没有图表容器），effect 首次运行就取不到 ref 而
 * 直接返回；依赖数组里只有 height，之后也不会重跑，图就永远出不来。
 */
export function SpeedChart({ times, speeds, height = DEFAULT_HEIGHT }: SpeedChartProps) {
  if (times.length < MIN_POINTS) {
    return (
      <div className="speed-chart-empty" style={{ height }}>
        还没有足够的速度数据
      </div>
    )
  }

  return <Plot times={times} speeds={speeds} height={height} />
}

/**
 * 真正画图的部分。只有确定有数据时才会被挂载。
 *
 * uPlot 没有官方 React 封装，但它就是「一个容器 + new uPlot(opts, data, el)」，
 * 包装起来很薄：实例建一次，数据变了调 setData，容器尺寸变了调 setSize。
 */
function Plot({ times, speeds, height }: PlotProps) {
  const hostRef = useRef<HTMLDivElement | null>(null)
  const plotRef = useRef<uPlot | null>(null)

  // 建实例。只跟 height 有关，数据变化走下面的 setData。
  useEffect(() => {
    const host = hostRef.current
    if (host === null) return

    const plot = new uPlot(buildOptions(host.clientWidth, height), toUplotData(times, speeds), host)
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
    // times/speeds 只在首次渲染时用一次，后续靠下面的 setData 更新；
    // 放进依赖会导致每次数据变化都重建实例（丢缩放、还会闪）。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [height])

  // 数据变化时只更新数据，不重建实例（重建会丢缩放状态、也会闪）。
  useEffect(() => {
    plotRef.current?.setData(toUplotData(times, speeds))
  }, [times, speeds])

  return <div className="speed-chart" style={{ height }} ref={hostRef} />
}

/**
 * 转成 uPlot 的数据格式：`[xs, ys]` 两列。x 轴用**秒**（不是毫秒）——
 * uPlot 的时间轴按秒算。
 *
 * 两列长度不一致时按短的截断：uPlot 对列长不齐会直接报错，宁可少画一点也不崩。
 */
function toUplotData(times: number[], speeds: number[]): uPlot.AlignedData {
  const n = Math.min(times.length, speeds.length)
  const xs = new Array<number>(n)
  const ys = new Array<number>(n)

  for (let i = 0; i < n; i += 1) {
    xs[i] = times[i] / 1000
    ys[i] = speeds[i]
  }

  return [xs, ys]
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
        stroke: '#5f6b7c',
        grid: { stroke: 'rgba(33, 41, 54, 0.9)', width: 1 },
        ticks: { stroke: 'rgba(33, 41, 54, 0.9)' },
        font: '11px system-ui, sans-serif',
        values: (_u, ticks) =>
          ticks.map((t) => new Date(t * 1000).toLocaleTimeString('zh-CN', { hour12: false })),
      },
      {
        stroke: '#5f6b7c',
        grid: { stroke: 'rgba(33, 41, 54, 0.9)', width: 1 },
        ticks: { stroke: 'rgba(33, 41, 54, 0.9)' },
        font: '11px system-ui, sans-serif',
        size: 56,
        // 轴上的数字走和别处一样的格式化，免得出现 1.2e+6 这种。
        values: (_u, ticks) => ticks.map((v) => formatSpeed(v)),
      },
    ],
    series: [
      {},
      {
        stroke: '#35b8e0',
        width: 2,
        fill: 'rgba(53, 184, 224, 0.12)',
        points: { show: false },
      },
    ],
  }
}
