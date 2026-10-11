/**
 * 时间格式化工具。
 *
 * 任务里没有 progress / speed / eta 字段，进度由 xterm 直接渲染 CLI 输出，
 * 所以这里只保留时间相关的格式化。
 */

/** RFC3339 时间转本地展示；空值或非法值返回占位符。 */
export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString('zh-CN', { hour12: false })
}

/** 只要时分秒，例如 21:46:03，用于列表里的紧凑展示。 */
export function formatClock(iso: string | null | undefined): string {
  if (!iso) return '—'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleTimeString('zh-CN', { hour12: false })
}

