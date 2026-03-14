import {useState, useRef, useEffect, useMemo} from 'react'
import DeckGL from '@deck.gl/react'
import {TileLayer} from '@deck.gl/geo-layers'
import {BitmapLayer, PolygonLayer, ScatterplotLayer, PathLayer} from '@deck.gl/layers'
import {FlyToInterpolator, WebMercatorViewport} from '@deck.gl/core'
import type {MapViewState, PickingInfo} from '@deck.gl/core'
import type {GeoJSONGeometry, Geometry, SpatialFeature} from './DataPanel'

export interface BBox {
    minX: number
    minY: number
    maxX: number
    maxY: number
}

export interface IndexedArea {
    id: string
    label: string
    bbox: BBox
    color?: [number, number, number]
    active?: boolean
}

interface Rect {
    minX: number
    minY: number
    maxX: number
    maxY: number
}

interface Props {
    features?: SpatialFeature[]
    indexedAreas?: IndexedArea[]
    dimMap?: boolean
    selectionBBox?: BBox | null
    onSelectionChange?: (bbox: BBox | null) => void
    drawMode?: 'none' | 'shift' | 'always'
    focusedFeature?: SpatialFeature | null
    baseMap?: 'osm' | 'satellite'
}

const INITIAL_VIEW_STATE: MapViewState = {
    longitude: 0,
    latitude: 20,
    zoom: 2,
    pitch: 0,
    bearing: 0,
}

const WORLD_RING: [number, number][] = [
    [-180, -85],
    [180, -85],
    [180, 85],
    [-180, 85],
]

const WORLD_BOUNDS: [number, number, number, number] = [-180, -85, 180, 85]

function clamp(value: number, min: number, max: number) {
    return Math.min(max, Math.max(min, value))
}

function clampViewState(vs: MapViewState): MapViewState {
    const [minLng, minLat, maxLng, maxLat] = WORLD_BOUNDS
    return {
        ...vs,
        longitude: clamp(vs.longitude, minLng, maxLng),
        latitude: clamp(vs.latitude, minLat, maxLat),
    }
}

function createTileLayer(id: string, url: string) {
    return new TileLayer({
        id,
        data: url,
        maxZoom: 19,
        minZoom: 0,
        tileSize: 256,
        renderSubLayers: (props) => {
            const {boundingBox} = props.tile
            return new BitmapLayer(props, {
                data: undefined,
                image: props.data,
                bounds: [boundingBox[0][0], boundingBox[0][1], boundingBox[1][0], boundingBox[1][1]],
            })
        },
    })
}

const OSM_LAYER = createTileLayer('osm', 'https://tile.openstreetmap.org/{z}/{x}/{y}.png')
const SATELLITE_LAYER = createTileLayer(
    'satellite',
    'https://services.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}'
)

// ── WKT parsing ───────────────────────────────────────────────────────────────

/** Parse "x y" or "x y z" pairs from a coordinate string */
function parseCoords(s: string): [number, number][] {
    const pairs: [number, number][] = []
    const re = /([-\d.eE]+)\s+([-\d.eE]+)(?:\s+[-\d.eE]+)?/g
    let m
    while ((m = re.exec(s)) !== null) {
        const x = parseFloat(m[1])
        const y = parseFloat(m[2])
        if (!isNaN(x) && !isNaN(y) && isFinite(x) && isFinite(y)) pairs.push([x, y])
    }
    return pairs
}

/** Extract all innermost parenthesized coordinate strings from a WKT */
function innerRings(wkt: string): string[] {
    const groups: string[] = []
    const re = /\(([^()]+)\)/g
    let m
    while ((m = re.exec(wkt)) !== null) groups.push(m[1])
    return groups
}

function normalizeWkt(wkt: string): string {
    const trimmed = wkt.trim()
    return trimmed.replace(/^SRID=\d+;/i, '').trim()
}

function isGeoJSONGeometry(geometry: Geometry): geometry is GeoJSONGeometry {
    return typeof geometry === 'object' && geometry !== null
        && 'type' in geometry && typeof (geometry as GeoJSONGeometry).type === 'string'
        && 'coordinates' in geometry
}

function collectCoords(value: unknown, out: [number, number][]) {
    if (!Array.isArray(value)) return
    if (value.length >= 2 && typeof value[0] === 'number' && typeof value[1] === 'number') {
        out.push([value[0], value[1]])
        return
    }
    for (const item of value) collectCoords(item, out)
}

function extractCoords(geometry: Geometry): [number, number][] {
    if (typeof geometry === 'string') {
        return parseCoords(normalizeWkt(geometry))
    }
    if (isGeoJSONGeometry(geometry)) {
        const out: [number, number][] = []
        collectCoords(geometry.coordinates, out)
        return out
    }
    return []
}

/** Compute a bounding box that contains all feature geometries */
function featuresBbox(features: SpatialFeature[]): [[number, number], [number, number]] | null {
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
    let any = false
    for (const f of features) {
        const all = extractCoords(f.geometry)
        for (const [x, y] of all) {
            if (x < minX) minX = x;
            if (x > maxX) maxX = x
            if (y < minY) minY = y;
            if (y > maxY) maxY = y
            any = true
        }
    }
    return any ? [[minX, minY], [maxX, maxY]] : null
}

function featureBbox(feature: SpatialFeature): [[number, number], [number, number]] | null {
    const coords = extractCoords(feature.geometry)
    if (!coords.length) return null
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
    for (const [x, y] of coords) {
        if (x < minX) minX = x
        if (x > maxX) maxX = x
        if (y < minY) minY = y
        if (y > maxY) maxY = y
    }
    return [[minX, minY], [maxX, maxY]]
}

// ── Geometry categorization ───────────────────────────────────────────────────

interface PointDatum {
    pos: [number, number];
    feature: SpatialFeature
}

interface PolyDatum {
    rings: [number, number][][];
    feature: SpatialFeature
}

interface PathDatum {
    path: [number, number][];
    feature: SpatialFeature
}

function toCoordPair(value: unknown): [number, number] | null {
    if (!Array.isArray(value) || value.length < 2) return null
    const [x, y] = value
    if (typeof x === 'number' && typeof y === 'number' && isFinite(x) && isFinite(y)) {
        return [x, y]
    }
    return null
}

function toCoordList(value: unknown): [number, number][] {
    if (!Array.isArray(value)) return []
    const coords: [number, number][] = []
    for (const item of value) {
        const pair = toCoordPair(item)
        if (pair) coords.push(pair)
    }
    return coords
}

function addGeoJSONGeometry(
    geometry: GeoJSONGeometry,
    feature: SpatialFeature,
    points: PointDatum[],
    polys: PolyDatum[],
    paths: PathDatum[],
) {
    const type = geometry.type.toLowerCase()
    const coords = geometry.coordinates

    if (type === 'point') {
        const pair = toCoordPair(coords)
        if (pair) points.push({pos: pair, feature})
        return
    }

    if (type === 'multipoint') {
        for (const item of Array.isArray(coords) ? coords : []) {
            const pair = toCoordPair(item)
            if (pair) points.push({pos: pair, feature})
        }
        return
    }

    if (type === 'linestring') {
        const path = toCoordList(coords)
        if (path.length >= 2) paths.push({path, feature})
        return
    }

    if (type === 'multilinestring') {
        for (const line of Array.isArray(coords) ? coords : []) {
            const path = toCoordList(line)
            if (path.length >= 2) paths.push({path, feature})
        }
        return
    }

    if (type === 'polygon') {
        const rings = (Array.isArray(coords) ? coords : [])
            .map(ring => toCoordList(ring))
            .filter(ring => ring.length >= 3)
        if (rings.length) polys.push({rings, feature})
        return
    }

    if (type === 'multipolygon') {
        for (const poly of Array.isArray(coords) ? coords : []) {
            const rings = (Array.isArray(poly) ? poly : [])
                .map(ring => toCoordList(ring))
                .filter(ring => ring.length >= 3)
            if (rings.length) polys.push({rings, feature})
        }
    }
}

function categorize(features: SpatialFeature[]) {
    const points: PointDatum[] = []
    const polys: PolyDatum[] = []
    const paths: PathDatum[] = []

    for (const f of features) {
        const geom = f.geometry
        if (isGeoJSONGeometry(geom)) {
            addGeoJSONGeometry(geom, f, points, polys, paths)
            continue
        }
        const g = normalizeWkt(typeof geom === 'string' ? geom : '')
        const upper = g.toUpperCase()

        if (upper.startsWith('POINT')) {
            const c = parseCoords(g)
            if (c[0]) points.push({pos: c[0], feature: f})

        } else if (upper.startsWith('MULTIPOINT')) {
            for (const ring of innerRings(g)) {
                const c = parseCoords(ring)
                if (c[0]) points.push({pos: c[0], feature: f})
            }

        } else if (upper.startsWith('POLYGON') || upper.startsWith('MULTIPOLYGON')) {
            // Each innermost paren group is a ring; render each as a filled polygon.
            for (const ring of innerRings(g)) {
                const c = parseCoords(ring)
                if (c.length >= 3) polys.push({rings: [c], feature: f})
            }

        } else if (upper.startsWith('LINESTRING') || upper.startsWith('MULTILINESTRING')) {
            for (const ring of innerRings(g)) {
                const c = parseCoords(ring)
                if (c.length >= 2) paths.push({path: c, feature: f})
            }
        }
    }

    return {points, polys, paths}
}

// ── Tooltip helpers ───────────────────────────────────────────────────────────

function resolveLabel(feature: SpatialFeature): string {
    const p = feature.properties
    const names = p.names
    if (typeof names === 'object' && names !== null) {
        const n = names as Record<string, unknown>
        if (typeof n.primary === 'string') return n.primary
    }
    if (typeof names === 'string' && names.startsWith('{')) {
        try {
            const n = JSON.parse(names);
            if (n?.primary) return String(n.primary)
        } catch { /**/
        }
    }
    if (typeof p.name === 'string' && p.name) return p.name
    if (typeof p.id === 'string') return p.id.slice(0, 20)
    return 'Feature'
}

function resolveSubtitle(feature: SpatialFeature): string {
    const p = feature.properties
    for (const key of ['class', 'subtype', 'confidence', 'height', 'level']) {
        if (key in p && p[key] != null) return `${key}: ${p[key]}`
    }
    const cats = p.categories
    if (typeof cats === 'object' && cats !== null) {
        const c = cats as Record<string, unknown>
        if (typeof c.primary === 'string') return c.primary
    }
    return ''
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function MapView({
                                     features,
                                     indexedAreas,
                                     dimMap = false,
                                     selectionBBox,
                                     onSelectionChange,
                                     drawMode = 'none',
                                     focusedFeature = null,
                                     baseMap = 'osm',
                                 }: Props) {
    const containerRef = useRef<HTMLDivElement>(null)
    const [viewState, setViewState] = useState<MapViewState>(INITIAL_VIEW_STATE)
    const vsRef = useRef<MapViewState>(INITIAL_VIEW_STATE)
    const [size, setSize] = useState({width: 0, height: 0})
    const [isDrawing, setIsDrawing] = useState(false)
    const [dragStart, setDragStart] = useState<[number, number] | null>(null)
    const [dragCurrent, setDragCurrent] = useState<[number, number] | null>(null)

    const safeFeatures = useMemo(() => features ?? [], [features])

    useEffect(() => {
        const el = containerRef.current
        if (!el) return
        const update = () => {
            setSize({width: el.clientWidth, height: el.clientHeight})
        }
        update()
        const ro = new ResizeObserver(update)
        ro.observe(el)
        return () => ro.disconnect()
    }, [])

    // Fly to fit features whenever the feature list changes
    useEffect(() => {
        if (!safeFeatures.length || !containerRef.current) return
        const bbox = featuresBbox(safeFeatures)
        if (!bbox) return
        const {clientWidth: w, clientHeight: h} = containerRef.current
        if (w === 0 || h === 0) return

        try {
            const vp = new WebMercatorViewport({width: w, height: h})
            const {longitude, latitude, zoom} = vp.fitBounds(bbox, {padding: 60})
            const next = {...vsRef.current, longitude, latitude, zoom: Math.min(zoom, 18)}
            vsRef.current = next
            setViewState(next)
        } catch { /* fitBounds can throw for degenerate bbox */
        }
    }, [safeFeatures])

    // Fly to a focused feature when selected
    useEffect(() => {
        if (!focusedFeature || !containerRef.current) return
        const bbox = featureBbox(focusedFeature)
        if (!bbox) return
        const {clientWidth: w, clientHeight: h} = containerRef.current
        if (w === 0 || h === 0) return

        try {
            const vp = new WebMercatorViewport({width: w, height: h})
            const [min, max] = bbox
            const isPoint = min[0] === max[0] && min[1] === max[1]
            let longitude = min[0]
            let latitude = min[1]
            let zoom = Math.max(vsRef.current.zoom, 15)

            if (!isPoint) {
                const fitted = vp.fitBounds(bbox, {padding: 80})
                longitude = fitted.longitude
                latitude = fitted.latitude
                zoom = Math.min(fitted.zoom, 18)
            }

            const next = {
                ...vsRef.current,
                longitude,
                latitude,
                zoom,
                transitionDuration: 1200,
                transitionInterpolator: new FlyToInterpolator({speed: 1.6}),
            }
            vsRef.current = next
            setViewState(next)
        } catch { /* ignore fit errors */ }
    }, [focusedFeature, size.width, size.height])

    const {points, polys, paths} = categorize(safeFeatures)

    const pointsLayer = points.length
        ? new ScatterplotLayer<PointDatum>({
            id: 'points',
            data: points,
            getPosition: d => d.pos,
            getRadius: 6,
            radiusUnits: 'pixels',
            getFillColor: [59, 130, 246, 210],
            getLineColor: [255, 255, 255, 180],
            lineWidthMinPixels: 1.5,
            stroked: true,
            pickable: true,
        })
        : null

    const polysLayer = polys.length
        ? new PolygonLayer<PolyDatum>({
            id: 'polys',
            data: polys,
            getPolygon: d => d.rings,
            getFillColor: [59, 130, 246, 40],
            getLineColor: [59, 130, 246, 200],
            lineWidthMinPixels: 1.5,
            filled: true,
            stroked: true,
            pickable: true,
        })
        : null

    const pathsLayer = paths.length
        ? new PathLayer<PathDatum>({
            id: 'paths',
            data: paths,
            getPath: d => d.path,
            getWidth: 2.5,
            widthUnits: 'pixels',
            getColor: [234, 88, 12, 210],
            pickable: true,
        })
        : null

    function bboxToPolygon(bbox: BBox): [number, number][][] {
        return [[
            [bbox.minX, bbox.minY],
            [bbox.maxX, bbox.minY],
            [bbox.maxX, bbox.maxY],
            [bbox.minX, bbox.maxY],
        ]]
    }

    function normalizeRect(bbox: BBox): Rect {
        const minX = Math.min(bbox.minX, bbox.maxX)
        const maxX = Math.max(bbox.minX, bbox.maxX)
        const minY = Math.min(bbox.minY, bbox.maxY)
        const maxY = Math.max(bbox.minY, bbox.maxY)
        return {minX, maxX, minY, maxY}
    }

    function unionRectangles(rects: Rect[]): Rect[] {
        if (rects.length === 0) return []
        const xs = Array.from(new Set(rects.flatMap(r => [r.minX, r.maxX]))).sort((a, b) => a - b)
        const results: Rect[] = []

        for (let i = 0; i < xs.length - 1; i += 1) {
            const x1 = xs[i]
            const x2 = xs[i + 1]
            if (x2 <= x1) continue
            const active = rects.filter(r => r.minX <= x1 && r.maxX >= x2)
            if (active.length === 0) continue

            const ys = active
                .map(r => [r.minY, r.maxY] as [number, number])
                .sort((a, b) => a[0] - b[0])

            let [curStart, curEnd] = ys[0]
            for (let j = 1; j < ys.length; j += 1) {
                const [start, end] = ys[j]
                if (start <= curEnd) {
                    curEnd = Math.max(curEnd, end)
                } else {
                    results.push({minX: x1, maxX: x2, minY: curStart, maxY: curEnd})
                    curStart = start
                    curEnd = end
                }
            }
            results.push({minX: x1, maxX: x2, minY: curStart, maxY: curEnd})
        }

        return results
    }

    const indexedRects = useMemo(() => {
        const rects = (indexedAreas ?? [])
            .filter(a => a.active !== false)
            .map(a => normalizeRect(a.bbox))
            .filter(r => r.maxX > r.minX && r.maxY > r.minY)
        return unionRectangles(rects)
    }, [indexedAreas])

    const dimPolygon = useMemo(() => {
        if (!dimMap) return null
        const holes = indexedRects.map(r => bboxToPolygon(r)[0])
        return [WORLD_RING, ...holes]
    }, [dimMap, indexedRects])

    const dimLayer = dimMap && dimPolygon
        ? new PolygonLayer({
            id: 'dim-overlay',
            data: [dimPolygon],
            getPolygon: d => d,
            getFillColor: [2, 6, 23, 95],
            stroked: false,
            filled: true,
            pickable: false,
        })
        : null

    const viewport = useMemo(() => {
        if (size.width === 0 || size.height === 0) return null
        return new WebMercatorViewport({width: size.width, height: size.height, ...viewState})
    }, [size, viewState])

    const previewBBox = useMemo(() => {
        if (!viewport || !dragStart || !dragCurrent) return null
        const start = viewport.unproject(dragStart) as [number, number]
        const end = viewport.unproject(dragCurrent) as [number, number]
        const minX = Math.min(start[0], end[0])
        const maxX = Math.max(start[0], end[0])
        const minY = Math.min(start[1], end[1])
        const maxY = Math.max(start[1], end[1])
        return {minX, minY, maxX, maxY}
    }, [viewport, dragStart, dragCurrent])

    const selectionLayer = selectionBBox || previewBBox
        ? new PolygonLayer({
            id: 'selection-bbox',
            data: [selectionBBox ?? previewBBox!],
            getPolygon: d => bboxToPolygon(d),
            getFillColor: selectionBBox ? [59, 130, 246, 60] : [59, 130, 246, 30],
            getLineColor: selectionBBox ? [59, 130, 246, 200] : [59, 130, 246, 140],
            lineWidthMinPixels: 2,
            filled: true,
            stroked: true,
            pickable: false,
        })
        : null

    const layers = [
        baseMap === 'satellite' ? SATELLITE_LAYER : OSM_LAYER,
        dimLayer,
        selectionLayer,
        polysLayer,
        pathsLayer,
        pointsLayer,
    ].filter(Boolean)

    const getTooltip = (info: PickingInfo) => {
        const object = info.object
        if (!object) return null
        const d = object as { feature?: SpatialFeature }
        const feature = d.feature
        if (!feature) return null
        const label = resolveLabel(feature)
        const sub = resolveSubtitle(feature)
        return {
            text: sub ? `${label}\n${sub}` : label,
            style: {
                fontSize: '11px',
                padding: '5px 8px',
                background: 'white',
                color: '#374151',
                borderRadius: '4px',
                boxShadow: '0 1px 4px rgba(0,0,0,0.12)',
                whiteSpace: 'pre',
            },
        }
    }

    return (
        <div ref={containerRef} className="relative w-full h-full select-none">
            <DeckGL
                viewState={viewState}
                onViewStateChange={({viewState: vs}) => {
                    const next = clampViewState(vs as MapViewState)
                    vsRef.current = next
                    setViewState(next)
                }}
                controller={{
                    dragPan: !isDrawing,
                    dragRotate: false,
                    touchRotate: false,
                    scrollZoom: true,
                    doubleClickZoom: true,
                }}
                layers={layers}
                style={{width: '100%', height: '100%'}}
                getTooltip={getTooltip}
                onDragStart={(info, event) => {
                    if (!onSelectionChange) return
                    if (!containerRef.current) return
                    const shouldDraw = drawMode === 'always' || (drawMode !== 'none' && event?.srcEvent?.shiftKey)
                    if (!shouldDraw) return
                    setIsDrawing(true)
                    setDragStart([info.x, info.y])
                    setDragCurrent([info.x, info.y])
                }}
                onDrag={info => {
                    if (!isDrawing) return
                    setDragCurrent([info.x, info.y])
                }}
                onDragEnd={() => {
                    if (!isDrawing || !dragStart || !dragCurrent || !viewport) {
                        setIsDrawing(false)
                        setDragStart(null)
                        setDragCurrent(null)
                        return
                    }
                    const start = viewport.unproject(dragStart) as [number, number]
                    const end = viewport.unproject(dragCurrent) as [number, number]
                    const minX = Math.min(start[0], end[0])
                    const maxX = Math.max(start[0], end[0])
                    const minY = Math.min(start[1], end[1])
                    const maxY = Math.max(start[1], end[1])
                    const minDrag = 3
                    const dx = Math.abs(dragCurrent[0] - dragStart[0])
                    const dy = Math.abs(dragCurrent[1] - dragStart[1])
                    if (dx > minDrag && dy > minDrag) {
                        onSelectionChange?.({minX, minY, maxX, maxY})
                    }
                    setIsDrawing(false)
                    setDragStart(null)
                    setDragCurrent(null)
                }}
            />

            {/* Feature count overlay */}
            {safeFeatures.length > 0 && (
                <div className="absolute top-2 left-1/2 -translate-x-1/2 pointer-events-none">
          <span className="text-[10px] text-gray-600 bg-white/90 px-2.5 py-1 rounded shadow-sm whitespace-nowrap">
            {safeFeatures.length} feature{safeFeatures.length !== 1 ? 's' : ''} loaded
          </span>
                </div>
            )}

            {/* Map attribution */}
            <div
                className="absolute bottom-2 right-2 text-[10px] text-gray-400 bg-white/80 px-1.5 py-0.5 rounded pointer-events-none">
                {baseMap === 'satellite' ? (
                    <>
                        Imagery ©{' '}
                        <a
                            href="https://www.esri.com/en-us/arcgis/products/arcgis-world-imagery/overview"
                            className="underline pointer-events-auto"
                            target="_blank"
                            rel="noreferrer"
                        >
                            Esri
                        </a>
                    </>
                ) : (
                    <>
                        ©{' '}
                        <a
                            href="https://www.openstreetmap.org/copyright"
                            className="underline pointer-events-auto"
                            target="_blank"
                            rel="noreferrer"
                        >
                            OpenStreetMap
                        </a>{' '}
                        contributors
                    </>
                )}
            </div>
        </div>
    )
}
