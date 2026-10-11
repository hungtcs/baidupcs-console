import { useCallback, useEffect, useRef, useState } from 'react'
import {
  createTask,
  deleteTask,
  getTasks,
  killTask,
  restartTask,
  toErrorMessage,
} from './api'
import type { Task } from './api'
import { appendSpeed, EMPTY_SPEED, totalSpeed } from './speedSeries'
import type { SpeedSeries } from './speedSeries'

/** 列表轮询间隔：后端没有推送，2 秒拉一次。 */
const POLL_INTERVAL = 2000

/**
 * 任务列表里真正会变的字段的签名。轮询回来的数据和上一次一样时，
 * 直接跳过 setState，避免每 2 秒整棵列表重渲染导致闪动。
 */
function signature(list: Task[]): string {
  let out = ''
  for (const task of list) {
    const prog = task.progress
    out +=
      `${task.id}\u0000${task.status}\u0000${task.pid}\u0000${task.exitCode}\u0000${task.finishedAt ?? ''}` +
      // startedAt 是「这一轮运行」的标识：重启后它变了，终端据此重连。
      `\u0000${task.startedAt}` +
      `\u0000${task.resumable ? '1' : '0'}` +
      // 进度要参与签名，否则列表里的进度条永远不会更新。
      `\u0000${prog ? `${prog.doneText}|${prog.speedText}|${prog.percent}|${prog.filesDone}` : ''}` +
      '\u0001'
  }
  return out
}

export interface UseTasksResult {
  tasks: Task[]
  /** 操作（停止 / 重新开始 / 删除）失败的提示，供 Toast 使用 */
  error: string | null
  dismissError: () => void
  /** 最近一次轮询失败的原因；成功时清空 */
  pollError: string | null
  /** 合计速度曲线（所有运行中任务之和）。前端自己攒的，刷新页面后从头开始。 */
  speedSeries: SpeedSeries
  refresh: () => Promise<void>
  create: (remotePath: string) => Promise<Task>
  kill: (id: string) => Promise<void>
  restart: (id: string) => Promise<void>
  remove: (id: string) => Promise<void>
}

export function useTasks(): UseTasksResult {
  const [tasks, setTasks] = useState<Task[]>([])
  const [error, setError] = useState<string | null>(null)
  const [pollError, setPollError] = useState<string | null>(null)
  const [speedSeries, setSpeedSeries] = useState<SpeedSeries>(EMPTY_SPEED)

  const tasksRef = useRef<Task[]>([])
  const sigRef = useRef<string>('')

  /** 只有内容真正变化时才写 state。 */
  const commit = useCallback((list: Task[]) => {
    tasksRef.current = list
    const sig = signature(list)
    if (sig === sigRef.current) return
    sigRef.current = sig
    setTasks(list)
  }, [])

  // 合计速度只能在轮询拿到新数据时采一次样：后端没有「所有任务总和」这种东西，
  // 而列表本来就是每 2 秒轮一次，所以这条曲线也是大约 2 秒一个点。
  const recordSpeed = useCallback((list: Task[]) => {
    setSpeedSeries((prev) => appendSpeed(prev, Date.now(), totalSpeed(list)))
  }, [])

  const refresh = useCallback(async () => {
    try {
      const list = await getTasks()
      commit(list)
      recordSpeed(list)
      setPollError(null)
    } catch (err) {
      setPollError(toErrorMessage(err))
    }
  }, [commit, recordSpeed])

  useEffect(() => {
    let alive = true
    const tick = async () => {
      try {
        const list = await getTasks()
        if (!alive) return
        commit(list)
        recordSpeed(list)
        setPollError(null)
      } catch (err) {
        if (!alive) return
        setPollError(toErrorMessage(err))
      }
    }

    void tick()
    const timer = window.setInterval(() => void tick(), POLL_INTERVAL)
    return () => {
      alive = false
      window.clearInterval(timer)
    }
  }, [commit, recordSpeed])

  /** 把一次操作失败转成 Toast 文案。 */
  const run = useCallback(async (action: () => Promise<void>) => {
    try {
      await action()
    } catch (err) {
      setError(toErrorMessage(err))
    }
  }, [])

  const replace = useCallback(
    (task: Task) => {
      const list = tasksRef.current
      const index = list.findIndex((item) => item.id === task.id)
      commit(index === -1 ? [task, ...list] : list.map((item) => (item.id === task.id ? task : item)))
    },
    [commit],
  )

  const kill = useCallback(
    (id: string) => run(async () => replace(await killTask(id))),
    [run, replace],
  )

  const restart = useCallback(
    (id: string) => run(async () => replace(await restartTask(id))),
    [run, replace],
  )

  const remove = useCallback(
    (id: string) =>
      run(async () => {
        await deleteTask(id)
        commit(tasksRef.current.filter((item) => item.id !== id))
      }),
    [run, commit],
  )

  /** 创建失败要让对话框自己显示错误，所以这里不吞异常。 */
  const create = useCallback(
    async (remotePath: string) => {
      const task = await createTask(remotePath)
      replace(task)
      void refresh()
      return task
    },
    [replace, refresh],
  )

  const dismissError = useCallback(() => setError(null), [])

  return {
    tasks,
    error,
    dismissError,
    pollError,
    speedSeries,
    refresh,
    create,
    kill,
    restart,
    remove,
  }
}
