import { useState } from 'react'
import { toErrorMessage } from '../api'
import { Browser } from './Browser'
import { Modal } from './Modal'

interface AddTaskDialogProps {
  onClose: () => void
  onCreate: (remotePath: string) => Promise<unknown>
  onCreated: (message: string) => void
}

export function AddTaskDialog({ onClose, onCreate, onCreated }: AddTaskDialogProps) {
  const [selected, setSelected] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = async () => {
    const remotePath = selected.trim()
    if (remotePath === '') {
      setError('请先选择或输入网盘路径')
      return
    }
    setSubmitting(true)
    setError(null)
    try {
      await onCreate(remotePath)
      onCreated('任务已创建')
      onClose()
    } catch (err) {
      setError(toErrorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title="新建下载"
      wide
      onClose={onClose}
      footer={
        <>
          <div className="footer-path">
            <span className="footer-path-label">已选择</span>
            <code className="footer-path-value">{selected.trim() === '' ? '未选择' : selected.trim()}</code>
          </div>
          <div className="footer-actions">
            <button type="button" className="btn btn-ghost" onClick={onClose}>
              取消
            </button>
            <button
              type="button"
              className="btn btn-primary"
              onClick={() => void submit()}
              disabled={submitting || selected.trim() === ''}
            >
              {submitting ? '创建中…' : '开始下载'}
            </button>
          </div>
        </>
      }
    >
      <div className="field">
        <label className="field-label" htmlFor="remote-path-input">
          网盘路径
        </label>
        <input
          id="remote-path-input"
          className="input"
          type="text"
          value={selected}
          placeholder="例如 /我的资源/a.pdf，也可以从下方浏览选择"
          onChange={(event) => setSelected(event.target.value)}
          spellCheck={false}
        />
      </div>

      <Browser selected={selected} onSelect={setSelected} />

      {error !== null && <div className="form-error">{error}</div>}
    </Modal>
  )
}
