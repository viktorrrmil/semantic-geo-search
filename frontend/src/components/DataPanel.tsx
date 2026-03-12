import { useState } from 'react'
import type { SelectedFile } from './S3Explorer'

const API = 'http://localhost:3001/api/v1'

export interface SpatialFeature {
  geometry: string
  properties: Record<string, unknown>
}

type FetchStatus = 'idle' | 'loading' | 'success' | 'error'

interface Props {
  selectedFile: SelectedFile | null
  onOpenS3Browser: () => void
  onFeaturesChange: (features: SpatialFeature[]) => void
}

export default function DataPanel({ selectedFile, onOpenS3Browser, onFeaturesChange }: Props) {
  const [k, setK] = useState('10')
  const [features, setFeatures] = useState<SpatialFeature[]>([])
  const [status, setStatus] = useState<FetchStatus>('idle')
  const [error, setError] = useState('')

  async function handleFetch() {
    if (!selectedFile) return
    setStatus('loading')
    setError('')
    try {
      const params = new URLSearchParams({
        path: selectedFile.path,
        k: String(Math.max(1, parseInt(k, 10) || 10)),
        region: selectedFile.region,
      })
      const res = await fetch(`${API}/spatial-data?${params}`)
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        throw new Error(body.error ?? `Server responded with ${res.status}`)
      }
      const data = await res.json()
      const rows: Record<string, unknown>[] = data.rows ?? []
      const parsed: SpatialFeature[] = rows.map(row => {
        const { geometry, ...props } = row
        return {
          geometry: typeof geometry === 'string' ? geometry : '',
          properties: props,
        }
      })
      setFeatures(parsed)
      onFeaturesChange(parsed)
      setStatus('success')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
      setStatus('error')
    }
  }

  function handleClear() {
    setFeatures([])
    onFeaturesChange([])
    setStatus('idle')
    setError('')
  }

  const geomCounts = countGeomTypes(features)

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* Header */}
      <div className="px-5 py-4 border-b border-gray-200 shrink-0">
        <h1 className="text-sm font-medium text-gray-700 tracking-tight">Data Explorer</h1>
        <p className="text-xs text-gray-400 mt-0.5">Fetch rows from a Parquet file</p>
      </div>

      <div className="flex-1 overflow-y-auto">

        {/* ── Selected file ─────────────────────────────────────────────── */}
        <div className="px-5 py-4 border-b border-gray-200 space-y-3">
          <label className="block text-xs text-gray-400">Selected file</label>

          {selectedFile ? (
            <div className="flex items-start gap-2.5 px-3 py-2.5 bg-blue-50 border border-blue-200 rounded-md">
              <svg width="14" height="14" viewBox="0 0 14 14" fill="none" className="mt-0.5 shrink-0 text-blue-500">
                <rect x="2" y="1" width="8" height="12" rx="1" fill="currentColor" fillOpacity=".2" stroke="currentColor" strokeWidth="1" />
                <path d="M8 1v3.5H11" stroke="currentColor" strokeWidth="1" strokeLinecap="round" />
                <path d="M4 7h5M4 9.5h3.5" stroke="currentColor" strokeWidth="0.8" strokeLinecap="round" />
              </svg>
              <div className="min-w-0">
                <p className="text-xs font-medium text-blue-700 truncate">{selectedFile.name}</p>
                <p className="text-[10px] text-blue-500 font-mono break-all leading-relaxed mt-0.5">
                  {selectedFile.path}
                </p>
              </div>
            </div>
          ) : (
            <div className="flex flex-col items-center py-8 gap-3 text-center">
              <div className="w-10 h-10 rounded-full border border-dashed border-gray-300 flex items-center justify-center">
                <svg width="18" height="18" viewBox="0 0 18 18" fill="none" className="text-gray-300">
                  <path d="M2 4.5C2 3.12 3.12 2 4.5 2h3.086c.265 0 .52.105.707.293L9.5 3.5h4C14.88 3.5 16 4.62 16 6v7.5C16 14.88 14.88 16 13.5 16h-9C3.12 16 2 14.88 2 13.5v-9z" fill="currentColor" fillOpacity=".2" stroke="currentColor" strokeWidth="1.2" />
                </svg>
              </div>
              <p className="text-xs text-gray-400 leading-relaxed">
                No file selected.<br />Open the S3 browser and click a file.
              </p>
              <button
                onClick={onOpenS3Browser}
                className="text-xs text-gray-600 border border-gray-200 hover:border-gray-400 hover:bg-gray-50 px-3 py-1.5 rounded-md transition-colors"
              >
                Open S3 Browser →
              </button>
            </div>
          )}

          {/* Fetch controls */}
          {selectedFile && (
            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <label className="text-xs text-gray-400 shrink-0 w-20">Rows (K)</label>
                <input
                  type="number"
                  min={1}
                  max={10000}
                  value={k}
                  onChange={e => setK(e.target.value)}
                  className="w-24 text-sm bg-white border border-gray-200 rounded-md px-3 py-1.5 text-gray-800 outline-none focus:border-gray-400 transition-colors"
                />
              </div>
              <div className="flex gap-2 pt-1">
                <button
                  onClick={handleFetch}
                  disabled={status === 'loading'}
                  className="flex-1 py-2 text-sm bg-gray-800 text-white rounded-md hover:bg-gray-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                >
                  {status === 'loading' ? 'Fetching…' : 'Fetch'}
                </button>
                {(status === 'success' || status === 'error') && (
                  <button
                    onClick={handleClear}
                    className="px-3 py-2 text-sm text-gray-400 border border-gray-200 rounded-md hover:bg-gray-50 transition-colors"
                  >
                    Clear
                  </button>
                )}
              </div>
            </div>
          )}
        </div>

        {/* ── Results ───────────────────────────────────────────────────── */}
        {status === 'error' && (
          <div className="mx-5 mt-5 px-4 py-3 bg-red-50 border border-red-100 rounded-md">
            <p className="text-xs text-red-500">{error}</p>
          </div>
        )}

        {status === 'loading' && (
          <div className="flex items-center justify-center py-16">
            <div className="flex gap-1.5">
              <span className="w-1.5 h-1.5 rounded-full bg-gray-300 animate-bounce [animation-delay:-0.3s]" />
              <span className="w-1.5 h-1.5 rounded-full bg-gray-300 animate-bounce [animation-delay:-0.15s]" />
              <span className="w-1.5 h-1.5 rounded-full bg-gray-300 animate-bounce" />
            </div>
          </div>
        )}

        {status === 'success' && features.length === 0 && (
          <div className="flex flex-col items-center justify-center py-12 gap-1 text-center px-6">
            <p className="text-sm text-gray-500">No features returned</p>
            <p className="text-xs text-gray-400">The file may be empty or have no geometry</p>
          </div>
        )}

        {status === 'success' && features.length > 0 && (
          <div className="px-5 py-3">
            {/* Summary row */}
            <div className="flex items-center gap-2 mb-3 flex-wrap">
              <p className="text-xs text-gray-500 font-medium">
                {features.length} feature{features.length !== 1 ? 's' : ''}
              </p>
              {Object.entries(geomCounts).map(([type, count]) => (
                <GeomBadge key={type} type={type} count={count} />
              ))}
            </div>

            {/* Feature list */}
            <ul className="space-y-1.5">
              {features.map((f, i) => (
                <FeatureRow key={i} feature={f} index={i} />
              ))}
            </ul>
          </div>
        )}
      </div>
    </div>
  )
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function wktGeomType(wkt: string): string {
  if (!wkt) return 'unknown'
  const m = wkt.match(/^(\w+)/i)
  return m ? m[1].toLowerCase() : 'unknown'
}

function countGeomTypes(features: SpatialFeature[]): Record<string, number> {
  const counts: Record<string, number> = {}
  for (const f of features) {
    const t = wktGeomType(f.geometry)
    counts[t] = (counts[t] ?? 0) + 1
  }
  return counts
}

function getPrimaryLabel(props: Record<string, unknown>): string {
  // names.primary (could be object or JSON string)
  const names = props.names
  if (typeof names === 'object' && names !== null) {
    const n = names as Record<string, unknown>
    if (typeof n.primary === 'string') return n.primary
  }
  if (typeof names === 'string' && names.startsWith('{')) {
    try {
      const parsed = JSON.parse(names)
      if (typeof parsed.primary === 'string') return parsed.primary
    } catch { /* ignore */ }
  }
  if (typeof props.name === 'string' && props.name) return props.name
  return ''
}

function formatVal(v: unknown): string {
  if (v === null || v === undefined) return '—'
  if (typeof v === 'number') return String(v)
  if (typeof v === 'boolean') return String(v)
  if (typeof v === 'string') {
    // Summarize JSON strings
    if (v.startsWith('{') || v.startsWith('[')) {
      try {
        const p = JSON.parse(v)
        if (Array.isArray(p)) return `[${p.length} items]`
        const primary = (p as Record<string, unknown>).primary
        if (typeof primary === 'string') return primary
        const keys = Object.keys(p)
        if (keys.length > 0) return String((p as Record<string, unknown>)[keys[0]]).slice(0, 40)
      } catch { /* ignore */ }
    }
    return v.slice(0, 50) + (v.length > 50 ? '…' : '')
  }
  if (typeof v === 'object') {
    const o = v as Record<string, unknown>
    if ('primary' in o && typeof o.primary === 'string') return o.primary
    return JSON.stringify(v).slice(0, 50) + (JSON.stringify(v).length > 50 ? '…' : '')
  }
  return String(v).slice(0, 50)
}

const DISPLAY_KEYS = ['id', 'class', 'subtype', 'confidence', 'height', 'level', 'category']

function getDisplayPairs(props: Record<string, unknown>): Array<[string, string]> {
  const pairs: Array<[string, string]> = []

  const label = getPrimaryLabel(props)
  if (label) pairs.push(['name', label])

  for (const key of DISPLAY_KEYS) {
    if (pairs.length >= 3) break
    if (key in props && props[key] !== null && props[key] !== undefined && props[key] !== '') {
      pairs.push([key, formatVal(props[key])])
    }
  }

  // Fill remaining slots with any non-null prop
  if (pairs.length < 2) {
    for (const [k, v] of Object.entries(props)) {
      if (pairs.length >= 3) break
      if (pairs.some(([pk]) => pk === k)) continue
      if (v !== null && v !== undefined && v !== '') {
        pairs.push([k, formatVal(v)])
      }
    }
  }

  return pairs.slice(0, 3)
}

// ── Sub-components ────────────────────────────────────────────────────────────

const GEOM_STYLES: Record<string, { label: string; cls: string }> = {
  point:              { label: 'Point',         cls: 'text-blue-600 bg-blue-50 border-blue-200' },
  multipoint:         { label: 'MultiPoint',    cls: 'text-blue-600 bg-blue-50 border-blue-200' },
  linestring:         { label: 'Line',          cls: 'text-orange-600 bg-orange-50 border-orange-200' },
  multilinestring:    { label: 'MultiLine',     cls: 'text-orange-600 bg-orange-50 border-orange-200' },
  polygon:            { label: 'Polygon',       cls: 'text-indigo-600 bg-indigo-50 border-indigo-200' },
  multipolygon:       { label: 'MultiPolygon',  cls: 'text-indigo-600 bg-indigo-50 border-indigo-200' },
}

function GeomBadge({ type, count }: { type: string; count: number }) {
  const style = GEOM_STYLES[type] ?? { label: type, cls: 'text-gray-500 bg-gray-100 border-gray-200' }
  return (
    <span className={`text-[10px] px-1.5 py-0.5 rounded border font-medium ${style.cls}`}>
      {style.label} ×{count}
    </span>
  )
}

function FeatureRow({ feature: f, index }: { feature: SpatialFeature; index: number }) {
  const type = wktGeomType(f.geometry)
  const style = GEOM_STYLES[type] ?? { label: type, cls: 'text-gray-400 bg-gray-100 border-gray-200' }
  const pairs = getDisplayPairs(f.properties)

  return (
    <li className="border border-gray-200 rounded-md px-3 py-2.5 bg-white hover:border-gray-300 transition-colors">
      <div className="flex items-center gap-2 mb-1.5">
        <span className={`text-[9px] px-1.5 py-0.5 rounded border font-medium uppercase tracking-wide ${style.cls}`}>
          {style.label}
        </span>
        <span className="text-[10px] text-gray-300 font-mono">#{index + 1}</span>
      </div>
      {pairs.length > 0 ? (
        <div className="space-y-0.5">
          {pairs.map(([k, v]) => (
            <div key={k} className="flex gap-1.5 items-baseline">
              <span className="text-[10px] text-gray-400 shrink-0 w-16 truncate">{k}</span>
              <span className="text-[11px] text-gray-700 truncate">{v}</span>
            </div>
          ))}
        </div>
      ) : (
        <p className="text-[10px] text-gray-400 italic">No displayable properties</p>
      )}
    </li>
  )
}
