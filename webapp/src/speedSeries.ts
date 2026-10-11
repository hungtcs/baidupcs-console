import type { Task } from './api'

/** 速度曲线保留的点数，和后端 `speedHistory` 的上限保持一致。 */
export const MAX_SPEED_POINTS = 300

/** 一条速度曲线的采样。时间轴统一用 Unix 毫秒。 */
export interface SpeedSeries {
  times: number[]
  speeds: number[]
}

export const EMPTY_SPEED: SpeedSeries = { times: [], speeds: [] }

/** 往曲线尾巴上加一个点，超出上限就从头部丢掉。 */
export function appendSpeed(prev: SpeedSeries, t: number, speed: number): SpeedSeries {
  const times = [...prev.times, t]
  const speeds = [...prev.speeds, speed]

  if (times.length <= MAX_SPEED_POINTS) return { times, speeds }

  const drop = times.length - MAX_SPEED_POINTS
  return { times: times.slice(drop), speeds: speeds.slice(drop) }
}

/** 当前所有运行中任务的速度之和，字节/秒。 */
export function totalSpeed(tasks: Task[]): number {
  let sum = 0
  for (const task of tasks) {
    if (task.status === 'running') sum += task.progress?.speed ?? 0
  }
  return sum
}
