import { useEffect, useState } from 'react'
import { LuChevronRight, LuCornerLeftUp } from 'react-icons/lu'
import { browse, toErrorMessage } from '../api'
import type { BrowseResult } from '../api'
import { renderFileIcon } from '../fileIcon'

interface BrowserProps {
  selected: string
  onSelect: (path: string) => void
}

interface Crumb {
  label: string
  path: string
}

/** 已加载的目录结果，用 path 标记它对应哪一次请求。 */
interface Loaded {
  path: string
  result: BrowseResult | null
  error: string | null
}

function buildCrumbs(path: string): Crumb[] {
  const crumbs: Crumb[] = [{ label: '/', path: '/' }]
  let acc = ''
  for (const segment of path.split('/').filter((part) => part !== '')) {
    acc += `/${segment}`
    crumbs.push({ label: segment, path: acc })
  }
  return crumbs
}

/** 网盘浏览器：面包屑 + 目录浏览，点目录进入，点文件选中。 */
export function Browser({ selected, onSelect }: BrowserProps) {
  const [path, setPath] = useState('/')
  const [loaded, setLoaded] = useState<Loaded | null>(null)

  useEffect(() => {
    let alive = true
    browse(path).then(
      (result) => {
        if (!alive) return
        setLoaded({ path, result, error: null })
        // 后端可能规范化了路径（例如去掉结尾斜杠），同步一次。
        if (result.path !== path) setPath(result.path)
      },
      (err: unknown) => {
        if (alive) setLoaded({ path, result: null, error: toErrorMessage(err) })
      },
    )
    return () => {
      alive = false
    }
  }, [path])

  // 只有结果对应当前请求时才使用，否则视为加载中。
  const current = loaded !== null && loaded.path === path ? loaded : null
  const loading = current === null
  const error = current === null ? null : current.error
  const data = current === null ? null : current.result
  // 保持后端返回的顺序，即 BaiduPCS-Go ls 的原始顺序，不在前端重排。
  const entries = data === null ? [] : data.entries
  const showParent = data !== null && data.parent !== data.path
  const crumbs = buildCrumbs(path)

  return (
    <div className="browser">
      <div className="browser-crumbs">
        {crumbs.map((crumb, index) => (
          <span key={crumb.path} className="crumb-item">
            {index > 0 && <LuChevronRight className="crumb-sep" aria-hidden="true" />}
            <button
              type="button"
              className="crumb"
              onClick={() => setPath(crumb.path)}
              disabled={crumb.path === path}
            >
              {crumb.label}
            </button>
          </span>
        ))}
      </div>

      <div className="browser-list">
        {loading && <div className="browser-status">加载中…</div>}
        {!loading && error !== null && <div className="browser-status browser-error">{error}</div>}
        {!loading && error === null && data !== null && (
          <>
            {showParent && (
              <div className="browser-row parent-row">
                <button type="button" className="entry-name" onClick={() => setPath(data.parent)}>
                  <LuCornerLeftUp className="entry-icon" aria-hidden="true" />
                  <span className="entry-label">返回上级</span>
                </button>
              </div>
            )}
            {entries.length === 0 && <div className="browser-status">此目录为空</div>}
            {entries.map((entry) => (
              <div
                key={entry.path}
                className={
                  selected === entry.path
                    ? 'browser-row is-selected'
                    : entry.isDir
                      ? 'browser-row is-dir'
                      : 'browser-row'
                }
              >
                <button
                  type="button"
                  className="entry-name"
                  onClick={() => (entry.isDir ? setPath(entry.path) : onSelect(entry.path))}
                  title={entry.path}
                >
                  {renderFileIcon(entry.name, entry.isDir, 'entry-icon')}
                  <span className="entry-label">{entry.name}</span>
                </button>
                {/* 直接用 CLI 的原样文本，保证与 `BaiduPCS-Go ls` 显示一致 */}
                <span className="entry-size">{entry.sizeText}</span>
                <span className="entry-mtime">{entry.modTimeText}</span>
                <button
                  type="button"
                  className="btn btn-ghost btn-sm"
                  onClick={() => onSelect(entry.path)}
                >
                  选择
                </button>
              </div>
            ))}
          </>
        )}
      </div>
    </div>
  )
}
