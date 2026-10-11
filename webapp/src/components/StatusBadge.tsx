import { LuCircleStop, LuLoaderCircle, LuSquare } from 'react-icons/lu'
import type { IconType } from 'react-icons'
import type { TaskStatus } from '../api'

/**
 * 三个状态的文案与图标。
 *
 * 注意：exited 只表示进程自然退出，**CLI 出错时退出码也是 0**，成败要看终端输出，
 * 所以这里不用勾号之类的「成功」语义，一律用「停下来」的图标 + 中性文案。
 */
const STATUS: Record<TaskStatus, { label: string; Icon: IconType; spin?: boolean }> = {
  running: { label: '运行中', Icon: LuLoaderCircle, spin: true },
  exited: { label: '已结束', Icon: LuCircleStop },
  killed: { label: '已停止', Icon: LuSquare },
}

export function StatusBadge({ status }: { status: TaskStatus }) {
  const { label, Icon, spin } = STATUS[status]

  return (
    <span className={`badge badge-${status}`}>
      <Icon className={spin === true ? 'badge-icon is-spin' : 'badge-icon'} aria-hidden="true" />
      {label}
    </span>
  )
}
