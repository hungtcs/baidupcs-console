import type { Task } from '../api'
import { TaskRow } from './TaskRow'

interface TaskListProps {
  tasks: Task[]
  selectedId: string | null
  onSelect: (id: string) => void
}

export function TaskList({ tasks, selectedId, onSelect }: TaskListProps) {
  if (tasks.length === 0) {
    return (
      <div className="task-list-empty">
        <p className="empty-title">暂无下载任务</p>
        <p className="empty-hint">点击上方「新建下载」，从网盘选择文件开始下载。</p>
      </div>
    )
  }

  return (
    <div className="task-list">
      {tasks.map((task) => (
        <TaskRow
          key={task.id}
          task={task}
          selected={task.id === selectedId}
          onSelect={onSelect}
        />
      ))}
    </div>
  )
}
