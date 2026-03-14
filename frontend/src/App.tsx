import {useState, type FormEvent} from 'react'
import MapView, {type BBox, type IndexedArea} from './components/MapView'
import S3Explorer, {type FileItem, type SelectedFile} from './components/S3Explorer'
import type {SpatialFeature} from './components/DataPanel'
import './App.css'

const API = 'http://localhost:3001/api/v1'

const HARD_CODED_AREAS: IndexedArea[] = [
    {
        id: 'demo-berlin',
        label: 'Berlin Mitte',
        bbox: {minX: 13.35, minY: 52.49, maxX: 13.45, maxY: 52.55},
        color: [16, 185, 129],
        active: true,
    },
]

type Page = 'main' | 'index'
type Status = 'idle' | 'loading' | 'success' | 'error'

interface SearchResult {
    id: string
    name: string
    confidence: number
    socials?: Record<string, string> | null
    geometry: string

    [key: string]: unknown
}

function App() {
    const [page, setPage] = useState<Page>('main')
    const [indexedAreas, setIndexedAreas] = useState<IndexedArea[]>(HARD_CODED_AREAS)
    const [selectedAreaId, setSelectedAreaId] = useState<string | null>(HARD_CODED_AREAS[0]?.id ?? null)
    const [searchQuery, setSearchQuery] = useState('')
    const [searchStatus, setSearchStatus] = useState<Status>('idle')
    const [searchError, setSearchError] = useState('')
    const [searchFeatures, setSearchFeatures] = useState<SpatialFeature[]>([])

    const [selectedFile, setSelectedFile] = useState<SelectedFile | null>(null)
    const [selectionBBox, setSelectionBBox] = useState<BBox | null>(null)
    const [rowCount, setRowCount] = useState('')
    const [indexStatus, setIndexStatus] = useState<Status>('idle')
    const [indexMsg, setIndexMsg] = useState('')

    const activeArea = indexedAreas.find(a => a.id === selectedAreaId && a.active !== false) ?? null

    async function handleSearchSubmit(e: FormEvent) {
        e.preventDefault()
        const query = searchQuery.trim()
        if (!query || !activeArea) {
            setSearchError('Select an indexed area and enter a search term.')
            setSearchStatus('error')
            return
        }
        setSearchStatus('loading')
        setSearchError('')
        try {
            const res = await fetch(`${API}/search-real`, {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({
                    category: query,
                    bbox_min_x: activeArea.bbox.minX,
                    bbox_max_x: activeArea.bbox.maxX,
                    bbox_min_y: activeArea.bbox.minY,
                    bbox_max_y: activeArea.bbox.maxY,
                    limit: 50,
                }),
            })
            if (!res.ok) throw new Error(`Server responded with ${res.status}`)
            const data = await res.json()
            const list: SearchResult[] = data.restaurants ?? []
            const mapped: SpatialFeature[] = list.map(row => {
                const {geometry, ...props} = row
                return {
                    geometry: typeof geometry === 'string' ? geometry : '',
                    properties: props,
                }
            })
            setSearchFeatures(mapped)
            setSearchStatus('success')
        } catch (err) {
            setSearchError(err instanceof Error ? err.message : 'Search failed')
            setSearchFeatures([])
            setSearchStatus('error')
        }
    }

    function handleSearchClear() {
        setSearchQuery('')
        setSearchError('')
        setSearchFeatures([])
        setSearchStatus('idle')
    }

    function handleToggleArea(id: string) {
        setIndexedAreas(prev => {
            const next = prev.map(area => area.id === id ? {...area, active: area.active === false} : area)
            if (selectedAreaId === id && next.find(a => a.id === id)?.active === false) {
                const fallback = next.find(a => a.active !== false)
                setSelectedAreaId(fallback?.id ?? null)
            }
            return next
        })
    }

    async function handleIndexSelection() {
        if (!selectedFile || !selectionBBox) return
        setIndexStatus('loading')
        setIndexMsg('')
        try {
            const count = parseInt(rowCount, 10)
            const files: FileItem[] = selectedFile.type === 'folder'
                ? (selectedFile.files ?? [])
                : [{path: selectedFile.path, name: selectedFile.name, region: selectedFile.region}]

            if (files.length === 0) {
                throw new Error('No files selected for indexing.')
            }

            let successCount = 0
            let lastError = ''

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
                try {
                    const res = await fetch(`${API}/index-file`, {
                        method: 'POST',
                        headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify(payload),
                    })
                    if (!res.ok) throw new Error(`Server responded with ${res.status}`)
                    successCount += 1
                } catch (err) {
                    lastError = err instanceof Error ? err.message : 'Failed to start indexing'
                }
            }

            if (successCount === 0) {
                throw new Error(lastError || 'Failed to start indexing')
            }

            const areaId = `${selectedFile.name}-${Date.now()}`
            const newArea: IndexedArea = {
                id: areaId,
                label: selectedFile.type === 'folder'
                    ? `${selectedFile.name} (${successCount} files)`
                    : selectedFile.name,
                bbox: selectionBBox,
                color: [59, 130, 246],
                active: true,
            }
            setIndexedAreas(prev => [...prev, newArea])
            setSelectedAreaId(areaId)
            if (successCount < files.length) {
                setIndexMsg(`Indexed ${successCount}/${files.length} files. Last error: ${lastError}`)
                setIndexStatus('error')
            } else {
                setIndexMsg(`Indexing started for ${successCount} file${successCount !== 1 ? 's' : ''}.`)
                setIndexStatus('success')
            }
        } catch (err) {
            setIndexMsg(err instanceof Error ? err.message : 'Failed to start indexing')
            setIndexStatus('error')
        }
    }

    const canIndex = !!selectedFile && !!selectionBBox && indexStatus !== 'loading'

    return (
        <div className="h-screen w-screen bg-[#f8f8f7]">
            {page === 'main' ? (
                <div className="relative h-full w-full overflow-hidden">
                    <MapView
                        features={searchFeatures}
                        indexedAreas={indexedAreas}
                        dimMap
                    />

                    <div className="absolute top-4 right-4 z-40">
                        <button
                            onClick={() => setPage('index')}
                            className="text-[11px] text-gray-500 bg-white/90 border border-gray-200 rounded-md px-3 py-1.5 shadow-sm hover:border-gray-300 hover:text-gray-700 transition-colors"
                        >
                            Open indexing dashboard
                        </button>
                    </div>

                    <div className="absolute top-4 left-4 z-40">
                        <IndexedAreasPanel
                            areas={indexedAreas}
                            selectedAreaId={selectedAreaId}
                            onSelect={id => setSelectedAreaId(id)}
                            onToggle={handleToggleArea}
                            onDelete={id => {
                                setIndexedAreas(prev => {
                                    const next = prev.filter(area => area.id !== id)
                                    if (selectedAreaId === id) {
                                        const fallback = next.find(area => area.active !== false)
                                        setSelectedAreaId(fallback?.id ?? null)
                                    }
                                    return next
                                })
                            }}
                        />
                    </div>

                    <div className="absolute top-4 left-1/2 -translate-x-1/2 z-40 w-[420px] max-w-[80vw]">
                        <form
                            onSubmit={handleSearchSubmit}
                            className="flex items-center gap-2 bg-white/90 border border-gray-200 rounded-full shadow-sm px-3 py-2"
                        >
                            <input
                                type="text"
                                value={searchQuery}
                                onChange={e => setSearchQuery(e.target.value)}
                                placeholder={activeArea ? `Search in ${activeArea.label}` : 'Select an indexed area to search'}
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
                                disabled={!searchQuery.trim() || !activeArea || searchStatus === 'loading'}
                                className="text-[11px] font-medium text-white bg-gray-800 px-3 py-1.5 rounded-full hover:bg-gray-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                            >
                                {searchStatus === 'loading' ? 'Searching…' : 'Search'}
                            </button>
                        </form>
                        {searchError && (
                            <p className="mt-2 text-[11px] text-rose-500 text-center">{searchError}</p>
                        )}
                    </div>
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
                                {indexStatus === 'loading'
                                    ? 'Indexing…'
                                    : `Index selection${selectedFile?.type === 'folder' ? ` (${selectedFile.files?.length ?? 0} files)` : ''}`}
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

function IndexedAreasPanel({
                               areas,
                               selectedAreaId,
                               onSelect,
                               onToggle,
                               onDelete,
                           }: {
    areas: IndexedArea[]
    selectedAreaId: string | null
    onSelect: (id: string) => void
    onToggle: (id: string) => void
    onDelete: (id: string) => void
}) {
    const [open, setOpen] = useState(true)

    return (
        <div className="w-64 bg-white/90 border border-gray-200 rounded-lg shadow-sm overflow-hidden">
            <button
                onClick={() => setOpen(v => !v)}
                className="w-full px-3 py-2 flex items-center justify-between text-[11px] text-gray-600 hover:text-gray-800 transition-colors"
            >
                <span>Indexed areas ({areas.length})</span>
                <span className="text-gray-300">{open ? '–' : '+'}</span>
            </button>
            {open && (
                <div className="border-t border-gray-100 max-h-56 overflow-y-auto">
                    {areas.length === 0 && (
                        <p className="px-3 py-3 text-[11px] text-gray-400">No indexed areas yet.</p>
                    )}
                    {areas.map(area => {
                        const active = area.active !== false
                        const selected = area.id === selectedAreaId
                        return (
                            <div
                                key={area.id}
                                className={`w-full px-3 py-2 text-[11px] border-l-2 transition-colors ${
                                    selected ? 'border-teal-500 bg-teal-50/60' : 'border-transparent hover:bg-gray-50'
                                }`}
                            >
                                <button
                                    type="button"
                                    onClick={() => onSelect(area.id)}
                                    className="w-full text-left"
                                >
                                    <div className="flex items-center justify-between gap-2">
                                        <div className="min-w-0">
                                            <p className={`truncate ${selected ? 'text-teal-700 font-medium' : 'text-gray-600'}`}>
                                                {area.label}
                                            </p>
                                            <p className="text-[10px] text-gray-400 font-mono truncate">
                                                {area.bbox.minX.toFixed(2)},{area.bbox.minY.toFixed(2)} → {area.bbox.maxX.toFixed(2)},{area.bbox.maxY.toFixed(2)}
                                            </p>
                                        </div>
                                    </div>
                                </button>
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
                                        onClick={() => onDelete(area.id)}
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
