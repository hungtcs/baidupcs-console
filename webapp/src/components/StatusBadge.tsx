import type { TaskStatus } from '../api'

/**
 * 三个状态的中文文案。
 * 注意：exited 只表示进程自然退出，CLI 出错时退出码也是 0，
 * 成败要看终端输出，所以这里不写「已完成」。
 */
const STATUS_LABELS: Record<TaskStatus, string> = {
  running: '运行中',
  exited: '已结束',
  killed: '已停止',
}

export function StatusBadge({ status }: { status: TaskStatus }) {
  return (
    <span className={`badge badge-${status}`}>
      <span className="status-dot" aria-hidden="true" />
      {STATUS_LABELS[status]}
    </span>
  )
}
