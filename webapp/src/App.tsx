import { useCallback, useEffect, useRef, useState } from 'react'
import { AddTaskDialog } from './components/AddTaskDialog'
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

const TERMINAL_STATE_TEXT: Record<TerminalState, string> = {
  idle: '',
  connecting: '连接中',
  open: '已连接',
  reconnecting: '重连中',
  closed: '已结束',
}

function App() {
  const { tasks, error, dismissError, pollError, create, kill, restart, remove } = useTasks()

  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [showAdd, setShowAdd] = useState(false)
  const [toast, setToast] = useState<ToastState | null>(null)
  const toastId = useRef(0)

  const selectedTask = selectedId === null ? undefined : tasks.find((item) => item.id === selectedId)
  const { containerRef, state: terminalState } = useTerminal(selectedTask?.id ?? null)

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

  return (
    <div className="app">
      <aside className="sidebar">
        <header className="sidebar-header">
          <h1 className="sidebar-title">任务</h1>
          <button
            type="button"
            className="btn btn-primary btn-sm"
            onClick={() => setShowAdd(true)}
          >
            ＋ 新建下载
          </button>
        </header>

        <div className="sidebar-body">
          <TaskList tasks={tasks} selectedId={selectedId} onSelect={setSelectedId} />
        </div>

        {pollError !== null && <div className="sidebar-foot">无法连接后端：{pollError}</div>}
      </aside>

      <main className="workspace">
        {selectedTask === undefined ? (
          <div className="workspace-empty">
            <p className="empty-title">未选择任务</p>
            <p className="empty-hint">从左侧选择一个任务，这里会显示它的实时终端输出。</p>
          </div>
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
                    停止
                  </button>
                ) : (
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    onClick={() => void restart(selectedTask.id)}
                  >
                    重新开始
                  </button>
                )}
                <button
                  type="button"
                  className="btn btn-ghost btn-sm btn-danger"
                  onClick={() => handleDelete(selectedTask)}
                >
                  删除
                </button>
              </div>

              {TERMINAL_STATE_TEXT[terminalState] !== '' && (
                <span className={`term-state term-${terminalState}`}>
                  <span className="term-state-dot" />
                  {TERMINAL_STATE_TEXT[terminalState]}
                </span>
              )}
            </header>

            <div className="terminal-pane">
              <div className="terminal-host" ref={containerRef} />
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
