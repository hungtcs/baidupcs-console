import type { Task } from '../api'
import { formatClock, formatDateTime } from '../format'
import { StatusBadge } from './StatusBadge'

interface TaskRowProps {
  task: Task
  selected: boolean
  onSelect: (id: string) => void
}

/** 左侧列表里的一项：名称 + 状态，点击切换右侧终端。 */
export function TaskRow({ task, selected, onSelect }: TaskRowProps) {
  return (
    <button
      type="button"
      className={selected ? 'task-row is-selected' : 'task-row'}
      onClick={() => onSelect(task.id)}
      title={task.remotePath}
    >
      <span className="task-row-name">{task.name}</span>
      <span className="task-row-meta">
        <StatusBadge status={task.status} />
        <span className="task-row-time" title={`开始于 ${formatDateTime(task.startedAt)}`}>
          {formatClock(task.startedAt)}
        </span>
      </span>
    </button>
  )
}
