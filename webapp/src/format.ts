/**
 * 时间格式化工具。
 *
 * 进度的数字（大小、速度、剩余时间）一律用后端从 CLI 输出里解析出来的**原样文本**，
 * 不做二次格式化，所以这里只保留时间相关的工具。
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

const SPEED_UNITS = ['B', 'KB', 'MB', 'GB', 'TB'] as const

/**
 * 把「字节/秒」格式化成 `1.88MB/s` 这样，风格对齐 CLI（1024 进制、两位小数）。
 *
 * 只用于我们自己算出来的量（比如概览页的合计速度）——单个任务的速度一律直接显示
 * 后端解析出来的原样文本，不要走这里。
 */
export function formatSpeed(bytesPerSecond: number): string {
  if (!Number.isFinite(bytesPerSecond) || bytesPerSecond <= 0) return '0B/s'

  let value = bytesPerSecond
  let unit = 0
  while (value >= 1024 && unit < SPEED_UNITS.length - 1) {
    value /= 1024
    unit += 1
  }

  const digits = unit === 0 || value >= 100 ? 0 : 2
  return `${value.toFixed(digits)}${SPEED_UNITS[unit]}/s`
}


/**
 * 把毫秒数格式化成人话：`12 秒` / `2 分 14 秒` / `1 小时 3 分`。
 *
 * 只用于我们自己算出来的时长（任务的运行时长）。CLI 报的耗时一律直接显示它的原样文本。
 */
export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return '—'

  const total = Math.floor(ms / 1000)
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const seconds = total % 60

  if (hours > 0) return `${hours} 小时 ${minutes} 分`
  if (minutes > 0) return `${minutes} 分 ${seconds} 秒`
  return `${seconds} 秒`
}
