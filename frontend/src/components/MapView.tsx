import { useState, useRef, useEffect } from 'react'
import DeckGL from '@deck.gl/react'
import { TileLayer } from '@deck.gl/geo-layers'
import { BitmapLayer, PolygonLayer, ScatterplotLayer, PathLayer } from '@deck.gl/layers'
import { WebMercatorViewport } from '@deck.gl/core'
import type { MapViewState, PickingInfo } from '@deck.gl/core'
import type { SpatialFeature } from './DataPanel'

export interface BBox {
  minX: number
  minY: number
  maxX: number
  maxY: number
}

interface Props {
  features: SpatialFeature[]
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

/** Compute a bounding box that contains all feature geometries */
function featuresBbox(features: SpatialFeature[]): [[number, number], [number, number]] | null {
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
  let any = false
  for (const f of features) {
    const all = parseCoords(f.geometry)
    for (const [x, y] of all) {
      if (x < minX) minX = x; if (x > maxX) maxX = x
      if (y < minY) minY = y; if (y > maxY) maxY = y
      any = true
    }
  }
  return any ? [[minX, minY], [maxX, maxY]] : null
}

// ── Geometry categorization ───────────────────────────────────────────────────

interface PointDatum  { pos: [number, number]; feature: SpatialFeature }
interface PolyDatum   { rings: [number, number][][]; feature: SpatialFeature }
interface PathDatum   { path: [number, number][]; feature: SpatialFeature }

function categorize(features: SpatialFeature[]) {
  const points: PointDatum[] = []
  const polys:  PolyDatum[]  = []
  const paths:  PathDatum[]  = []

  for (const f of features) {
    const g = f.geometry?.trim() ?? ''
    const upper = g.toUpperCase()

    if (upper.startsWith('POINT')) {
      const c = parseCoords(g)
      if (c[0]) points.push({ pos: c[0], feature: f })

    } else if (upper.startsWith('MULTIPOINT')) {
      for (const ring of innerRings(g)) {
        const c = parseCoords(ring)
        if (c[0]) points.push({ pos: c[0], feature: f })
      }

    } else if (upper.startsWith('POLYGON') || upper.startsWith('MULTIPOLYGON')) {
      // Each innermost paren group is a ring; render each as a filled polygon.
      for (const ring of innerRings(g)) {
        const c = parseCoords(ring)
        if (c.length >= 3) polys.push({ rings: [c], feature: f })
      }

    } else if (upper.startsWith('LINESTRING') || upper.startsWith('MULTILINESTRING')) {
      for (const ring of innerRings(g)) {
        const c = parseCoords(ring)
        if (c.length >= 2) paths.push({ path: c, feature: f })
      }
    }
  }

  return { points, polys, paths }
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
    try { const n = JSON.parse(names); if (n?.primary) return String(n.primary) } catch { /**/ }
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

export default function MapView({ features }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [viewState, setViewState] = useState<MapViewState>(INITIAL_VIEW_STATE)
  const vsRef = useRef<MapViewState>(INITIAL_VIEW_STATE)

  // Fly to fit features whenever the feature list changes
  useEffect(() => {
    if (!features.length || !containerRef.current) return
    const bbox = featuresBbox(features)
    if (!bbox) return
    const { clientWidth: w, clientHeight: h } = containerRef.current
    if (w === 0 || h === 0) return

    try {
      const vp = new WebMercatorViewport({ width: w, height: h })
      const { longitude, latitude, zoom } = vp.fitBounds(bbox, { padding: 60 })
      const next = { ...vsRef.current, longitude, latitude, zoom: Math.min(zoom, 18) }
      vsRef.current = next
      setViewState(next)
    } catch { /* fitBounds can throw for degenerate bbox */ }
  }, [features])

  const { points, polys, paths } = categorize(features)

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

  const layers = [OSM_LAYER, polysLayer, pathsLayer, pointsLayer].filter(Boolean)

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
        onViewStateChange={({ viewState: vs }) => {
          const next = vs as MapViewState
          vsRef.current = next
          setViewState(next)
        }}
        controller
        layers={layers}
        style={{ width: '100%', height: '100%' }}
        getTooltip={getTooltip}
      />

      {/* Feature count overlay */}
      {features.length > 0 && (
        <div className="absolute top-2 left-1/2 -translate-x-1/2 pointer-events-none">
          <span className="text-[10px] text-gray-600 bg-white/90 px-2.5 py-1 rounded shadow-sm whitespace-nowrap">
            {features.length} feature{features.length !== 1 ? 's' : ''} loaded
          </span>
        </div>
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
