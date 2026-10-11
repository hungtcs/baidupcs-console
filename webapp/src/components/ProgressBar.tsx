import { LuCircleCheck, LuClock, LuGauge, LuTimer } from 'react-icons/lu'
import type { Progress } from '../api'

function clampPercent(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, value))
}

/**
 * 终端上方的进度读数。
 *
 * 这是整个界面里唯一「用力」的地方：百分比用大号等宽数字立在最前面，下面是贯穿
 * 整幅宽度的进度尺，再往下才是细节。为的是隔着一米、或拿在手上瞟一眼就能读到。
 *
 * 数字一律用 CLI 的原样文本，不做二次格式化，保证和下面的终端里看到的一致。
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
      <div className="progress-readout">
        {known && <span className="progress-percent">{percent.toFixed(1)}%</span>}
        <span className="progress-bytes">
          {progress.doneText}
          <span className="progress-bytes-sep">/</span>
          {progress.totalText}
        </span>
      </div>

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

        {multiFile && <span className="progress-stat">已完成 {progress.filesDone} 个文件</span>}
        {multiFile && progress.currentName !== '' && (
          <span className="progress-stat progress-current" title={progress.currentName}>
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
