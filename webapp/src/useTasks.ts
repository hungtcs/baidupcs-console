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

/** 列表轮询间隔：后端没有推送，2 秒拉一次。 */
const POLL_INTERVAL = 2000

/**
 * 任务列表里真正会变的字段的签名。轮询回来的数据和上一次一样时，
 * 直接跳过 setState，避免每 2 秒整棵列表重渲染导致闪动。
 */
function signature(list: Task[]): string {
  let out = ''
  for (const task of list) {
    out += `${task.id}\u0000${task.status}\u0000${task.pid}\u0000${task.exitCode}\u0000${task.finishedAt ?? ''}\u0001`
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

  const refresh = useCallback(async () => {
    try {
      const list = await getTasks()
      commit(list)
      setPollError(null)
    } catch (err) {
      setPollError(toErrorMessage(err))
    }
  }, [commit])

  useEffect(() => {
    let alive = true
    const tick = async () => {
      try {
        const list = await getTasks()
        if (!alive) return
        commit(list)
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
  }, [commit])

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

  return { tasks, error, dismissError, pollError, refresh, create, kill, restart, remove }
}
