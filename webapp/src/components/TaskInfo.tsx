import { useEffect, useState } from 'react'
import { LuCheck, LuCopy } from 'react-icons/lu'
import type { Task } from '../api'
import { formatDateTime, formatDuration } from '../format'

/** 一行「标签 + 值」。 */
function Row({
  label,
  children,
  copy,
}: {
  label: string
  children: React.ReactNode
  /** 给一个可复制的原文（路径、命令这类），有值时才显示复制按钮 */
  copy?: string
}) {
  const [copied, setCopied] = useState(false)

  const onCopy = () => {
    if (copy === undefined) return
    void navigator.clipboard.writeText(copy).then(
      () => {
        setCopied(true)
        window.setTimeout(() => setCopied(false), 1400)
      },
      () => {
        /* 剪贴板不可用（非 https、无权限）就算了，不值得打扰用户 */
      },
    )
  }

  return (
    <div className="task-info-row">
      <dt className="task-info-key">{label}</dt>
      <dd className="task-info-value">
        <span className="task-info-text">{children}</span>
        {copy !== undefined && copy !== '' && (
          <button
            type="button"
            className="task-info-copy"
            onClick={onCopy}
            aria-label={`复制${label}`}
            title="复制"
          >
            {copied ? <LuCheck aria-hidden="true" /> : <LuCopy aria-hidden="true" />}
          </button>
        )}
      </dd>
    </div>
  )
}

function joinPath(dir: string, name: string): string {
  if (dir === '') return name
  return dir.endsWith('/') ? dir + name : `${dir}/${name}`
}

/**
 * 任务信息。
 *
 * 任务详情页的主体就是这个——终端退到底部一小块去了。这里回答的是「这是什么、
 * 下到哪去了、跑了多久、还能不能续」这几个问题。
 *
 * 关于退出码：没有单列一行，因为它不表达成败——CLI 出错时退出码也是 0，
 * 判断成功与否只能看终端输出。列出来只会误导。
 */
export function TaskInfo({ task }: { task: Task }) {
  // 运行中的任务，时长要自己走。用 state 存「现在」而不是在 render 里直接读时钟：
  // 在 render 里调 Date.now() 是不纯的（React Compiler 没法安全地记住结果），
  // 而且每秒 tick 一次看着才像在跑。
  const [now, setNow] = useState(() => Date.now())
  const running = task.finishedAt === null

  useEffect(() => {
    if (!running) return
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [running])

  const target = joinPath(task.saveDir, task.name)
  const started = new Date(task.startedAt).getTime()
  const finished = task.finishedAt === null ? null : new Date(task.finishedAt).getTime()
  const elapsed = Number.isNaN(started) ? null : (finished ?? now) - started

  return (
    <section className="task-info">
      <h3 className="panel-title">任务信息</h3>

      <dl className="task-info-list">
        <Row label="网盘路径" copy={task.remotePath}>
          <code>{task.remotePath}</code>
        </Row>

        <Row label="保存位置" copy={target}>
          <code>{target}</code>
        </Row>

        <Row label="类型">{task.isDir ? '目录' : '文件'}</Row>

        <Row label="开始时间">{formatDateTime(task.startedAt)}</Row>

        <Row label={finished === null ? '已运行' : '耗时'}>
          {elapsed === null ? '—' : formatDuration(elapsed)}
        </Row>

        {finished !== null && <Row label="结束时间">{formatDateTime(task.finishedAt)}</Row>}

        <Row label="断点续传">
          {task.resumable ? '磁盘上有未完成的分片，点「继续」接着下' : '没有可续传的分片'}
        </Row>

        {task.progress !== undefined && task.progress.filesTotal > 1 && (
          <Row label="文件进度">
            {task.progress.filesDone} / {task.progress.filesTotal} 个文件
            {task.progress.currentName !== '' && `，当前 ${task.progress.currentName}`}
          </Row>
        )}

        {task.pid > 0 && <Row label="进程">PID {task.pid}</Row>}

        <Row label="完整命令" copy={task.commandLine}>
          <code>{task.commandLine}</code>
        </Row>

        <Row label="任务 ID" copy={task.id}>
          <code>{task.id}</code>
        </Row>
      </dl>
    </section>
  )
}
