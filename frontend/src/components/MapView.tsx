import { useState, useRef, useCallback, useEffect } from 'react'
import DeckGL from '@deck.gl/react'
import { TileLayer } from '@deck.gl/geo-layers'
import { BitmapLayer, PolygonLayer, ScatterplotLayer, TextLayer } from '@deck.gl/layers'
import { WebMercatorViewport } from '@deck.gl/core'
import type { MapViewState } from '@deck.gl/core'
import type { Restaurant, ChunkInfo } from './SearchView'

export interface BBox {
  minX: number
  minY: number
  maxX: number
  maxY: number
}

interface Props {
  bbox: BBox | null
  onBboxChange: (bbox: BBox | null) => void
  restaurants: Restaurant[]
  chunks: ChunkInfo[]
}

const INITIAL_VIEW_STATE: MapViewState = {
  longitude: 0,
  latitude: 20,
  zoom: 2,
  pitch: 0,
  bearing: 0,
}

const OSM_LAYER = new TileLayer({
  id: 'osm',
  data: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png',
  maxZoom: 19,
  minZoom: 0,
  tileSize: 256,
  renderSubLayers: (props) => {
    const { boundingBox } = props.tile
    return new BitmapLayer(props, {
      data: undefined,
      image: props.data,
      bounds: [boundingBox[0][0], boundingBox[0][1], boundingBox[1][0], boundingBox[1][1]],
    })
  },
})

function bboxRing(b: BBox) {
  return [[[b.minX, b.minY], [b.maxX, b.minY], [b.maxX, b.maxY], [b.minX, b.maxY], [b.minX, b.minY]]]
}

/** Parse WKT "POINT(lng lat)" → [lng, lat] */
function parseWktPoint(wkt: string): [number, number] | null {
  const m = wkt.match(/POINT\s*\(\s*([-\d.]+)\s+([-\d.]+)\s*\)/i)
  return m ? [parseFloat(m[1]), parseFloat(m[2])] : null
}

export default function MapView({ bbox, onBboxChange, restaurants, chunks }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [viewState, setViewState] = useState<MapViewState>(INITIAL_VIEW_STATE)
  // Keep a ref so event handlers always see the latest value without stale closures
  const vsRef = useRef<MapViewState>(INITIAL_VIEW_STATE)

  const dragStart = useRef<{ x: number; y: number } | null>(null)
  const [isDrawing, setIsDrawing] = useState(false)

  // Fly to bbox when restaurants arrive
  useEffect(() => {
    if (!restaurants.length || !bbox || !containerRef.current) return
    const { clientWidth: w, clientHeight: h } = containerRef.current
    const vp = new WebMercatorViewport({ width: w, height: h })
    const { longitude, latitude, zoom } = vp.fitBounds(
      [[bbox.minX, bbox.minY], [bbox.maxX, bbox.maxY]],
      { padding: 80 },
    )
    const next = { ...vsRef.current, longitude, latitude, zoom }
    vsRef.current = next
    setViewState(next)
  }, [restaurants]) // eslint-disable-line react-hooks/exhaustive-deps

  /** Convert a pixel coordinate to [lng, lat] using the current view state */
  const unproject = useCallback((px: number, py: number): [number, number] | null => {
    const el = containerRef.current
    if (!el) return null
    const vs = vsRef.current
    const vp = new WebMercatorViewport({
      width: el.clientWidth,
      height: el.clientHeight,
      longitude: vs.longitude,
      latitude: vs.latitude,
      zoom: vs.zoom,
      pitch: vs.pitch ?? 0,
      bearing: vs.bearing ?? 0,
    })
    const [lng, lat] = vp.unproject([px, py])
    return [lng, lat]
  }, [])

  function onMouseDown(e: React.MouseEvent<HTMLDivElement>) {
    if (!e.shiftKey) return
    e.preventDefault()
    dragStart.current = { x: e.nativeEvent.offsetX, y: e.nativeEvent.offsetY }
    setIsDrawing(true)
    onBboxChange(null)
  }

  function onMouseMove(e: React.MouseEvent<HTMLDivElement>) {
    if (!isDrawing || !dragStart.current) return
    e.preventDefault()
    const a = unproject(dragStart.current.x, dragStart.current.y)
    const b = unproject(e.nativeEvent.offsetX, e.nativeEvent.offsetY)
    if (!a || !b) return
    onBboxChange({
      minX: Math.min(a[0], b[0]),
      maxX: Math.max(a[0], b[0]),
      minY: Math.min(a[1], b[1]),
      maxY: Math.max(a[1], b[1]),
    })
  }

  function onMouseUp(e: React.MouseEvent<HTMLDivElement>) {
    if (!isDrawing) return
    e.preventDefault()
    dragStart.current = null
    setIsDrawing(false)
  }

  // ── Layers ──────────────────────────────────────────────────────────────────

  const selectionLayer = bbox
    ? new PolygonLayer({
        id: 'bbox',
        data: [{ polygon: bboxRing(bbox) }],
        getPolygon: (d: any) => d.polygon,
        getFillColor: [59, 130, 246, 30],
        getLineColor: [59, 130, 246, 210],
        lineWidthMinPixels: 1.5,
        filled: true,
        stroked: true,
        pickable: false,
      })
    : null

  const points = restaurants
    .map(r => ({ ...r, coords: parseWktPoint(r.geometry) }))
    .filter((r): r is typeof r & { coords: [number, number] } => r.coords !== null)

  const dotsLayer = points.length
    ? new ScatterplotLayer({
        id: 'dots',
        data: points,
        getPosition: d => d.coords,
        getRadius: 7,
        radiusUnits: 'pixels',
        getFillColor: d =>
          d.confidence >= 0.9  ? [16, 185, 129, 230] :
          d.confidence >= 0.75 ? [245, 158, 11, 230]  :
                                  [156, 163, 175, 230],
        getLineColor: [255, 255, 255, 200],
        lineWidthMinPixels: 1.5,
        stroked: true,
        pickable: true,
      })
    : null

  const labelsLayer = points.length
    ? new TextLayer({
        id: 'labels',
        data: points,
        getPosition: d => d.coords,
        getText: d => d.name,
        getSize: 11,
        getColor: [30, 30, 30, 220],
        background: true,
        getBackgroundColor: [255, 255, 255, 210],
        getBorderColor: [200, 200, 200, 180],
        getBorderWidth: 0.5,
        backgroundPadding: [3, 2, 3, 2],
        getPixelOffset: [0, -18],
        fontFamily: 'system-ui, sans-serif',
        pickable: false,
      })
    : null

  const chunksLayer = chunks.length
    ? new PolygonLayer({
        id: 'chunks',
        data: chunks.map(c => ({
          polygon: bboxRing({ minX: c.bbox_min_x, maxX: c.bbox_max_x, minY: c.bbox_min_y, maxY: c.bbox_max_y }),
          label: c.category,
        })),
        getPolygon: (d: any) => d.polygon,
        getFillColor: [16, 185, 129, 18],
        getLineColor: [16, 185, 129, 160],
        lineWidthMinPixels: 1,
        getDashArray: [4, 3],
        dashJustified: true,
        extensions: [],
        filled: true,
        stroked: true,
        pickable: false,
      })
    : null

  const layers = [
    OSM_LAYER,
    chunksLayer,
    selectionLayer,
    dotsLayer,
    labelsLayer,
  ].filter(Boolean)

  return (
    <div
      ref={containerRef}
      className="relative w-full h-full select-none"
      style={{ cursor: isDrawing ? 'crosshair' : 'default' }}
      onMouseDown={onMouseDown}
      onMouseMove={onMouseMove}
      onMouseUp={onMouseUp}
      onMouseLeave={onMouseUp}
    >
      <DeckGL
        viewState={viewState}
        onViewStateChange={({ viewState: vs }) => {
          const next = vs as MapViewState
          vsRef.current = next
          setViewState(next)
        }}
        controller={!isDrawing}
        layers={layers}
        style={{ width: '100%', height: '100%' }}
        getTooltip={({ object }) => {
          if (!object) return null
          const r = object as Restaurant
          return {
            text: `${r.name}\nConfidence: ${Math.round(r.confidence * 100)}%`,
            style: {
              fontSize: '11px',
              padding: '5px 8px',
              background: 'white',
              color: '#374151',
              borderRadius: '4px',
              boxShadow: '0 1px 4px rgba(0,0,0,0.12)',
            },
          }
        }}
      />

      {/* Hint */}
      <div className="absolute top-2 left-1/2 -translate-x-1/2 pointer-events-none">
        <span className="text-[10px] text-gray-500 bg-white/85 px-2.5 py-1 rounded shadow-sm whitespace-nowrap">
          {isDrawing
            ? 'Release to set bounding box'
            : 'Hold Shift + drag to draw a bounding box'}
        </span>
      </div>

      {/* Clear bbox */}
      {bbox && !isDrawing && (
        <button
          onClick={() => onBboxChange(null)}
          className="absolute top-2 right-2 text-[10px] text-gray-500 bg-white/90 hover:bg-white border border-gray-200 px-2 py-0.5 rounded shadow-sm transition-colors"
        >
          Clear selection
        </button>
      )}

      {/* OSM attribution */}
      <div className="absolute bottom-2 right-2 text-[10px] text-gray-400 bg-white/80 px-1.5 py-0.5 rounded pointer-events-none">
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
      </div>
    </div>
  )
}
