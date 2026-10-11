/**
 * 后端 API 契约对应的类型定义与请求封装。
 * 契约见 docs/API.md，无鉴权，错误统一为 { "error": "<message>" }。
 *
 * 注意：任务对象里没有 progress / speed / eta 字段，后端不解析 CLI 输出，
 * 进度与成败都在终端文本里，由 xterm 展示。
 */

export type TaskStatus = 'running' | 'exited' | 'killed'

export interface Task {
  id: string
  /** 展示名 */
  name: string
  remotePath: string
  /** 落盘目录（即 --saveto 的值） */
  saveDir: string
  args: string[]
  commandLine: string
  status: TaskStatus
  /** killed 时为 -1；注意 CLI 出错时退出码也是 0，不能靠它判断成败 */
  exitCode: number
  pid: number
  startedAt: string
  /** RFC3339 或 null */
  finishedAt: string | null
}

export interface BrowseEntry {
  name: string
  /** 完整路径，可直接用于创建任务 */
  path: string
  isDir: boolean
  /** 解析出来的字节数，目录为 0 */
  size: number
  /** CLI 大小列的原样文本，目录为 "-"。显示时用这个才能与 CLI 完全一致。 */
  sizeText: string
  /** RFC3339 格式的修改时间 */
  modTime: string
  /** CLI 日期列的原样文本，形如 2017-07-20 17:03:36 */
  modTimeText: string
}

export interface BrowseResult {
  path: string
  parent: string
  entries: BrowseEntry[]
}

export interface Health {
  ok: boolean
  cliPath: string
  saveRoot: string
  mode: string
}

export class ApiError extends Error {
  readonly status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body !== undefined && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  let res: Response
  try {
    res = await fetch(path, { ...init, headers })
  } catch {
    throw new ApiError('无法连接到服务器，请确认后端已启动', 0)
  }

  if (!res.ok) {
    let message = `请求失败（HTTP ${res.status}）`
    try {
      const data: unknown = await res.json()
      if (
        data !== null &&
        typeof data === 'object' &&
        'error' in data &&
        typeof (data as { error: unknown }).error === 'string'
      ) {
        message = (data as { error: string }).error
      }
    } catch {
      // 响应体不是 JSON，沿用默认文案
    }
    throw new ApiError(message, res.status)
  }

  if (res.status === 204) {
    return undefined as unknown as T
  }

  return (await res.json()) as T
}

export function toErrorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message
  if (err instanceof Error) return err.message
  return '发生未知错误'
}

/* ------------------------------- 任务 ------------------------------- */

/** GET /api/tasks —— 最新的在前。 */
export async function getTasks(): Promise<Task[]> {
  const data = await request<{ tasks: Task[] }>('/api/tasks')
  return data.tasks ?? []
}

/** POST /api/tasks —— 会先校验路径存在，再启动后台进程。 */
export function createTask(remotePath: string): Promise<Task> {
  return request<Task>('/api/tasks', {
    method: 'POST',
    body: JSON.stringify({ remotePath }),
  })
}

/** POST /api/tasks/{id}/kill —— 结束进程，保留任务记录与已下载分片。 */
export function killTask(id: string): Promise<Task> {
  return request<Task>(`/api/tasks/${encodeURIComponent(id)}/kill`, { method: 'POST' })
}

/** POST /api/tasks/{id}/restart —— 用同样的命令重新起进程，会断点续传。 */
export function restartTask(id: string): Promise<Task> {
  return request<Task>(`/api/tasks/${encodeURIComponent(id)}/restart`, { method: 'POST' })
}

/** DELETE /api/tasks/{id} —— 结束并移除任务，磁盘文件不动。 */
export function deleteTask(id: string): Promise<void> {
  return request<void>(`/api/tasks/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

/* ------------------------------- 浏览 ------------------------------- */

export function browse(path?: string): Promise<BrowseResult> {
  const query = path !== undefined && path !== '' ? `?path=${encodeURIComponent(path)}` : ''
  return request<BrowseResult>(`/api/browse${query}`)
}

/* ------------------------------- 其它 ------------------------------- */

export function getHealth(): Promise<Health> {
  return request<Health>('/api/health')
}
