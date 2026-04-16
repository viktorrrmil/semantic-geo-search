# Semantic Geo Search - Technical Project Explanation

Semantic Geo Search is an application layer that makes a vector-search backend usable for geographic data. It provides a map-driven UI and a thin Go API that handles validation, browsing public spatial data on S3, previewing Parquet files, and forwarding search and indexing requests to an external backend (referred to in the code as the "main backend").

The system is intentionally split: this repository owns user workflow, visualization, and data access tooling, while the heavy semantic search and indexing engine lives elsewhere.

## Architecture at a glance

```
Browser (React + deck.gl)
  -> Go API (gin)
      -> External vector-search backend (not in this repo)
      -> S3 public buckets (list, read)
      -> DuckDB in-memory (Parquet preview)
```

### Key roles
- **Frontend:** interactive map and indexing workflow.
- **Backend:** request validation, proxying, S3 browsing, Parquet sampling, and optional query expansion.
- **External backend:** semantic search, indexing, and indexed-area metadata.

## Backend: Go API server

**Entry point:** `backend/main.go`  
**Framework:** Gin with permissive CORS (`GET`, `POST`, `OPTIONS`)  
**Config:** `.env` (loaded when present)

| Env var | Purpose | Default |
| --- | --- | --- |
| `PORT` | API port | `3001` |
| `MAIN_BACKEND_URL` | External vector backend | `http://localhost:8080` |
| `GEMINI_API_KEY` | Enables query expansion | unset |

### API surface

| Route | Method | Purpose |
| --- | --- | --- |
| `/search` | POST | Semantic search proxy with optional hybrid reranking |
| `/api/v1/query-expansion` | GET | Reports if query expansion is enabled |
| `/api/v1/list-files/*url` | GET | List public S3 bucket/prefix |
| `/api/v1/spatial-data` | GET | Preview Parquet rows via DuckDB |
| `/api/v1/index-file` | POST | Forward indexing request (async) |
| `/api/v1/geo/indexed-areas` | GET | Proxy indexed area metadata |

### Search flow (proxy + hybrid reranking)

`POST /search` performs strict payload validation and then forwards a normalized request to the external backend's `/semantic-geo-search/` endpoint. The handler accepts:
- `query` (required string)
- `top_k` or `count` (optional numeric)
- `expand` (optional boolean for query expansion)
- `hybrid` (optional boolean, default true)
- Hybrid tuning params: `alpha`, `beta`, `decay_radius_km`, `category`
- Optional map center: `center_lat`, `center_lng`

If **query expansion** is enabled (via `GEMINI_API_KEY`), the backend calls Gemini (`gemini-2.5-flash`) with a strict system prompt to expand the query while preserving named entities. A `/api/v1/query-expansion` endpoint exposes whether this feature is available so the UI can toggle it.

If **hybrid reranking** is enabled, the backend:
1. Requests a larger candidate set from the external backend (top_k * 3, min 20).
2. Normalizes semantic distances into scores.
3. Computes a geo score using Haversine distance from the provided map center with exponential decay.
4. Applies a small category bonus when categories match.
5. Produces a final weighted score and returns the trimmed top K.

This keeps the search engine simple while letting the app layer tune results for spatial context.

### Indexing flow (async)

`POST /api/v1/index-file` validates bounding box inputs, ensures finite values, and enforces `min < max` for both axes. It forwards to the external backend's `/api/v1/semantic-geo-search/index` and returns `202 Accepted`, signaling that indexing is asynchronous and should be tracked elsewhere. The response includes a summary of the request (S3 path, bbox, count, all flag).

### S3 browsing (public buckets)

`GET /api/v1/list-files/*url` supports both `s3://bucket/prefix` and `bucket/prefix` formats. It uses the AWS SDK v2 with **anonymous credentials** and `ListObjectsV2` with a `/` delimiter to emulate folders. The result is a directory-like listing with files (including size) and folders.

### Parquet preview (DuckDB)

`GET /api/v1/spatial-data` is a lightweight data peek:
- Uses a singleton in-memory DuckDB instance.
- Loads `httpfs` and `spatial` extensions on startup.
- Accepts S3 or local paths.
- Converts geometry columns to WKT with `ST_AsText`.
- Normalizes DuckDB-specific types (UUID, Decimal, Map, Interval, BLOB) into JSON-safe values.
- Enforces `k <= 10,000` and validates the region to avoid unsafe SQL interpolation.

This endpoint gives the UI a quick preview without a separate ETL step or datastore.

### Notable backend utilities

`services/indexer_service.go` contains helpers for building deterministic embedding text from structured rows and sending batch payloads to a backend `/add-batch` endpoint. These utilities are not currently wired into HTTP handlers but document the intended indexing format and can be used for offline jobs.

## Frontend: React + Vite + TypeScript

**UI stack:** React 19, Vite, Tailwind CSS, deck.gl  
**Map rendering:** deck.gl layers for points, lines, and polygons

### Main map search

The default page lets a user search across indexed areas:
- Loads indexed area metadata from `/api/v1/geo/indexed-areas`.
- Sends semantic search to `/search`.
- Displays results on the map and in a side panel.
- Exposes a "Top K" control and hybrid-ranking settings.
- Optional query expansion toggle (only shown when the backend reports it is available).

The UI sends `center_lat`/`center_lng` based on the current map view, giving the backend a location anchor for hybrid reranking.

### Map rendering (MapView)

`MapView.tsx` is the visualization engine:
- Accepts geometry as WKT or GeoJSON.
- Extracts coordinates and categorizes them into points, paths, and polygons.
- Renders:
  - Points with `ScatterplotLayer`
  - Lines with `PathLayer`
  - Polygons with `PolygonLayer`
- Supports Shift+drag to draw a bounding box for indexing.
- Builds a dim overlay that masks unindexed regions by unioning rectangles derived from indexed areas.
- Uses `FlyToInterpolator` to smoothly pan and zoom to selected results or areas.

### Indexing dashboard

The indexing page is a structured workflow:
1. Browse public S3 data and select a file or folder.
2. Draw a bounding box on the map.
3. Optionally set a row limit.
4. Submit requests to `/api/v1/index-file` for each selected file.

The UI treats indexing as asynchronous and immediately updates the indexed-areas overlay.

### S3 explorer

`S3Explorer.tsx` is a full S3 browser with:
- URL input and breadcrumb navigation.
- Region override.
- Folder selection (bulk file selection).
- Optional "index this file" dialog (used by the drawer variant).

### Data explorer and legacy UI

`DataPanel.tsx` can fetch and summarize Parquet rows from `/api/v1/spatial-data`, but it is not currently wired into `App.tsx`.  
`SearchView.tsx` targets endpoints (`/search-real`, `/preprocess`, `/chunks`) that are not implemented in the Go backend and appears to be a legacy or alternate flow.

## Production-minded choices

- **Strict validation and guardrails** (bbox validation, row limits, region sanitization).
- **Hybrid reranking** on top of the external search engine to integrate geo distance and category bias without changing the upstream backend.
- **Anonymous S3 access** to avoid credential management in the app layer.
- **Singleton DuckDB** to avoid repeated initialization and keep Parquet preview fast.
- **Explicit HTTP timeouts** for external calls.
- **Clear separation of concerns**: the app layer focuses on workflow, while search and indexing live in the external backend.

## Operational notes and limitations

- The external vector-search backend must be running and reachable at `MAIN_BACKEND_URL`.
- The frontend uses hardcoded `http://localhost:3001` endpoints.
- Query expansion is available only when `GEMINI_API_KEY` is configured.
- Some UI components reference endpoints not implemented in this repo.

## Test coverage highlights

The hybrid ranking logic includes unit tests (`hybrid_ranking_service_test.go`) that validate semantic normalization, geo scoring, and category boosts.

## Sample data

`backend/places_dubai.*` and `backend/chunks/` provide example datasets for local experimentation.
