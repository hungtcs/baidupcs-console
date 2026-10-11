import { LuCircleCheck, LuClock, LuGauge, LuTimer } from 'react-icons/lu'
import type { Progress } from '../api'

function clampPercent(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, value))
}

/**
 * 终端上方的下载进度状态栏。
 *
 * 数据是后端从 CLI 输出里解析出来的，每秒从终端 WebSocket 推过来，所以它和下面的
 * 终端内容说的是同一件事，只是把关键数字拎出来了。数字一律用 CLI 的原样文本，
 * 不做二次格式化，保证和终端里看到的一致。
 *
 * 目录下载时 CLI 逐文件打印、事先不告诉目录总大小，所以百分比是「当前文件」的，
 * 另外用「已完成 N 个文件」和「当前 xxx」补充说明。
 */
export function ProgressBar({ progress }: { progress: Progress }) {
  // 总量未知时画不出有意义的进度条，只显示文本。
  const known = progress.totalBytes > 0
  const percent = clampPercent(progress.percent)
  const multiFile = progress.filesTotal > 1
  const hasEta = progress.etaText !== '' && progress.etaText !== '-'

  return (
    <div className={progress.finished ? 'progress is-done' : 'progress'}>
      <div
        className="progress-track"
        role="progressbar"
        aria-label="下载进度"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={known ? Math.round(percent) : undefined}
      >
        <div className="progress-fill" style={{ width: known ? `${percent}%` : '0%' }} />
      </div>

      <div className="progress-meta">
        {known && <span className="progress-percent">{percent.toFixed(1)}%</span>}

        <span className="progress-bytes">
          {progress.doneText} / {progress.totalText}
        </span>

        {progress.speedText !== '' && (
          <span className="progress-stat">
            <LuGauge aria-hidden="true" />
            {progress.speedText}
          </span>
        )}
        {progress.elapsedText !== '' && (
          <span className="progress-stat">
            <LuTimer aria-hidden="true" />
            已用 {progress.elapsedText}
          </span>
        )}
        {hasEta && (
          <span className="progress-stat">
            <LuClock aria-hidden="true" />
            剩 {progress.etaText}
          </span>
        )}

        {multiFile && (
          <span className="progress-stat progress-files">
            已完成 {progress.filesDone} 个文件
          </span>
        )}
        {multiFile && progress.currentName !== '' && (
          <span className="progress-current" title={progress.currentName}>
            当前 {progress.currentName}
          </span>
        )}

        {progress.finished && (
          <span className="progress-stat progress-finished">
            <LuCircleCheck aria-hidden="true" />
            下载完成
          </span>
        )}
      </div>
    </div>
  )
}
