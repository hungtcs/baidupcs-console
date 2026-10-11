import type { Progress, Task } from '../api'
import { renderFileIcon } from '../fileIcon'
import { formatClock, formatDateTime } from '../format'
import { StatusBadge } from './StatusBadge'

interface TaskRowProps {
  task: Task
  selected: boolean
  onSelect: (id: string) => void
}

function clampPercent(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, value))
}

/** 左侧列表里的一项：图标 + 名称 + 状态，有进度时再加一条细进度条。 */
export function TaskRow({ task, selected, onSelect }: TaskRowProps) {
  const { progress } = task

  return (
    <button
      type="button"
      className={selected ? 'task-row is-selected' : 'task-row'}
      onClick={() => onSelect(task.id)}
      title={task.remotePath}
    >
      <div className="task-row-head">
        {renderFileIcon(
          task.name,
          task.isDir,
          task.isDir ? 'task-row-icon is-dir' : 'task-row-icon',
        )}
        <span className="task-row-name">{task.name}</span>
        <StatusBadge status={task.status} />
      </div>

      {progress !== undefined ? (
        <TaskRowProgress progress={progress} />
      ) : (
        <div className="task-row-foot">
          <span title={`开始于 ${formatDateTime(task.startedAt)}`}>
            {formatClock(task.startedAt)}
          </span>
          {task.isDir && <span className="task-row-tag">目录</span>}
        </div>
      )}
    </button>
  )
}

/** 列表里的迷你进度条。数据来自列表轮询，所以大约 2 秒才动一次。 */
function TaskRowProgress({ progress }: { progress: Progress }) {
  const known = progress.totalBytes > 0
  const percent = clampPercent(progress.percent)

  return (
    <div className="task-row-progress">
      <div className="task-row-track">
        <div
          className={progress.finished ? 'task-row-fill is-done' : 'task-row-fill'}
          style={{ width: known ? `${percent}%` : '0%' }}
        />
      </div>

      <div className="task-row-stats">
        {known && <span className="task-row-percent">{percent.toFixed(1)}%</span>}
        <span className="task-row-bytes">
          {progress.doneText} / {progress.totalText}
        </span>
        {progress.speedText !== '' && <span className="task-row-speed">{progress.speedText}</span>}
        {progress.filesTotal > 1 && (
          <span className="task-row-tag">已完成 {progress.filesDone} 个文件</span>
        )}
      </div>
    </div>
  )
}
