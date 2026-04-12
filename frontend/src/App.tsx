import {useEffect, useRef, useState, type FormEvent} from 'react'
import MapView, {type BBox, type IndexedArea} from './components/MapView'
import S3Explorer, {type FileItem, type SelectedFile} from './components/S3Explorer'
import type {GeoJSONGeometry, Geometry, SpatialFeature} from './components/DataPanel'
import './App.css'

const API = 'http://localhost:3001/api/v1'
const SEARCH_API = 'http://localhost:3001'
const INDEXED_AREAS_API = 'http://localhost:3001/api/v1/geo/indexed-areas'
const QUERY_EXPANSION_API = 'http://localhost:3001/api/v1/query-expansion'
const DEFAULT_TOP_K = 5
const TOP_K_PRESETS = [5, 10, 25, 50]

type Page = 'main' | 'index'
type Status = 'idle' | 'loading' | 'success' | 'error'

interface SearchResult {
    id: string
    embed_text: string
    geom: Geometry
    category?: string | null
    country?: string | null
    confidence?: number | null
    raw?: unknown
}

interface SearchResponseEnvelope {
    expanded_query?: unknown
    results?: unknown
    rows?: unknown
}

interface QueryExpansionConfigResponse {
    query_expansion_enabled?: unknown
    enabled?: unknown
}

interface IndexedAreaRow {
    id?: string | number
    source?: string | null
    bbox_min_x?: number | string | null
    bbox_max_x?: number | string | null
    bbox_min_y?: number | string | null
    bbox_max_y?: number | string | null
    bbox?: {
        min_x?: number | string | null
        max_x?: number | string | null
        min_y?: number | string | null
        max_y?: number | string | null
    } | null
    total_points?: number | string | null
    indexed_points?: number | string | null
    indexed_percent?: number | string | null
}

function isGeoJSONGeometry(value: unknown): value is GeoJSONGeometry {
    return typeof value === 'object' && value !== null
        && 'type' in value && typeof (value as GeoJSONGeometry).type === 'string'
        && 'coordinates' in value
}

function toNumber(value: unknown): number | null {
    if (typeof value === 'number' && Number.isFinite(value)) return value
    if (typeof value === 'string' && value.trim() !== '') {
        const parsed = Number(value)
        if (Number.isFinite(parsed)) return parsed
    }
    return null
}

function normalizeIndexedArea(row: IndexedAreaRow, index: number): IndexedArea | null {
    const minX = toNumber(row.bbox?.min_x ?? row.bbox_min_x)
    const maxX = toNumber(row.bbox?.max_x ?? row.bbox_max_x)
    const minY = toNumber(row.bbox?.min_y ?? row.bbox_min_y)
    const maxY = toNumber(row.bbox?.max_y ?? row.bbox_max_y)
    if (minX == null || maxX == null || minY == null || maxY == null) return null

    const source = typeof row.source === 'string' ? row.source : ''
    const id = row.id != null
        ? String(row.id)
        : `${source || 'area'}-${minX}-${minY}-${maxX}-${maxY}`
    const totalPoints = toNumber(row.total_points)
    const indexedPoints = toNumber(row.indexed_points)
    const indexedPercent = toNumber(row.indexed_percent)

    return {
        id,
        label: source || `Area ${index + 1}`,
        bbox: {minX, minY, maxX, maxY},
        source: source || undefined,
        totalPoints: totalPoints ?? undefined,
        indexedPoints: indexedPoints ?? undefined,
        indexedPercent: indexedPercent ?? undefined,
        active: true,
    }
}

function parseIndexedAreasResponse(data: unknown): IndexedArea[] {
    const rows = Array.isArray(data)
        ? data
        : (data && typeof data === 'object' && Array.isArray((data as {areas?: unknown}).areas))
            ? (data as {areas: unknown[]}).areas
            : null
    if (!rows) throw new Error('Unexpected response format')
    return rows
        .map((row, index) => normalizeIndexedArea(row as IndexedAreaRow, index))
        .filter((area): area is IndexedArea => area !== null)
}

function parseQueryExpansionEnabled(data: unknown): boolean {
    if (data && typeof data === 'object') {
        const payload = data as QueryExpansionConfigResponse
        if (typeof payload.query_expansion_enabled === 'boolean') return payload.query_expansion_enabled
        if (typeof payload.enabled === 'boolean') return payload.enabled
    }
    return false
}

function mergeActiveState(prev: IndexedArea[], next: IndexedArea[]) {
    if (prev.length === 0) return next
    const prevActive = new Map(prev.map(area => [area.id, area.active]))
    return next.map(area => (
        prevActive.has(area.id)
            ? {...area, active: prevActive.get(area.id)}
            : area
    ))
}

function normalizeSearchResult(row: Record<string, unknown>, index: number): SearchResult {
    const id = typeof row.id === 'string' ? row.id : (row.id != null ? String(row.id) : '')
    const geom = typeof row.geom === 'string'
        ? row.geom
        : (isGeoJSONGeometry(row.geom) ? row.geom : '')
    return {
        id: id || `row-${index}`,
        embed_text: typeof row.embed_text === 'string' ? row.embed_text : '',
        geom,
        category: typeof row.category === 'string' ? row.category : null,
        country: typeof row.country === 'string' ? row.country : null,
        confidence: typeof row.confidence === 'number' ? row.confidence : null,
        raw: row.raw,
    }
}

function parseSearchResultsResponse(data: unknown): {rows: Record<string, unknown>[], expandedQuery: string | null} {
    if (Array.isArray(data)) {
        return {rows: data as Record<string, unknown>[], expandedQuery: null}
    }
    if (data && typeof data === 'object') {
        const payload = data as SearchResponseEnvelope
        const expandedQuery = typeof payload.expanded_query === 'string' && payload.expanded_query.trim() !== ''
            ? payload.expanded_query
            : null
        if (Array.isArray(payload.results)) {
            return {rows: payload.results as Record<string, unknown>[], expandedQuery}
        }
        if (Array.isArray(payload.rows)) {
            return {rows: payload.rows as Record<string, unknown>[], expandedQuery}
        }
    }
    throw new Error('Unexpected response format')
}

function toSpatialFeature(row: SearchResult): SpatialFeature {
    const props: Record<string, unknown> = {
        id: row.id,
        name: row.embed_text,
        embed_text: row.embed_text,
    }
    if (row.category) props.category = row.category
    if (row.country) props.country = row.country
    if (row.confidence != null) props.confidence = row.confidence
    if (row.raw != null) props.raw = row.raw
    return {
        geometry: row.geom ?? '',
        properties: props,
    }
}

function resolveTopK(value: string) {
    const parsed = parseInt(value, 10)
    return Math.max(1, Number.isFinite(parsed) ? parsed : DEFAULT_TOP_K)
}

function formatConfidence(value: number | null | undefined) {
    if (value == null || Number.isNaN(value)) return null
    const pct = value <= 1 ? Math.round(value * 100) : Math.round(value)
    return `${pct}%`
}

function resolveIndexedPercent(area: IndexedArea): number | null {
    if (typeof area.indexedPercent === 'number' && Number.isFinite(area.indexedPercent)) {
        return Math.round(area.indexedPercent)
    }
    const total = area.totalPoints
    const indexed = area.indexedPoints
    if (typeof total === 'number' && Number.isFinite(total) && typeof indexed === 'number' && Number.isFinite(indexed) && total > 0) {
        return Math.round((indexed / total) * 100)
    }
    return null
}

function App() {
    const [page, setPage] = useState<Page>('main')
    const [indexedAreas, setIndexedAreas] = useState<IndexedArea[]>([])
    const [indexedAreasStatus, setIndexedAreasStatus] = useState<Status>('idle')
    const [indexedAreasError, setIndexedAreasError] = useState('')
    const [selectedIndexedAreaId, setSelectedIndexedAreaId] = useState<string | null>(null)
    const [searchQuery, setSearchQuery] = useState('')
    const [queryExpansionAvailable, setQueryExpansionAvailable] = useState(false)
    const [queryExpansionEnabled, setQueryExpansionEnabled] = useState(false)
    const [expandedQuery, setExpandedQuery] = useState<string | null>(null)
    const [topK, setTopK] = useState(String(DEFAULT_TOP_K))
    const [showTopKMenu, setShowTopKMenu] = useState(false)
    const [searchStatus, setSearchStatus] = useState<Status>('idle')
    const [searchError, setSearchError] = useState('')
    const [searchResults, setSearchResults] = useState<SearchResult[]>([])
    const [searchFeatures, setSearchFeatures] = useState<SpatialFeature[]>([])
    const [selectedResultId, setSelectedResultId] = useState<string | null>(null)
    const [focusedFeature, setFocusedFeature] = useState<SpatialFeature | null>(null)
    const [showIndexedOverlay, setShowIndexedOverlay] = useState(false)
    const [showSatellite, setShowSatellite] = useState(false)

    const [selectedFile, setSelectedFile] = useState<SelectedFile | null>(null)
    const [selectionBBox, setSelectionBBox] = useState<BBox | null>(null)
    const [rowCount, setRowCount] = useState('')
    const [indexStatus, setIndexStatus] = useState<Status>('idle')
    const [indexMsg, setIndexMsg] = useState('')

    const topKMenuRef = useRef<HTMLDivElement>(null)
    const selectedResult = selectedResultId
        ? searchResults.find(result => result.id === selectedResultId) ?? null
        : null
    const selectedIndexedArea = selectedIndexedAreaId
        ? indexedAreas.find(area => area.id === selectedIndexedAreaId) ?? null
        : null

    useEffect(() => {
        if (!showTopKMenu) return
        const handler = (event: MouseEvent) => {
            if (!topKMenuRef.current) return
            if (!topKMenuRef.current.contains(event.target as Node)) {
                setShowTopKMenu(false)
            }
        }
        document.addEventListener('mousedown', handler)
        return () => document.removeEventListener('mousedown', handler)
    }, [showTopKMenu])

    useEffect(() => {
        if (!selectedIndexedAreaId) return
        if (!indexedAreas.some(area => area.id === selectedIndexedAreaId)) {
            setSelectedIndexedAreaId(null)
        }
    }, [indexedAreas, selectedIndexedAreaId])

    useEffect(() => {
        const controller = new AbortController()
        void loadIndexedAreas(controller.signal)
        return () => controller.abort()
    }, [])

    useEffect(() => {
        const controller = new AbortController()
        void loadQueryExpansionConfig(controller.signal)
        return () => controller.abort()
    }, [])

    useEffect(() => {
        if (!queryExpansionAvailable) {
            setQueryExpansionEnabled(false)
        }
    }, [queryExpansionAvailable])

    useEffect(() => {
        if (!queryExpansionEnabled) {
            setExpandedQuery(null)
        }
    }, [queryExpansionEnabled])

    async function loadIndexedAreas(signal?: AbortSignal) {
        setIndexedAreasStatus('loading')
        setIndexedAreasError('')
        try {
            const res = await fetch(INDEXED_AREAS_API, {signal})
            if (!res.ok) throw new Error(`Server responded with ${res.status}`)
            const data: unknown = await res.json()
            const parsed = parseIndexedAreasResponse(data)
            setIndexedAreas(prev => mergeActiveState(prev, parsed))
            setIndexedAreasStatus('success')
        } catch (err) {
            if (err instanceof DOMException && err.name === 'AbortError') return
            setIndexedAreasError(err instanceof Error ? err.message : 'Failed to load indexed areas')
            setIndexedAreasStatus('error')
        }
    }

    async function loadQueryExpansionConfig(signal?: AbortSignal) {
        try {
            const res = await fetch(QUERY_EXPANSION_API, {signal})
            if (!res.ok) return
            const data: unknown = await res.json()
            setQueryExpansionAvailable(parseQueryExpansionEnabled(data))
        } catch {
            // Silently ignore; query expansion stays hidden/disabled.
        }
    }

    async function handleSearchSubmit(e: FormEvent) {
        e.preventDefault()
        const query = searchQuery.trim()
        if (!query) {
            setSearchError('Enter a search query to search all indexed areas.')
            setSearchStatus('error')
            return
        }
        setShowTopKMenu(false)
        setSearchStatus('loading')
        setSearchError('')
        setSelectedResultId(null)
        setFocusedFeature(null)
        setExpandedQuery(null)
        try {
            const resolvedTopK = resolveTopK(topK)
            const useQueryExpansion = queryExpansionAvailable && queryExpansionEnabled
            console.log(`Searching for "${query}" with topK=${resolvedTopK}...`)
            const res = await fetch(`${SEARCH_API}/search`, {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({
                    query,
                    count: resolvedTopK,
                    ...(useQueryExpansion ? {expand: true} : {}),
                }),
            })
            if (!res.ok) throw new Error(`Server responded with ${res.status}`)
            const data: unknown = await res.json()
            const {rows, expandedQuery: responseExpandedQuery} = parseSearchResultsResponse(data)
            const parsed = rows.map((row, index) => normalizeSearchResult(row as Record<string, unknown>, index))
            const trimmed = parsed.slice(0, resolvedTopK)
            const mapped = trimmed.map(toSpatialFeature)
            setExpandedQuery(useQueryExpansion ? responseExpandedQuery : null)
            console.log('Search results:', trimmed)
            setSearchResults(trimmed)
            setSearchFeatures(mapped)
            setSearchStatus('success')
        } catch (err) {
            setSearchError(err instanceof Error ? err.message : 'Search failed')
            setSearchResults([])
            setSearchFeatures([])
            setExpandedQuery(null)
            setSearchStatus('error')
        }
    }

    function handleSearchClear() {
        setSearchQuery('')
        setSearchError('')
        setSearchResults([])
        setSearchFeatures([])
        setSelectedResultId(null)
        setFocusedFeature(null)
        setExpandedQuery(null)
        setShowTopKMenu(false)
        setSearchStatus('idle')
    }

    function handleToggleArea(id: string) {
        setIndexedAreas(prev => prev.map(area => area.id === id ? {...area, active: area.active === false} : area))
    }

    function handleIndexSelection() {
        if (!selectedFile || !selectionBBox) return
        setIndexMsg('')
        const count = parseInt(rowCount, 10)
        const files: FileItem[] = selectedFile.type === 'folder'
            ? (selectedFile.files ?? [])
            : [{path: selectedFile.path, name: selectedFile.name, region: selectedFile.region}]

        if (files.length === 0) {
            setIndexMsg('No files selected for indexing.')
            setIndexStatus('error')
            return
        }

        setIndexStatus('success')
        setIndexMsg('Indexing has started.')

        for (const file of files) {
            const payload: Record<string, unknown> = {
                s3_path: file.path,
                region: file.region,
                bbox_min_x: selectionBBox.minX,
                bbox_max_x: selectionBBox.maxX,
                bbox_min_y: selectionBBox.minY,
                bbox_max_y: selectionBBox.maxY,
                all: rowCount.trim() === '',
            }
            if (!isNaN(count) && rowCount.trim() !== '') {
                payload.count = count
            }
            void fetch(`${API}/index-file`, {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify(payload),
            }).then(res => {
                if (!res.ok) throw new Error(`Server responded with ${res.status}`)
            }).catch(err => {
                console.error('Failed to start indexing', err)
                setIndexStatus('error')
                setIndexMsg('Indexing started, but one or more requests failed to start.')
            })
        }

        void loadIndexedAreas()
    }

    const canIndex = !!selectedFile && !!selectionBBox

    return (
        <div className="h-screen w-screen bg-[#f8f8f7]">
            {page === 'main' ? (
                <div className="relative h-full w-full overflow-hidden">
                    <MapView
                        features={searchFeatures}
                        indexedAreas={indexedAreas}
                        dimMap={showIndexedOverlay}
                        focusedFeature={focusedFeature}
                        focusedBBox={selectedIndexedArea?.bbox ?? null}
                        baseMap={showSatellite ? 'satellite' : 'osm'}
                    />

                    <div className="absolute top-4 right-4 z-40">
                        <button
                            onClick={() => setPage('index')}
                            className="text-[11px] text-gray-500 bg-white/90 border border-gray-200 rounded-md px-3 py-1.5 shadow-sm hover:border-gray-300 hover:text-gray-700 transition-colors"
                        >
                            Open indexing dashboard
                        </button>
                    </div>

                    <div className="absolute top-4 left-1/2 -translate-x-1/2 z-40 max-w-[90vw]">
                        <div className="flex flex-col items-center gap-2">
                            <div className="flex items-center gap-2">
                            <form
                                onSubmit={handleSearchSubmit}
                                className="flex items-center gap-2 bg-white/90 border border-gray-200 rounded-full shadow-sm px-3 py-2 w-[420px] max-w-[70vw]"
                            >
                            <input
                                type="text"
                                value={searchQuery}
                                onChange={e => setSearchQuery(e.target.value)}
                                placeholder="Search across indexed areas"
                                className="flex-1 bg-transparent text-sm text-gray-700 placeholder-gray-400 outline-none"
                            />
                            {searchQuery && (
                                <button
                                    type="button"
                                    onClick={handleSearchClear}
                                    className="text-gray-300 hover:text-gray-500 transition-colors"
                                >
                                    ✕
                                </button>
                            )}
                            <button
                                type="submit"
                                disabled={!searchQuery.trim() || searchStatus === 'loading'}
                                className="text-[11px] font-medium text-white bg-gray-800 px-3 py-1.5 rounded-full hover:bg-gray-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                            >
                                {searchStatus === 'loading' ? 'Searching…' : 'Search'}
                            </button>
                            </form>
                            <div className="relative" ref={topKMenuRef}>
                            <button
                                type="button"
                                onClick={() => setShowTopKMenu(v => !v)}
                                className="h-9 w-9 rounded-full bg-white/90 border border-gray-200 shadow-sm text-gray-400 hover:text-gray-600 transition-colors flex items-center justify-center"
                                aria-label="Search options"
                                aria-expanded={showTopKMenu}
                            >
                                ⋯
                            </button>
                            <div
                                className={`absolute right-0 mt-2 w-44 bg-white border border-gray-200 rounded-md shadow-md z-50 transition-all duration-200 origin-top-right ${
                                    showTopKMenu
                                        ? 'opacity-100 scale-100 translate-y-0'
                                        : 'opacity-0 scale-95 -translate-y-1 pointer-events-none'
                                }`}
                            >
                                <div className="px-3 py-2 border-b border-gray-100">
                                    <p className="text-[10px] text-gray-400 mb-1">Top K</p>
                                    <input
                                        type="number"
                                        min={1}
                                        value={topK}
                                        onChange={e => setTopK(e.target.value)}
                                        className="w-full text-[11px] bg-gray-50 border border-gray-200 rounded px-2 py-1 text-gray-700 outline-none focus:border-gray-300"
                                    />
                                </div>
                                <div className="p-2 flex flex-wrap gap-1.5">
                                    {TOP_K_PRESETS.map(k => (
                                        <button
                                            key={k}
                                            type="button"
                                            onClick={() => {
                                                setTopK(String(k))
                                                setShowTopKMenu(false)
                                            }}
                                            className="text-[10px] text-gray-600 border border-gray-200 rounded px-2 py-1 hover:bg-gray-50"
                                        >
                                            {k}
                                        </button>
                                    ))}
                                </div>
                            </div>
                            </div>
                            </div>
                            {queryExpansionAvailable && (
                                <div className="flex items-center gap-2 text-[11px] text-gray-500">
                                    <label className="flex items-center gap-1.5 cursor-pointer select-none">
                                        <input
                                            type="checkbox"
                                            checked={queryExpansionEnabled}
                                            onChange={e => setQueryExpansionEnabled(e.target.checked)}
                                            className="accent-teal-600"
                                        />
                                        <span>Query Expansion</span>
                                    </label>
                                    <button
                                        type="button"
                                        title="Rewrites your query into a richer description before search to improve recall."
                                        aria-label="Query expansion help"
                                        className="h-4 w-4 rounded-full border border-gray-200 text-[9px] leading-none text-gray-400 hover:text-gray-600 hover:border-gray-300 transition-colors"
                                    >
                                        i
                                    </button>
                                </div>
                            )}
                            {searchStatus === 'success' && expandedQuery && (
                                <p className="text-[11px] text-gray-400 text-center max-w-[70vw] truncate">
                                    Searched for: <span className="text-gray-600">"{expandedQuery}"</span>
                                </p>
                            )}
                        </div>
                        {searchError && (
                            <p className="mt-2 text-[11px] text-rose-500 text-center">{searchError}</p>
                        )}
                    </div>
                    <div className="absolute top-4 left-4 z-30 w-80 max-w-[85vw] flex flex-col gap-3">
                        <div className="bg-white/90 border border-gray-200 rounded-lg shadow-sm flex flex-col overflow-hidden max-h-[55vh] min-h-[220px]">
                            <div className="px-4 py-2.5 border-b border-gray-100">
                                <p className="text-[11px] text-gray-400">Search results</p>
                                <p className="text-xs text-gray-600">
                                    Top {resolveTopK(topK)} results
                                </p>
                            </div>
                            <div className="flex-1 overflow-y-auto min-h-0">
                                {searchStatus === 'idle' && (
                                    <div className="flex flex-col items-center justify-center py-10 gap-2 text-center px-6">
                                        <div className="w-9 h-9 rounded-full border border-dashed border-gray-300 flex items-center justify-center text-gray-300 text-xl">
                                            ⌕
                                        </div>
                                        <p className="text-xs text-gray-400">
                                            Enter a query to search across all indexed areas
                                        </p>
                                    </div>
                                )}

                                {searchStatus === 'loading' && (
                                    <div className="flex items-center justify-center py-10">
                                        <div className="flex gap-1.5">
                                            <span className="w-1.5 h-1.5 rounded-full bg-gray-300 animate-bounce [animation-delay:-0.3s]" />
                                            <span className="w-1.5 h-1.5 rounded-full bg-gray-300 animate-bounce [animation-delay:-0.15s]" />
                                            <span className="w-1.5 h-1.5 rounded-full bg-gray-300 animate-bounce" />
                                        </div>
                                    </div>
                                )}

                                {searchStatus === 'error' && (
                                    <div className="mx-4 mt-4 px-3 py-2 bg-rose-50 border border-rose-100 rounded-md">
                                        <p className="text-xs text-rose-500">{searchError}</p>
                                    </div>
                                )}

                                {searchStatus === 'success' && searchResults.length === 0 && (
                                    <div className="flex flex-col items-center justify-center py-10 gap-1 text-center px-6">
                                        <p className="text-sm text-gray-500">No results found</p>
                                        <p className="text-xs text-gray-400">Try a different query</p>
                                    </div>
                                )}

                                {searchStatus === 'success' && searchResults.length > 0 && (
                                    <ul className="p-3 space-y-2">
                                        {searchResults.map((result, index) => (
                                            <SearchResultRow
                                                key={result.id}
                                                result={result}
                                                selected={selectedResultId === result.id}
                                                onSelect={() => {
                                                    setSelectedResultId(result.id)
                                                    const feature = searchFeatures[index]
                                                    if (feature?.geometry) {
                                                        setFocusedFeature(feature)
                                                    }
                                                }}
                                            />
                                        ))}
                                    </ul>
                                )}
                            </div>
                        </div>

                        <IndexedAreasPanel
                            areas={indexedAreas}
                            onToggle={handleToggleArea}
                            onDelete={id => {
                                setIndexedAreas(prev => prev.filter(area => area.id !== id))
                            }}
                            className="w-full max-h-[30vh]"
                            overlayEnabled={showIndexedOverlay}
                            onOverlayToggle={() => setShowIndexedOverlay(v => !v)}
                            satelliteEnabled={showSatellite}
                            onSatelliteToggle={() => setShowSatellite(v => !v)}
                            status={indexedAreasStatus}
                            error={indexedAreasError}
                            selectedId={selectedIndexedAreaId}
                            onSelect={id => {
                                setSelectedIndexedAreaId(id)
                                setFocusedFeature(null)
                            }}
                        />
                    </div>

                    <SelectedResultPanel result={selectedResult} />
                    <IndexedAreaPopup
                        area={selectedIndexedArea}
                        onClose={() => setSelectedIndexedAreaId(null)}
                    />
                </div>
            ) : (
                <div className="flex h-full w-full overflow-hidden">
                    <div className="flex flex-col w-1/2 border-r border-gray-200 bg-white">
                        <div className="px-5 py-4 border-b border-gray-200 flex items-center justify-between">
                            <div>
                                <h1 className="text-sm font-medium text-gray-700">Indexing Dashboard</h1>
                                <p className="text-xs text-gray-400">Select a file and draw a bbox to index</p>
                            </div>
                            <button
                                onClick={() => setPage('main')}
                                className="text-[11px] text-gray-500 border border-gray-200 rounded-md px-2.5 py-1 hover:border-gray-300 hover:text-gray-700 transition-colors"
                            >
                                Back to map
                            </button>
                        </div>

                        <div className="flex-1 min-h-0">
                            <S3Explorer
                                variant="panel"
                                selectedFile={selectedFile}
                                onFileSelect={file => {
                                    setSelectedFile(file)
                                    setSelectionBBox(null)
                                    setIndexMsg('')
                                    setIndexStatus('idle')
                                }}
                                showIndexActions={false}
                            />
                        </div>

                        <div className="border-t border-gray-200 px-5 py-4 space-y-3">
                            <div>
                                <label className="block text-xs text-gray-400 mb-1">Selected file</label>
                                {selectedFile ? (
                                    <div className="text-xs text-gray-600">
                                        <p className="font-medium text-gray-700">
                                            {selectedFile.type === 'folder' ? `Folder: ${selectedFile.name}` : selectedFile.name}
                                        </p>
                                        <p className="text-[10px] text-gray-400 font-mono break-all">{selectedFile.path}</p>
                                        {selectedFile.type === 'folder' && (
                                            <p className="text-[10px] text-gray-400 mt-1">
                                                {selectedFile.files?.length ?? 0} files selected
                                            </p>
                                        )}
                                    </div>
                                ) : (
                                    <p className="text-xs text-gray-400">Choose a file from the list above.</p>
                                )}
                            </div>

                            <div>
                                <label className="block text-xs text-gray-400 mb-1">Rows to index</label>
                                <input
                                    type="number"
                                    min={1}
                                    value={rowCount}
                                    onChange={e => setRowCount(e.target.value)}
                                    placeholder="Leave empty for all rows"
                                    className="w-full text-sm border border-gray-200 rounded-md px-3 py-2 text-gray-700 outline-none focus:border-gray-400 transition-colors"
                                />
                            </div>

                            <div>
                                <label className="block text-xs text-gray-400 mb-1">Bounding box</label>
                                {selectionBBox ? (
                                    <div
                                        className="font-mono text-[11px] text-gray-500 bg-gray-50 border border-gray-200 rounded-md px-3 py-2 leading-relaxed">
                                        <span className="text-gray-400">W</span> {selectionBBox.minX.toFixed(4)}&ensp;
                                        <span className="text-gray-400">E</span> {selectionBBox.maxX.toFixed(4)}&ensp;
                                        <span className="text-gray-400">S</span> {selectionBBox.minY.toFixed(4)}&ensp;
                                        <span className="text-gray-400">N</span> {selectionBBox.maxY.toFixed(4)}
                                    </div>
                                ) : (
                                    <p className="text-[11px] text-gray-400">Hold Shift + drag on the map to draw a
                                        bbox.</p>
                                )}
                            </div>

                            {indexMsg && (
                                <p className={`text-[11px] ${indexStatus === 'error' ? 'text-rose-500' : 'text-emerald-600'}`}>
                                    {indexMsg}
                                </p>
                            )}

                            <button
                                onClick={handleIndexSelection}
                                disabled={!canIndex}
                                className="w-full py-2 text-sm bg-gray-800 text-white rounded-md hover:bg-gray-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                            >
                                Index selection
                            </button>
                        </div>
                    </div>

                    <div className="relative w-1/2 bg-[#f8f8f7]">
                        <MapView
                            selectionBBox={selectionBBox}
                            onSelectionChange={selectedFile ? setSelectionBBox : undefined}
                            drawMode={selectedFile ? 'shift' : 'none'}
                        />
                        <div
                            className="absolute top-4 right-4 text-[11px] text-gray-500 bg-white/90 border border-gray-200 rounded-md px-2.5 py-1.5 shadow-sm">
                            {selectedFile ? (
                                <>
                                    Hold <span className="font-medium text-gray-600">Shift</span> + drag to draw a
                                    bounding box
                                </>
                            ) : (
                                'Select a file to start drawing a bbox'
                            )}
                        </div>
                    </div>
                </div>
            )}
        </div>
    )
}

function SearchResultRow({
    result,
    selected,
    onSelect,
}: {
    result: SearchResult
    selected: boolean
    onSelect: () => void
}) {
    const confidence = formatConfidence(result.confidence)
    return (
        <li>
            <button
                type="button"
                onClick={onSelect}
                className={`w-full text-left border rounded-md px-3 py-2.5 transition-colors ${
                    selected ? 'border-teal-400 bg-teal-50/70' : 'border-gray-200 bg-white hover:border-gray-300'
                }`}
            >
                <div className="flex items-start justify-between gap-2">
                    <span className="text-sm text-gray-800 leading-snug truncate">
                        {result.embed_text || result.id}
                    </span>
                    {confidence && (
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-gray-100 text-gray-500 font-medium tabular-nums">
                            {confidence}
                        </span>
                    )}
                </div>
                {(result.category || result.country) && (
                    <div className="mt-1 flex flex-wrap gap-2 text-[10px] text-gray-400">
                        {result.category && <span>{result.category}</span>}
                        {result.country && <span>{result.country}</span>}
                    </div>
                )}
                <p className="mt-1 text-[10px] text-gray-300 font-mono truncate">{result.id}</p>
            </button>
        </li>
    )
}

function formatDetailValue(value: unknown): string {
    if (value === null || value === undefined) return '—'
    if (typeof value === 'string') return value
    if (typeof value === 'number' || typeof value === 'boolean') return String(value)
    if (typeof value === 'object') {
        try {
            return JSON.stringify(value, null, 2)
        } catch {
            return String(value)
        }
    }
    return String(value)
}

function SelectedResultPanel({result}: { result: SearchResult | null }) {
    if (!result) return null
    const entries: Array<[string, unknown]> = [
        ['id', result.id],
        ['category', result.category],
        ['country', result.country],
        ['confidence', formatConfidence(result.confidence)],
        ['geometry', result.geom],
    ]
    if (result.raw && typeof result.raw === 'object') {
        for (const [key, value] of Object.entries(result.raw as Record<string, unknown>)) {
            entries.push([key, value])
        }
    }
    return (
        <div className="absolute top-20 right-4 z-30 w-80 max-w-[85vw] bg-white/95 border border-gray-200 rounded-lg shadow-sm overflow-hidden">
            <div className="px-4 py-3 border-b border-gray-100">
                <p className="text-[11px] text-gray-400">Selected result</p>
                <p className="text-sm text-gray-700 mt-1 leading-snug">{result.embed_text || result.id}</p>
            </div>
            <div className="px-4 py-3 space-y-2 max-h-[60vh] overflow-y-auto">
                {entries.map(([key, value]) => (
                    <div key={key} className="text-[11px] text-gray-500">
                        <p className="uppercase tracking-wide text-[9px] text-gray-400">{key}</p>
                        <pre className="whitespace-pre-wrap break-words text-gray-700">{formatDetailValue(value)}</pre>
                    </div>
                ))}
            </div>
        </div>
    )
}

function IndexedAreaPopup({
    area,
    onClose,
}: {
    area: IndexedArea | null
    onClose: () => void
}) {
    if (!area) return null
    const percent = resolveIndexedPercent(area)
    const percentLabel = percent == null ? '—' : `${percent}%`
    const total = typeof area.totalPoints === 'number' ? area.totalPoints : null
    const indexed = typeof area.indexedPoints === 'number' ? area.indexedPoints : null
    return (
        <div className="absolute bottom-6 right-4 z-30 w-80 max-w-[85vw] bg-white/95 border border-gray-200 rounded-lg shadow-lg overflow-hidden">
            <div className="px-4 py-3 border-b border-gray-100 flex items-start justify-between gap-2">
                <div className="min-w-0">
                    <p className="text-[11px] text-gray-400">Indexed area</p>
                    <p className="text-sm text-gray-700 mt-1 leading-snug truncate">{area.label}</p>
                    {area.source && (
                        <p className="text-[10px] text-gray-400 truncate">{area.source}</p>
                    )}
                </div>
                <button
                    type="button"
                    onClick={onClose}
                    className="text-gray-300 hover:text-gray-500 transition-colors"
                    aria-label="Close indexed area"
                >
                    ✕
                </button>
            </div>
            <div className="px-4 py-3 space-y-3">
                <div className="flex items-center justify-between text-[11px] text-gray-500">
                    <span>Indexed coverage</span>
                    <span className="font-medium text-gray-700">{percentLabel}</span>
                </div>
                <div className="h-2 rounded-full bg-gray-100 overflow-hidden">
                    <div
                        className="h-full bg-teal-500 transition-all"
                        style={{width: `${percent ?? 0}%`}}
                    />
                </div>
                <div className="grid grid-cols-2 gap-2 text-[11px]">
                    <div className="bg-gray-50 rounded-md px-2.5 py-2">
                        <p className="text-[9px] uppercase tracking-wide text-gray-400">Indexed points</p>
                        <p className="text-sm text-gray-700">
                            {indexed != null ? indexed.toLocaleString() : '—'}
                        </p>
                    </div>
                    <div className="bg-gray-50 rounded-md px-2.5 py-2">
                        <p className="text-[9px] uppercase tracking-wide text-gray-400">Total points</p>
                        <p className="text-sm text-gray-700">
                            {total != null ? total.toLocaleString() : '—'}
                        </p>
                    </div>
                </div>
                <div className="text-[11px] text-gray-500">
                    <p className="text-[9px] uppercase tracking-wide text-gray-400">Bounding box</p>
                    <p className="font-mono text-[10px] text-gray-600 mt-1">
                        W {area.bbox.minX.toFixed(4)} · E {area.bbox.maxX.toFixed(4)}
                    </p>
                    <p className="font-mono text-[10px] text-gray-600">
                        S {area.bbox.minY.toFixed(4)} · N {area.bbox.maxY.toFixed(4)}
                    </p>
                </div>
            </div>
        </div>
    )
}

function IndexedAreasPanel({
                                 areas,
                                 onToggle,
                                 onDelete,
                                 className,
                                 overlayEnabled,
                                 onOverlayToggle,
                                 satelliteEnabled,
                                 onSatelliteToggle,
                                 status,
                                 error,
                                 selectedId,
                                 onSelect,
                             }: {
    areas: IndexedArea[]
    onToggle: (id: string) => void
    onDelete: (id: string) => void
    className?: string
    overlayEnabled: boolean
    onOverlayToggle: () => void
    satelliteEnabled: boolean
    onSatelliteToggle: () => void
    status: Status
    error: string
    selectedId: string | null
    onSelect: (id: string) => void
}) {
    const [open, setOpen] = useState(true)
    const panelClass = `bg-white/90 border border-gray-200 rounded-lg shadow-sm overflow-hidden ${className ?? 'w-64'}`

    return (
        <div className={panelClass}>
            <div className="w-full px-3 py-2 flex items-center justify-between gap-2">
                <button
                    onClick={() => setOpen(v => !v)}
                    className="flex items-center gap-2 text-[11px] text-gray-600 hover:text-gray-800 transition-colors"
                >
                    <span>Indexed areas ({areas.length})</span>
                    <span className="text-gray-300">{open ? '–' : '+'}</span>
                </button>
                <div className="flex items-center gap-3">
                    <label className="flex items-center gap-1.5 text-[10px] text-gray-400">
                        <span>Indexed view</span>
                        <input
                            type="checkbox"
                            checked={overlayEnabled}
                            onChange={onOverlayToggle}
                            className="accent-teal-600"
                        />
                    </label>
                    <label className="flex items-center gap-1.5 text-[10px] text-gray-400">
                        <span>Satellite</span>
                        <input
                            type="checkbox"
                            checked={satelliteEnabled}
                            onChange={onSatelliteToggle}
                            className="accent-teal-600"
                        />
                    </label>
                </div>
            </div>
            {open && (
                <div className="border-t border-gray-100 max-h-56 overflow-y-auto">
                    {status === 'loading' && (
                        <p className="px-3 py-2 text-[11px] text-gray-400">Loading indexed areas…</p>
                    )}
                    {status === 'error' && error && (
                        <p className="px-3 py-2 text-[11px] text-rose-500">{error}</p>
                    )}
                    {status !== 'loading' && areas.length === 0 && (
                        <p className="px-3 py-3 text-[11px] text-gray-400">No indexed areas yet.</p>
                    )}
                    {areas.map(area => {
                        const active = area.active !== false
                        const totalPoints = area.totalPoints
                        const indexedPoints = area.indexedPoints
                        const indexedPercent = resolveIndexedPercent(area)
                        const hasCounts = typeof totalPoints === 'number'
                            && Number.isFinite(totalPoints)
                            && typeof indexedPoints === 'number'
                            && Number.isFinite(indexedPoints)
                        const isSelected = selectedId === area.id
                        const percentText = indexedPercent == null ? '—' : `${indexedPercent}%`
                        return (
                            <div
                                key={area.id}
                                className={`w-full px-3 py-2 text-[11px] border-l-2 transition-colors cursor-pointer ${
                                    isSelected ? 'border-teal-400 bg-teal-50/60' : 'border-transparent hover:bg-gray-50'
                                }`}
                                onClick={() => onSelect(area.id)}
                            >
                                <div className="flex items-center justify-between gap-2">
                                    <div className="min-w-0">
                                        <p className="truncate text-gray-600">
                                            {area.label}
                                        </p>
                                        <p className="text-[10px] text-gray-400 font-mono truncate">
                                            {area.bbox.minX.toFixed(2)},{area.bbox.minY.toFixed(2)} → {area.bbox.maxX.toFixed(2)},{area.bbox.maxY.toFixed(2)}
                                        </p>
                                        {(hasCounts || typeof indexedPercent === 'number') && (
                                            <p className="text-[10px] text-gray-400">
                                                {hasCounts
                                                    ? `Indexed ${indexedPoints!.toLocaleString()} / ${totalPoints!.toLocaleString()}`
                                                    : 'Indexed'}
                                                {` (${percentText})`}
                                            </p>
                                        )}
                                    </div>
                                </div>
                                <div className="mt-1.5 flex items-center justify-between">
                                    <label className="flex items-center gap-1 text-[10px] text-gray-400">
                                        <input
                                            type="checkbox"
                                            checked={active}
                                            onClick={e => e.stopPropagation()}
                                            onChange={() => onToggle(area.id)}
                                            className="accent-teal-600"
                                        />
                                        Show
                                    </label>
                                    <button
                                        type="button"
                                        onClick={e => {
                                            e.stopPropagation()
                                            onDelete(area.id)
                                        }}
                                        className="text-[10px] text-gray-400 hover:text-rose-500 transition-colors"
                                    >
                                        Delete
                                    </button>
                                </div>
                            </div>
                        )
                    })}
                </div>
            )}
        </div>
    )
}

export default App
