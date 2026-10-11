import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  LuCircleStop,
  LuDownload,
  LuLayoutDashboard,
  LuLoaderCircle,
  LuPlus,
  LuRotateCw,
  LuTerminal,
  LuTrash2,
  LuWifi,
  LuWifiOff,
} from 'react-icons/lu'
import type { IconType } from 'react-icons'
import { AddTaskDialog } from './components/AddTaskDialog'
import { Overview } from './components/Overview'
import { ProgressBar } from './components/ProgressBar'
import { SpeedChart } from './components/SpeedChart'
import { TaskList } from './components/TaskList'
import { Toast } from './components/Toast'
import { useTasks } from './useTasks'
import { useTerminal } from './useTerminal'
import type { TerminalState } from './useTerminal'
import './App.css'

interface ToastState {
  id: number
  text: string
  kind: 'info' | 'error'
}

/** 终端连接状态的显示文案与图标。 */
const TERMINAL_STATE: Record<TerminalState, { text: string; Icon: IconType; spin?: boolean }> = {
  idle: { text: '', Icon: LuTerminal },
  connecting: { text: '连接中', Icon: LuLoaderCircle, spin: true },
  open: { text: '已连接', Icon: LuWifi },
  reconnecting: { text: '重连中', Icon: LuWifiOff },
  closed: { text: '已结束', Icon: LuCircleStop },
}

function App() {
  const { tasks, error, dismissError, pollError, speedSeries, create, kill, restart, remove } =
    useTasks()

  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [showAdd, setShowAdd] = useState(false)
  const [toast, setToast] = useState<ToastState | null>(null)
  const toastId = useRef(0)

  const selectedTask = selectedId === null ? undefined : tasks.find((item) => item.id === selectedId)
  // 第二个参数把「同一个任务的不同轮次」区分开：点「继续」时任务 ID 不变、但 startedAt 会变，
  // 终端据此重连到新进程，不用刷新页面。
  const { containerRef, state: terminalState, progress, speed, hasOutput } = useTerminal(
    selectedTask?.id ?? null,
    selectedTask?.startedAt ?? null,
  )

  // 列表里的进度来自 2 秒一次的轮询；选中任务另外有一条每秒推送的进度（终端 WebSocket）。
  // 用后者覆盖一下，让正在看的那个任务跟手一些。
  const listTasks = useMemo(() => {
    if (progress === null || selectedId === null) return tasks
    return tasks.map((task) => (task.id === selectedId ? { ...task, progress } : task))
  }, [tasks, progress, selectedId])

  const showToast = useCallback((text: string, kind: 'info' | 'error' = 'info') => {
    toastId.current += 1
    setToast({ id: toastId.current, text, kind })
  }, [])

  const closeToast = useCallback(() => setToast(null), [])

  // 操作失败统一转成轻提示。
  useEffect(() => {
    if (error !== null) {
      showToast(error, 'error')
      dismissError()
    }
  }, [error, dismissError, showToast])

  const handleCreate = useCallback(
    async (remotePath: string) => {
      const task = await create(remotePath)
      setSelectedId(task.id)
      return task
    },
    [create],
  )

  const handleDelete = useCallback(
    (task: { id: string; name: string }) => {
      if (window.confirm(`确定删除「${task.name}」的任务记录吗？磁盘上已下载的文件不会删除。`)) {
        void remove(task.id)
      }
    },
    [remove],
  )

  // 跑着的任务一定显示终端（哪怕还没吐第一个字）；已经停下的任务只有在确实收到过
  // 输出时才显示，否则那块黑窗口毫无意义。
  const showTerminal = selectedTask !== undefined && (selectedTask.status === 'running' || hasOutput)

  const terminalInfo = TERMINAL_STATE[terminalState]
  const TerminalStateIcon = terminalInfo.Icon

  return (
    <div className="app">
      <aside className="sidebar">
        <header className="sidebar-header">
          <h1 className="sidebar-title">
            <LuDownload aria-hidden="true" />
            任务
            {tasks.length > 0 && <span className="sidebar-count">{tasks.length}</span>}
          </h1>
          <button
            type="button"
            className="btn btn-primary btn-sm"
            onClick={() => setShowAdd(true)}
          >
            <LuPlus aria-hidden="true" />
            新建下载
          </button>
        </header>

        <div className="sidebar-body">
          {/* 常驻入口：选中任务后得有办法回到概览 */}
          <button
            type="button"
            className={selectedId === null ? 'nav-item is-active' : 'nav-item'}
            onClick={() => setSelectedId(null)}
          >
            <LuLayoutDashboard aria-hidden="true" />
            概览
          </button>

          <TaskList tasks={listTasks} selectedId={selectedId} onSelect={setSelectedId} />
        </div>

        {pollError !== null && (
          <div className="sidebar-foot">
            <LuWifiOff aria-hidden="true" />
            <span>无法连接后端：{pollError}</span>
          </div>
        )}
      </aside>

      <main className="workspace">
        {selectedTask === undefined ? (
          <Overview
            tasks={tasks}
            speedSeries={speedSeries}
            onSelect={setSelectedId}
            onNewTask={() => setShowAdd(true)}
          />
        ) : (
          <>
            <header className="toolbar">
              <code className="command-line" title={selectedTask.commandLine}>
                {selectedTask.commandLine}
              </code>

              <div className="toolbar-actions">
                {selectedTask.status === 'running' ? (
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    onClick={() => void kill(selectedTask.id)}
                  >
                    <LuCircleStop aria-hidden="true" />
                    停止
                  </button>
                ) : (
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    onClick={() => void restart(selectedTask.id)}
                    // 磁盘上还有没下完的分片时，重跑会从断点接着下，所以叫「继续」；
                    // 什么都没有时会从头开始，那才是「重新开始」。
                    title={
                      selectedTask.resumable
                        ? '磁盘上还有未下完的分片，将从断点接着下载'
                        : '本地没有可续传的分片，将从头开始下载'
                    }
                  >
                    {selectedTask.resumable ? (
                      <LuLoaderCircle aria-hidden="true" />
                    ) : (
                      <LuRotateCw aria-hidden="true" />
                    )}
                    {selectedTask.resumable ? '继续' : '重新开始'}
                  </button>
                )}
                <button
                  type="button"
                  className="btn btn-ghost btn-sm btn-danger"
                  onClick={() => handleDelete(selectedTask)}
                >
                  <LuTrash2 aria-hidden="true" />
                  删除
                </button>
              </div>

              {terminalInfo.text !== '' && (
                <span className={`term-state term-${terminalState}`}>
                  <TerminalStateIcon
                    className={terminalInfo.spin === true ? 'is-spin' : undefined}
                    aria-hidden="true"
                  />
                  {terminalInfo.text}
                </span>
              )}
            </header>

            {/* 后端从终端输出里解析出来的进度；还没解析到就不显示这一条 */}
            {progress !== null && <ProgressBar progress={progress} />}

            <SpeedChart times={speed.times} speeds={speed.speeds} />

            <div className="terminal-pane">
              {/* 容器始终挂载（ref 要一直在），没输出时只是隐藏，避免白占一块黑窗口。
                  visibility 而不是 display，因为 FitAddon 需要能量到尺寸。 */}
              <div
                className={showTerminal ? 'terminal-host' : 'terminal-host is-hidden'}
                ref={containerRef}
              />

              {!showTerminal && (
                <div className="terminal-empty">
                  <LuTerminal className="empty-icon" aria-hidden="true" />
                  <p className="empty-title">没有终端输出</p>
                  <p className="empty-hint">
                    这次运行没有产生任何输出。服务重启后恢复的任务不会保留上次的输出，
                    点「继续」重新开始就能看到新的。
                  </p>
                </div>
              )}
            </div>
          </>
        )}
      </main>

      {showAdd && (
        <AddTaskDialog
          onClose={() => setShowAdd(false)}
          onCreate={handleCreate}
          onCreated={showToast}
        />
      )}

      {toast !== null && (
        <Toast key={toast.id} text={toast.text} kind={toast.kind} onClose={closeToast} />
      )}
    </div>
  )
}

export default App
