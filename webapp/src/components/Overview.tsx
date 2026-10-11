import { useEffect, useState } from 'react'
import {
  LuCircleStop,
  LuCloudDownload,
  LuFolderOpen,
  LuGauge,
  LuLayoutDashboard,
  LuLoaderCircle,
  LuMonitorCog,
  LuPlus,
  LuServer,
  LuSquare,
} from 'react-icons/lu'
import type { IconType } from 'react-icons'
import { getHealth, toErrorMessage } from '../api'
import type { Health, Task } from '../api'
import { renderFileIcon } from '../fileIcon'
import { formatSpeed } from '../format'
import { totalSpeed } from '../speedSeries'
import type { SpeedSeries } from '../speedSeries'
import { SpeedChart } from './SpeedChart'

interface OverviewProps {
  tasks: Task[]
  /** 合计速度曲线，由 useTasks 在轮询时采样 */
  speedSeries: SpeedSeries
  onSelect: (id: string) => void
  onNewTask: () => void
}

function clampPercent(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, value))
}

/**
 * 没选中任务时右侧显示的概览。
 *
 * 只展示已有数据，不额外请求任务列表（那份数据 App 已经拿着了）；运行环境那一栏
 * 来自 `GET /api/health`，所以在组件里单独拉一次。
 */
export function Overview({ tasks, speedSeries, onSelect, onNewTask }: OverviewProps) {
  const running = tasks.filter((task) => task.status === 'running')
  const stopped = tasks.filter((task) => task.status === 'killed')
  const finished = tasks.filter((task) => task.status === 'exited')

  // 合计速度只能自己算：CLI 每一行只报单个文件的速度，没有「总共多快」这种文本。
  const speed = totalSpeed(tasks)

  return (
    <div className="overview">
      <header className="overview-header">
        <h2 className="overview-title">
          <LuLayoutDashboard aria-hidden="true" />
          概览
        </h2>
        <button type="button" className="btn btn-primary btn-sm" onClick={onNewTask}>
          <LuPlus aria-hidden="true" />
          新建下载
        </button>
      </header>

      <div className="overview-body">
        <section className="stat-row">
          <StatCard icon={LuCloudDownload} label="任务总数" value={String(tasks.length)} />
          <StatCard
            icon={LuLoaderCircle}
            label="下载中"
            value={String(running.length)}
            tone={running.length > 0 ? 'running' : undefined}
            spin={running.length > 0}
          />
          <StatCard icon={LuSquare} label="已停止" value={String(stopped.length)} tone="stopped" />
          <StatCard icon={LuCircleStop} label="已结束" value={String(finished.length)} />
          <StatCard
            icon={LuGauge}
            label="合计速度"
            value={formatSpeed(speed)}
            tone={speed > 0 ? 'running' : undefined}
          />
        </section>

        <section className="overview-section">
          <h3 className="overview-section-title">
            <LuLoaderCircle aria-hidden="true" />
            正在下载
            {running.length > 0 && <span className="overview-badge">{running.length}</span>}
          </h3>

          {running.length === 0 ? (
            <p className="overview-hint">
              {tasks.length === 0
                ? '还没有任何任务。点右上角「新建下载」从网盘选文件。'
                : '当前没有正在下载的任务。从左侧选一个任务查看它的终端输出，或点「继续」重新开始。'}
            </p>
          ) : (
            <div className="overview-running">
              {running.map((task) => (
                <RunningRow key={task.id} task={task} onSelect={onSelect} />
              ))}
            </div>
          )}
        </section>

        <section className="overview-section">
          <h3 className="overview-section-title">
            <LuGauge aria-hidden="true" />
            合计速度
            <span className="overview-section-note">最近 5 分钟 · 所有运行中任务之和</span>
          </h3>
          <SpeedChart times={speedSeries.times} speeds={speedSeries.speeds} />
        </section>

        <EnvironmentSection />
      </div>
    </div>
  )
}

/** 一张统计卡片。 */
function StatCard({
  icon: Icon,
  label,
  value,
  tone,
  spin = false,
}: {
  icon: IconType
  label: string
  value: string
  tone?: 'running' | 'stopped'
  spin?: boolean
}) {
  const classes = ['stat-card']
  if (tone !== undefined) classes.push(`is-${tone}`)

  return (
    <div className={classes.join(' ')}>
      <span className="stat-icon">
        <Icon className={spin ? 'is-spin' : undefined} aria-hidden="true" />
      </span>
      <span className="stat-value">{value}</span>
      <span className="stat-label">{label}</span>
    </div>
  )
}

/** 正在下载的任务：一行，带进度条，点一下切到它的终端。 */
function RunningRow({ task, onSelect }: { task: Task; onSelect: (id: string) => void }) {
  const progress = task.progress
  const known = progress !== undefined && progress.totalBytes > 0
  const percent = progress === undefined ? 0 : clampPercent(progress.percent)

  return (
    <button type="button" className="overview-running-row" onClick={() => onSelect(task.id)}>
      <div className="overview-running-head">
        {renderFileIcon(task.name, task.isDir, 'overview-running-icon')}
        <span className="overview-running-name" title={task.remotePath}>
          {task.name}
        </span>
        {progress?.speedText !== undefined && progress.speedText !== '' && (
          <span className="overview-running-speed">{progress.speedText}</span>
        )}
      </div>

      <div className="overview-running-track">
        <div
          className="overview-running-fill"
          style={{ width: known ? `${percent}%` : '0%' }}
        />
      </div>

      <div className="overview-running-meta">
        {known && <span className="overview-running-percent">{percent.toFixed(1)}%</span>}
        {progress !== undefined ? (
          <span>
            {progress.doneText} / {progress.totalText}
            {progress.etaText !== '' && progress.etaText !== '-' && ` · 剩 ${progress.etaText}`}
          </span>
        ) : (
          <span>等待输出…</span>
        )}
      </div>
    </button>
  )
}

/** 运行环境。这些值启动后就固定了，所以只拉一次。 */
function EnvironmentSection() {
  const [health, setHealth] = useState<Health | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let alive = true
    getHealth().then(
      (data) => {
        if (alive) setHealth(data)
      },
      (err: unknown) => {
        if (alive) setError(toErrorMessage(err))
      },
    )
    return () => {
      alive = false
    }
  }, [])

  return (
    <section className="overview-section">
      <h3 className="overview-section-title">
        <LuServer aria-hidden="true" />
        运行环境
      </h3>

      {error !== null ? (
        <p className="overview-hint">读取失败：{error}</p>
      ) : health === null ? (
        <p className="overview-hint">读取中…</p>
      ) : (
        <dl className="env-list">
          <div className="env-item">
            <dt>
              <LuMonitorCog aria-hidden="true" />
              CLI
            </dt>
            <dd>
              <code>{health.cliPath}</code>
            </dd>
          </div>
          <div className="env-item">
            <dt>
              <LuFolderOpen aria-hidden="true" />
              下载目录
            </dt>
            <dd>
              <code>{health.saveRoot}</code>
            </dd>
          </div>
          <div className="env-item">
            <dt>
              <LuCloudDownload aria-hidden="true" />
              下载模式
            </dt>
            <dd>
              <code>{health.mode}</code>
            </dd>
          </div>
        </dl>
      )}
    </section>
  )
}
