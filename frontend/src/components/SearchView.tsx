import { useState, useRef, useEffect, useCallback } from 'react'
import type { BBox } from './MapView'

export interface Restaurant {
  id: string
  name: string
  confidence: number
  socials: Record<string, string> | null
  geometry: string
}

export interface ChunkInfo {
  chunk_id: string
  category: string
  bbox_min_x: number
  bbox_max_x: number
  bbox_min_y: number
  bbox_max_y: number
  record_count: number
  created_at: string
}

interface SearchResponse {
  restaurants: Restaurant[]
  total: number
  message?: string
}

interface ChunksListResponse {
  chunks: ChunkInfo[]
  total: number
  message?: string
}

interface PreprocessResponse {
  chunk_id: string
  category: string
  record_count: number
  message?: string
}

type SearchStatus = 'idle' | 'loading' | 'success' | 'error'
type PreprocessStatus = 'idle' | 'loading' | 'success' | 'error'

const API = 'http://localhost:3001/api/v1'

interface Props {
  bbox: BBox | null
  onResults: (restaurants: Restaurant[]) => void
  onChunksChange: (chunks: ChunkInfo[]) => void
}

export default function SearchView({ bbox, onResults, onChunksChange }: Props) {
  const [category, setCategory] = useState('')
  const [limit, setLimit] = useState('25')
  const [results, setResults] = useState<Restaurant[]>([])
  const [searchStatus, setSearchStatus] = useState<SearchStatus>('idle')
  const [searchError, setSearchError] = useState('')

  const [preprocessStatus, setPreprocessStatus] = useState<PreprocessStatus>('idle')
  const [preprocessMsg, setPreprocessMsg] = useState('')

  const [chunks, setChunks] = useState<ChunkInfo[]>([])
  const [chunksLoading, setChunksLoading] = useState(false)
  const [activeChunkId, setActiveChunkId] = useState<string | null>(null)

  const inputRef = useRef<HTMLInputElement>(null)
  const canSearch = !!category.trim() && !!bbox
  const canPreprocess = !!category.trim() && !!bbox && preprocessStatus !== 'loading'

  // Load chunks on mount and after preprocess
  const loadChunks = useCallback(async () => {
    setChunksLoading(true)
    try {
      const res = await fetch(`${API}/chunks`)
      if (!res.ok) throw new Error(`${res.status}`)
      const data: ChunksListResponse = await res.json()
      const list = data.chunks ?? []
      setChunks(list)
      onChunksChange(list)
    } catch {
      // silently ignore — chunks panel just stays empty
    } finally {
      setChunksLoading(false)
    }
  }, [onChunksChange])

  useEffect(() => { loadChunks() }, [loadChunks])

  // ── Search (live query) ────────────────────────────────────────────────────
  async function handleSearch(e?: React.FormEvent) {
    e?.preventDefault()
    if (!canSearch) return
    setSearchStatus('loading')
    setSearchError('')
    setActiveChunkId(null)
    try {
      const res = await fetch(`${API}/search-real`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          category: category.trim(),
          bbox_min_x: bbox!.minX,
          bbox_max_x: bbox!.maxX,
          bbox_min_y: bbox!.minY,
          bbox_max_y: bbox!.maxY,
          limit: parseInt(limit, 10) || 25,
        }),
      })
      if (!res.ok) throw new Error(`Server responded with ${res.status}`)
      const data: SearchResponse = await res.json()
      const list = data.restaurants ?? []
      setResults(list)
      onResults(list)
      setSearchStatus('success')
    } catch (err) {
      setSearchError(err instanceof Error ? err.message : 'Unknown error')
      setSearchStatus('error')
    }
  }

  // ── Precompute bbox chunk ──────────────────────────────────────────────────
  async function handlePreprocess() {
    setPreprocessStatus('loading')
    setPreprocessMsg('')
    try {
      const res = await fetch(`${API}/preprocess`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          category: category.trim(),
          bbox_min_x: bbox!.minX,
          bbox_max_x: bbox!.maxX,
          bbox_min_y: bbox!.minY,
          bbox_max_y: bbox!.maxY,
        }),
      })
      if (!res.ok) throw new Error(`Server responded with ${res.status}`)
      const data: PreprocessResponse = await res.json()
      setPreprocessMsg(`Chunk ready — ${data.record_count} records`)
      setPreprocessStatus('success')
      await loadChunks()
    } catch (err) {
      setPreprocessMsg(err instanceof Error ? err.message : 'Unknown error')
      setPreprocessStatus('error')
    }
  }

  // ── Search from precomputed chunk ──────────────────────────────────────────
  async function handleChunkSearch(chunkId: string) {
    setSearchStatus('loading')
    setSearchError('')
    setActiveChunkId(chunkId)
    setResults([])
    onResults([])
    try {
      const res = await fetch(`${API}/search-chunk/${chunkId}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          limit: parseInt(limit, 10) || 25,
        }),
      })
      if (!res.ok) throw new Error(`Server responded with ${res.status}`)
      const data: SearchResponse = await res.json()
      const list = data.restaurants ?? []
      setResults(list)
      onResults(list)
      setSearchStatus('success')
    } catch (err) {
      setSearchError(err instanceof Error ? err.message : 'Unknown error')
      setSearchStatus('error')
    }
  }

  function handleReset() {
    setCategory('')
    setLimit('25')
    setResults([])
    onResults([])
    setSearchStatus('idle')
    setSearchError('')
    setActiveChunkId(null)
    setPreprocessStatus('idle')
    setPreprocessMsg('')
    inputRef.current?.focus()
  }

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* Header */}
      <div className="px-5 py-4 border-b border-gray-200 shrink-0">
        <h1 className="text-sm font-medium text-gray-700 tracking-tight">Semantic Geo Search</h1>
        <p className="text-xs text-gray-400 mt-0.5">Explore locations by meaning</p>
      </div>

      <div className="flex-1 overflow-y-auto">
        {/* ── Query form ─────────────────────────────────────────────────── */}
        <form onSubmit={handleSearch} className="px-5 py-4 border-b border-gray-200 space-y-3">
          {/* Category */}
          <div>
            <label className="block text-xs text-gray-400 mb-1">Category</label>
            <div className="relative">
              <input
                ref={inputRef}
                type="text"
                value={category}
                onChange={e => setCategory(e.target.value)}
                placeholder="e.g. pizza_restaurant"
                className="w-full text-sm bg-white border border-gray-200 rounded-md px-3 py-2 pr-7 text-gray-800 placeholder-gray-400 outline-none focus:border-gray-400 transition-colors"
              />
              {category && (
                <button
                  type="button"
                  onClick={() => setCategory('')}
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-300 hover:text-gray-500 transition-colors"
                >
                  ✕
                </button>
              )}
            </div>
          </div>

          {/* Limit */}
          <div>
            <label className="block text-xs text-gray-400 mb-1">Limit</label>
            <input
              type="number"
              min={1}
              max={1000}
              value={limit}
              onChange={e => setLimit(e.target.value)}
              className="w-full text-sm bg-white border border-gray-200 rounded-md px-3 py-2 text-gray-800 outline-none focus:border-gray-400 transition-colors"
            />
          </div>

          {/* BBox readout */}
          <div>
            <label className="block text-xs text-gray-400 mb-1">Bounding box</label>
            {bbox ? (
              <div className="font-mono text-[11px] text-gray-500 bg-gray-50 border border-gray-200 rounded-md px-3 py-2 leading-relaxed">
                <span className="text-gray-400">W</span> {bbox.minX.toFixed(4)}&ensp;
                <span className="text-gray-400">E</span> {bbox.maxX.toFixed(4)}&ensp;
                <span className="text-gray-400">S</span> {bbox.minY.toFixed(4)}&ensp;
                <span className="text-gray-400">N</span> {bbox.maxY.toFixed(4)}
              </div>
            ) : (
              <div className="text-[11px] text-gray-400 bg-gray-50 border border-dashed border-gray-200 rounded-md px-3 py-2">
                Hold{' '}
                <kbd className="font-mono bg-white border border-gray-200 rounded px-1">Shift</kbd>
                {' '}+ drag on the map
              </div>
            )}
          </div>

          {/* Action buttons */}
          <div className="flex gap-2 pt-1">
            <button
              type="submit"
              disabled={searchStatus === 'loading' || !canSearch}
              className="flex-1 py-2 text-sm bg-gray-800 text-white rounded-md hover:bg-gray-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              {searchStatus === 'loading' && !activeChunkId ? 'Searching…' : 'Search'}
            </button>
            <button
              type="button"
              onClick={handlePreprocess}
              disabled={!canPreprocess}
              title="Precompute this bbox as a reusable chunk"
              className="px-3 py-2 text-sm text-gray-600 border border-gray-200 rounded-md hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed transition-colors whitespace-nowrap"
            >
              {preprocessStatus === 'loading' ? 'Computing…' : '+ Precompute'}
            </button>
            {(searchStatus === 'success' || searchStatus === 'error') && (
              <button
                type="button"
                onClick={handleReset}
                className="px-3 py-2 text-sm text-gray-400 border border-gray-200 rounded-md hover:bg-gray-50 transition-colors"
              >
                Reset
              </button>
            )}
          </div>

          {/* Preprocess feedback */}
          {preprocessMsg && (
            <p className={`text-[11px] ${preprocessStatus === 'error' ? 'text-red-400' : 'text-gray-400'}`}>
              {preprocessMsg}
            </p>
          )}
        </form>

        {/* ── Precomputed chunks ─────────────────────────────────────────── */}
        <div className="px-5 py-4 border-b border-gray-200">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs text-gray-400">Precomputed chunks</span>
            <button
              onClick={loadChunks}
              disabled={chunksLoading}
              className="text-[10px] text-gray-400 hover:text-gray-600 transition-colors disabled:opacity-40"
            >
              {chunksLoading ? 'Loading…' : 'Refresh'}
            </button>
          </div>

          {chunks.length === 0 && !chunksLoading && (
            <p className="text-[11px] text-gray-400">
              No chunks yet — precompute a bbox above to speed up repeated searches.
            </p>
          )}

          {chunks.length > 0 && (
            <ul className="space-y-1.5">
              {chunks.map(c => (
                <ChunkRow
                  key={c.chunk_id}
                  chunk={c}
                  active={activeChunkId === c.chunk_id}
                  loading={searchStatus === 'loading' && activeChunkId === c.chunk_id}
                  onSearch={() => handleChunkSearch(c.chunk_id)}
                />
              ))}
            </ul>
          )}
        </div>

        {/* ── Results ───────────────────────────────────────────────────── */}
        <div>
          {searchStatus === 'idle' && (
            <div className="flex flex-col items-center justify-center py-16 gap-2 text-center px-6">
              <div className="w-9 h-9 rounded-full border border-dashed border-gray-300 flex items-center justify-center text-gray-300 text-xl">
                ⌕
              </div>
              <p className="text-xs text-gray-400">
                {!bbox && !category ? 'Draw a bounding box and enter a category'
                  : !bbox           ? 'Draw a bounding box on the map'
                  : !category       ? 'Enter a category to search'
                  :                   'Ready — hit Search or use a chunk'}
              </p>
            </div>
          )}

          {searchStatus === 'loading' && (
            <div className="flex items-center justify-center py-16">
              <div className="flex gap-1.5">
                <span className="w-1.5 h-1.5 rounded-full bg-gray-300 animate-bounce [animation-delay:-0.3s]" />
                <span className="w-1.5 h-1.5 rounded-full bg-gray-300 animate-bounce [animation-delay:-0.15s]" />
                <span className="w-1.5 h-1.5 rounded-full bg-gray-300 animate-bounce" />
              </div>
            </div>
          )}

          {searchStatus === 'error' && (
            <div className="mx-5 mt-5 px-4 py-3 bg-red-50 border border-red-100 rounded-md">
              <p className="text-xs text-red-500">{searchError}</p>
            </div>
          )}

          {searchStatus === 'success' && results.length === 0 && (
            <div className="flex flex-col items-center justify-center py-16 gap-1 text-center px-6">
              <p className="text-sm text-gray-500">No results found</p>
              <p className="text-xs text-gray-400">Try a different category or area</p>
            </div>
          )}

          {searchStatus === 'success' && results.length > 0 && (
            <div className="px-5 py-3">
              <p className="text-xs text-gray-400 mb-3">
                {results.length} result{results.length !== 1 ? 's' : ''}
                {activeChunkId && <span className="ml-1 text-gray-300">· from chunk</span>}
              </p>
              <ul className="space-y-2">
                {results.map(r => <ResultCard key={r.id} restaurant={r} />)}
              </ul>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

// ── Sub-components ───────────────────────────────────────────────────────────

function ChunkRow({
  chunk: c,
  active,
  loading,
  onSearch,
}: {
  chunk: ChunkInfo
  active: boolean
  loading: boolean
  onSearch: () => void
}) {
  return (
    <li className={`flex items-center justify-between gap-2 px-3 py-2 rounded-md border text-[11px] transition-colors
      ${active ? 'border-gray-400 bg-gray-50' : 'border-gray-200 bg-white hover:border-gray-300'}`}>
      <div className="min-w-0">
        <p className="text-gray-700 truncate">{c.category}</p>
        <p className="text-gray-400 font-mono truncate">
          {c.bbox_min_x.toFixed(2)},{c.bbox_min_y.toFixed(2)} → {c.bbox_max_x.toFixed(2)},{c.bbox_max_y.toFixed(2)}
        </p>
        <p className="text-gray-400">{c.record_count} records</p>
      </div>
      <button
        onClick={onSearch}
        disabled={loading}
        className="shrink-0 px-2.5 py-1 text-[11px] text-gray-600 border border-gray-200 rounded hover:bg-gray-100 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
      >
        {loading ? '…' : 'Search'}
      </button>
    </li>
  )
}

function ResultCard({ restaurant: r }: { restaurant: Restaurant }) {
  const socialEntries = r.socials ? Object.entries(r.socials) : []
  const pct = Math.round(r.confidence * 100)
  return (
    <li className="border border-gray-200 rounded-md px-4 py-3 bg-white hover:border-gray-300 transition-colors">
      <div className="flex items-start justify-between gap-3">
        <span className="text-sm text-gray-800 leading-snug">{r.name || '—'}</span>
        <ConfidenceBadge value={pct} />
      </div>
      {socialEntries.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1">
          {socialEntries.map(([platform, handle]) => (
            <a
              key={platform}
              href={handle.startsWith('http') ? handle : `https://${handle}`}
              target="_blank"
              rel="noreferrer"
              className="text-xs text-gray-400 hover:text-gray-600 transition-colors capitalize"
            >
              {platform}
            </a>
          ))}
        </div>
      )}
      <p className="mt-2 text-[10px] text-gray-300 font-mono truncate" title={r.geometry}>
        {r.geometry}
      </p>
    </li>
  )
}

function ConfidenceBadge({ value }: { value: number }) {
  const cls =
    value >= 80 ? 'text-emerald-600 bg-emerald-50' :
    value >= 50 ? 'text-amber-600 bg-amber-50' :
                  'text-gray-400 bg-gray-100'
  return (
    <span className={`shrink-0 text-[10px] px-1.5 py-0.5 rounded font-medium tabular-nums ${cls}`}>
      {value}%
    </span>
  )
}
