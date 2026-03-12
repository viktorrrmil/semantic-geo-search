import { useState, useCallback } from 'react'

const API = 'http://localhost:3001/api/v1'

interface S3Entry {
  key: string
  type: 'file' | 'folder'
  size?: number
}

interface S3ListResponse {
  bucket: string
  prefix: string
  entries: S3Entry[]
}

interface IndexDialogState {
  path: string
  name: string
  isFolder: boolean
  fileCount: number
}

export interface SelectedFile {
  path: string
  name: string
  region: string
}

interface Props {
  isOpen: boolean
  onClose: () => void
  selectedFilePath: string | null
  onFileSelect: (file: SelectedFile | null) => void
}

function buildBreadcrumbs(path: string): Array<{ label: string; path: string }> {
  if (!path) return []
  const parts = path.split('/').filter(Boolean)
  return parts.map((part, i) => ({
    label: part,
    path: parts.slice(0, i + 1).join('/'),
  }))
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
}

function entryDisplayName(key: string): string {
  const clean = key.replace(/\/$/, '')
  return clean.split('/').pop() ?? clean
}

export default function S3Explorer({ isOpen, onClose, selectedFilePath, onFileSelect }: Props) {
  const [urlInput, setUrlInput] = useState('s3://overturemaps-us-west-2/release')
  const [currentPath, setCurrentPath] = useState('')
  const [entries, setEntries] = useState<S3Entry[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [region, setRegion] = useState('us-west-2')

  const [indexDialog, setIndexDialog] = useState<IndexDialogState | null>(null)
  const [indexCount, setIndexCount] = useState('')
  const [indexing, setIndexing] = useState(false)
  const [indexMsg, setIndexMsg] = useState('')

  const breadcrumbs = buildBreadcrumbs(currentPath)
  const bucket = currentPath ? currentPath.split('/')[0] : ''
  const fileEntries = entries.filter(e => e.type === 'file')

  const navigate = useCallback(async (path: string) => {
    if (!path) return
    setLoading(true)
    setError('')
    try {
      const encoded = path.split('/').map(encodeURIComponent).join('/')
      const res = await fetch(`${API}/list-files/${encoded}?region=${region}`)
      if (!res.ok) throw new Error(`Server responded with ${res.status}`)
      const data: S3ListResponse = await res.json()
      setEntries(data.entries ?? [])
      setCurrentPath(path)
      setUrlInput(`s3://${path}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to list files')
    } finally {
      setLoading(false)
    }
  }, [region])

  function handleUrlSubmit(e: React.FormEvent) {
    e.preventDefault()
    const clean = urlInput.trim().replace(/^s3:\/\//, '').replace(/\/+$/, '')
    if (clean) navigate(clean)
  }

  function handleFileClick(entry: S3Entry) {
    const filePath = `s3://${bucket}/${entry.key}`
    if (selectedFilePath === filePath) {
      onFileSelect(null)
    } else {
      onFileSelect({ path: filePath, name: entryDisplayName(entry.key), region })
    }
  }

  function openIndexDialog(path: string, name: string, isFolder: boolean, fileCount = 0) {
    setIndexDialog({ path, name, isFolder, fileCount })
    setIndexCount(isFolder ? String(fileCount) : '')
    setIndexMsg('')
  }

  function handleIndexFolder() {
    const name = currentPath.split('/').pop() ?? currentPath
    openIndexDialog(`s3://${currentPath}`, name, true, fileEntries.length)
  }

  function handleFileIndex(entry: S3Entry) {
    const name = entryDisplayName(entry.key)
    openIndexDialog(`s3://${bucket}/${entry.key}`, name, false)
  }

  async function submitIndex() {
    if (!indexDialog) return
    setIndexing(true)
    setIndexMsg('')
    try {
      const count = parseInt(indexCount, 10)
      const res = await fetch(`${API}/index-file`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          s3_path: indexDialog.path,
          count: isNaN(count) || indexCount === '' ? null : count,
          region,
        }),
      })
      if (!res.ok) throw new Error(`Server responded with ${res.status}`)
      setIndexMsg('Indexing started successfully.')
      setTimeout(() => { setIndexDialog(null); setIndexMsg('') }, 1500)
    } catch (err) {
      setIndexMsg(err instanceof Error ? err.message : 'Failed to start indexing')
    } finally {
      setIndexing(false)
    }
  }

  return (
    <>
      <div
        className={`fixed top-0 left-0 h-full w-[360px] bg-white border-r border-gray-200 shadow-xl z-50 flex flex-col transition-transform duration-200 ease-in-out ${
          isOpen ? 'translate-x-0' : '-translate-x-full'
        }`}
      >
        {/* Header */}
        <div className="px-4 py-3 border-b border-gray-200 flex items-center justify-between shrink-0">
          <div>
            <h2 className="text-sm font-medium text-gray-700">S3 Browser</h2>
            <p className="text-xs text-gray-400">Click a file to select it</p>
          </div>
          <button
            onClick={onClose}
            className="text-gray-300 hover:text-gray-500 transition-colors p-1 rounded"
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
              <path d="M1 1l12 12M13 1L1 13" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
            </svg>
          </button>
        </div>

        {/* URL + region */}
        <form onSubmit={handleUrlSubmit} className="px-4 py-3 border-b border-gray-200 shrink-0 space-y-2">
          <div>
            <label className="block text-xs text-gray-400 mb-1">S3 Path</label>
            <div className="flex gap-2">
              <input
                type="text"
                value={urlInput}
                onChange={e => setUrlInput(e.target.value)}
                placeholder="s3://bucket/prefix"
                spellCheck={false}
                className="flex-1 text-xs bg-white border border-gray-200 rounded-md px-3 py-1.5 text-gray-800 placeholder-gray-400 outline-none focus:border-gray-400 font-mono transition-colors"
              />
              <button
                type="submit"
                disabled={loading || !urlInput.trim()}
                className="px-3 py-1.5 text-xs bg-gray-800 text-white rounded-md hover:bg-gray-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
              >
                Go
              </button>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <label className="text-xs text-gray-400 shrink-0">Region</label>
            <input
              type="text"
              value={region}
              onChange={e => setRegion(e.target.value)}
              spellCheck={false}
              className="flex-1 text-xs bg-white border border-gray-200 rounded-md px-2 py-1 text-gray-700 outline-none focus:border-gray-400 font-mono transition-colors"
            />
          </div>
        </form>

        {/* Breadcrumbs */}
        {breadcrumbs.length > 0 && (
          <div className="px-4 py-2 border-b border-gray-100 shrink-0 flex flex-wrap gap-x-0.5 gap-y-1 items-center min-h-[34px]">
            {breadcrumbs.map((crumb, i) => {
              const isLast = i === breadcrumbs.length - 1
              return (
                <span key={crumb.path} className="flex items-center">
                  {i > 0 && <span className="text-[10px] text-gray-300 mx-0.5">/</span>}
                  <button
                    onClick={() => !isLast && navigate(crumb.path)}
                    disabled={isLast}
                    className={`text-[10px] px-1 py-0.5 rounded transition-colors ${
                      isLast
                        ? 'text-gray-600 font-medium cursor-default'
                        : 'text-gray-400 hover:text-gray-700 hover:bg-gray-100'
                    }`}
                  >
                    {crumb.label}
                  </button>
                </span>
              )
            })}
          </div>
        )}

        {/* Index-all button */}
        {fileEntries.length > 0 && !loading && (
          <div className="px-4 py-2 border-b border-gray-100 shrink-0">
            <button
              onClick={handleIndexFolder}
              className="text-[11px] text-indigo-600 hover:text-indigo-800 border border-indigo-200 hover:border-indigo-300 px-3 py-1.5 rounded-md bg-indigo-50 hover:bg-indigo-100 transition-colors w-full text-left"
            >
              ↓ Index all {fileEntries.length} file{fileEntries.length !== 1 ? 's' : ''} in this folder
            </button>
          </div>
        )}

        {/* Entry list */}
        <div className="flex-1 overflow-y-auto">
          {error && (
            <div className="mx-4 mt-3 px-3 py-2 bg-red-50 border border-red-100 rounded-md">
              <p className="text-xs text-red-500">{error}</p>
            </div>
          )}

          {loading && (
            <div className="flex items-center justify-center py-12">
              <div className="flex gap-1.5">
                <span className="w-1.5 h-1.5 rounded-full bg-gray-300 animate-bounce [animation-delay:-0.3s]" />
                <span className="w-1.5 h-1.5 rounded-full bg-gray-300 animate-bounce [animation-delay:-0.15s]" />
                <span className="w-1.5 h-1.5 rounded-full bg-gray-300 animate-bounce" />
              </div>
            </div>
          )}

          {!loading && !error && entries.length === 0 && currentPath && (
            <div className="flex items-center justify-center py-12">
              <p className="text-xs text-gray-400">This location is empty</p>
            </div>
          )}

          {!loading && !error && !currentPath && (
            <div className="flex items-center justify-center py-12 px-6 text-center">
              <p className="text-xs text-gray-400 leading-relaxed">
                Enter an S3 path above and press <span className="font-medium text-gray-500">Go</span> to browse
              </p>
            </div>
          )}

          {!loading && entries.length > 0 && (
            <ul className="py-1.5">
              {entries.map(entry => (
                <EntryRow
                  key={entry.key}
                  entry={entry}
                  bucket={bucket}
                  isSelected={selectedFilePath === `s3://${bucket}/${entry.key}`}
                  onNavigate={path => navigate(path)}
                  onFileClick={() => handleFileClick(entry)}
                  onIndex={() => handleFileIndex(entry)}
                />
              ))}
            </ul>
          )}
        </div>
      </div>

      {/* Index dialog */}
      {indexDialog && (
        <IndexDialog
          target={indexDialog}
          count={indexCount}
          onCountChange={setIndexCount}
          loading={indexing}
          message={indexMsg}
          onSubmit={submitIndex}
          onCancel={() => setIndexDialog(null)}
        />
      )}
    </>
  )
}

// ── Sub-components ────────────────────────────────────────────────────────────

function EntryRow({
  entry,
  bucket,
  isSelected,
  onNavigate,
  onFileClick,
  onIndex,
}: {
  entry: S3Entry
  bucket: string
  isSelected: boolean
  onNavigate: (path: string) => void
  onFileClick: () => void
  onIndex: () => void
}) {
  const name = entryDisplayName(entry.key)
  const isFolder = entry.type === 'folder'

  function handleRowClick() {
    if (isFolder) {
      onNavigate(`${bucket}/${entry.key.replace(/\/$/, '')}`)
    } else {
      onFileClick()
    }
  }

  return (
    <li
      className={`flex items-center gap-2.5 px-4 py-2 cursor-pointer transition-colors ${
        isSelected
          ? 'bg-blue-50 border-l-2 border-blue-400'
          : isFolder
          ? 'hover:bg-gray-50'
          : 'hover:bg-gray-50 pl-4'
      }`}
      style={isSelected ? { paddingLeft: '14px' } : undefined}
      onClick={handleRowClick}
    >
      {/* Icon */}
      <span className={`shrink-0 ${isSelected ? 'text-blue-500' : 'text-gray-400'}`}>
        {isFolder ? (
          <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
            <path
              d="M1 3.5C1 2.67 1.67 2 2.5 2h2.586c.265 0 .52.105.707.293L6.5 3h5C12.33 3 13 3.67 13 4.5v6c0 .83-.67 1.5-1.5 1.5h-9C1.67 12 1 11.33 1 10.5v-7z"
              fill="currentColor"
              fillOpacity=".25"
              stroke="currentColor"
              strokeWidth="1"
            />
          </svg>
        ) : (
          <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
            <rect x="2" y="1" width="8" height="12" rx="1" fill="currentColor" fillOpacity=".15" stroke="currentColor" strokeWidth="1" />
            <path d="M8 1v3.5H11" stroke="currentColor" strokeWidth="1" strokeLinecap="round" />
            <path d="M4 7h5M4 9.5h3.5" stroke="currentColor" strokeWidth="0.8" strokeLinecap="round" />
          </svg>
        )}
      </span>

      {/* Name + size */}
      <div className="flex-1 min-w-0">
        <p className={`text-xs truncate ${isSelected ? 'text-blue-700 font-medium' : isFolder ? 'text-gray-700' : 'text-gray-600'}`}>
          {name}
        </p>
        {entry.size != null && (
          <p className="text-[10px] text-gray-400 font-mono">{formatSize(entry.size)}</p>
        )}
      </div>

      {/* Right-side action */}
      {isFolder ? (
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" className="shrink-0 text-gray-300">
          <path d="M3 2l4 3-4 3" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      ) : (
        <button
          onClick={e => { e.stopPropagation(); onIndex() }}
          className="shrink-0 text-[10px] text-indigo-500 border border-indigo-200 hover:border-indigo-400 hover:bg-indigo-50 px-1.5 py-0.5 rounded transition-colors"
          title="Index this file"
        >
          Index
        </button>
      )}
    </li>
  )
}

function IndexDialog({
  target,
  count,
  onCountChange,
  loading,
  message,
  onSubmit,
  onCancel,
}: {
  target: IndexDialogState
  count: string
  onCountChange: (v: string) => void
  loading: boolean
  message: string
  onSubmit: () => void
  onCancel: () => void
}) {
  const isSuccess = message.toLowerCase().includes('success') || message.toLowerCase().includes('started')

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center">
      <div className="absolute inset-0 bg-black/25" onClick={onCancel} />
      <div className="relative bg-white rounded-lg shadow-2xl border border-gray-200 w-[360px] p-5 z-10 mx-4">
        <h3 className="text-sm font-medium text-gray-800 mb-0.5">
          {target.isFolder ? 'Index folder' : 'Index file'}
        </h3>
        <p className="text-[11px] text-gray-400 font-mono break-all mb-4 leading-relaxed">
          {target.path}
        </p>

        {target.isFolder && (
          <p className="text-xs text-gray-500 mb-3">
            Found <span className="font-medium text-gray-700">{target.fileCount}</span> file{target.fileCount !== 1 ? 's' : ''} in this folder.
          </p>
        )}

        <label className="block text-xs text-gray-500 mb-1.5">How many items to index?</label>
        <div className="flex gap-2 mb-4">
          <input
            type="number"
            min={1}
            value={count}
            onChange={e => onCountChange(e.target.value)}
            placeholder="Leave empty for all"
            autoFocus
            className="flex-1 text-sm border border-gray-200 rounded-md px-3 py-1.5 text-gray-800 placeholder-gray-300 outline-none focus:border-gray-400 transition-colors"
          />
          <button
            type="button"
            onClick={() => onCountChange(target.isFolder ? String(target.fileCount) : '')}
            className="px-2.5 py-1.5 text-xs text-gray-600 border border-gray-200 rounded-md hover:bg-gray-50 transition-colors whitespace-nowrap"
          >
            {target.isFolder ? `All (${target.fileCount})` : 'All'}
          </button>
        </div>

        {!count && (
          <p className="text-[11px] text-gray-400 mb-3 -mt-2">Leaving empty will index all available items.</p>
        )}

        {message && (
          <p className={`text-xs mb-3 ${isSuccess ? 'text-emerald-600' : 'text-red-500'}`}>{message}</p>
        )}

        <div className="flex gap-2 justify-end">
          <button onClick={onCancel} className="px-3 py-1.5 text-xs text-gray-600 border border-gray-200 rounded-md hover:bg-gray-50 transition-colors">
            Cancel
          </button>
          <button
            onClick={onSubmit}
            disabled={loading}
            className="px-3 py-1.5 text-xs bg-gray-800 text-white rounded-md hover:bg-gray-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            {loading ? 'Indexing…' : 'Index'}
          </button>
        </div>
      </div>
    </div>
  )
}
