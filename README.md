# Semantic Geo Search

Semantic Geo Search is an application layer that turns a vector-search backend into a usable geospatial search product.

It provides a map-driven interface and a lightweight Go orchestration layer for dataset browsing, indexing workflows, and search execution over geographic data.

The underlying vector search and indexing engine runs in a separate service ([Lynx](https://github.com/viktorrrmil/lynx)). This repository focuses on workflow, geospatial interaction, and retrieval orchestration.

This project is intentionally split: the backend focuses on workflow and data access, while the external search service focuses on embeddings, indexing, and retrieval. The result is a production-minded system for geo discovery without bundling a full search engine into the UI layer.

## What the system does

- Browse public spatial datasets in S3.
- Preview Parquet files locally or over S3.
- Draw bounding boxes on a map to scope indexing.
- Run semantic searches across indexed areas.
- Visualize indexed coverage and search results on the map.

## Architecture
This design enables semantic retrieval systems to be extended with geospatial and UI-driven constraints without modifying the underlying search engine.
```
Browser (React + deck.gl)
  -> Go API (gin)
      -> External vector-search backend
      -> Public S3 (anonymous listing)
      -> DuckDB (Parquet preview)
```

### Why this architecture
- **Thin API layer:** validates inputs and keeps the UI decoupled from the upstream search engine.
- **Hybrid ranking:** application-level ranking layer that combines semantic similarity with geospatial distance and optional category bias, without modifying the underlying search engine.
- **DuckDB preview:** fast Parquet sampling without standing up a data warehouse.
- **Anonymous S3 browsing:** no credentials needed for public buckets.

## Repository layout

```text
semantic-geo-search/
├── backend/          Go API server and services
├── frontend/         React + Vite UI
├── explanation.md    Deep dive on architecture and flows
└── README.md         This file
```

Sample data lives under `backend/`:
`places_dubai.json`, `places_dubai.parquet`, and `chunks/`.

## Backend overview (Go)

**Runtime:** Go 1.25+  
**Framework:** Gin  
**Config:** `.env` (optional)

### Key routes

| Route | Method | Purpose |
| --- | --- | --- |
| `/search` | POST | Proxy search with optional hybrid reranking |
| `/api/v1/query-expansion` | GET | Reports query expansion availability |
| `/api/v1/list-files/*url` | GET | List public S3 bucket/prefix |
| `/api/v1/spatial-data` | GET | Preview Parquet rows via DuckDB |
| `/api/v1/index-file` | POST | Forward indexing request (async) |
| `/api/v1/geo/indexed-areas` | GET | Proxy indexed area metadata |

### Notable backend decisions
- **Strict validation:** bbox sanity checks, row limits, and region sanitization.
- **Hybrid reranking:** optional scoring layer that blends semantic distance with geo distance.
- **Query expansion:** optional Gemini-based query enrichment gated by `GEMINI_API_KEY`.
- **DuckDB singleton:** preloads `httpfs` and `spatial` extensions and normalizes types to JSON.

## Frontend overview (React)

**Stack:** React 19, Vite, Tailwind CSS, deck.gl

### Main flows
1. **Map search:** query indexed areas, visualize results, and inspect scores.
2. **Indexing dashboard:** select S3 files, draw a bbox, and submit async indexing.

### Notable frontend decisions
- Supports **WKT and GeoJSON** for geometry rendering.
- Uses deck.gl layers for **points, lines, and polygons** with interactive selection.
- Map center is fed into search requests to drive geo-aware reranking.

## Configuration

### Backend env vars

| Variable | Purpose | Default |
| --- | --- | --- |
| `PORT` | API port | `3001` |
| `MAIN_BACKEND_URL` | External search backend | `http://localhost:8080` |
| `GEMINI_API_KEY` | Enables query expansion | unset |

### Frontend endpoints

The UI currently uses hardcoded localhost URLs:
- `http://localhost:3001` for search
- `http://localhost:3001/api/v1/...` for API endpoints

To point at a different host, update the constants in `frontend/src/App.tsx`.

## Running locally

### Backend

```bash
cd backend
go run .
```

With custom settings:

```bash
cd backend
PORT=3001 MAIN_BACKEND_URL=http://localhost:8080 go run .
```

### Frontend

```bash
cd frontend
npm install
npm run dev
```

## Typical workflow

1. Start the backend and frontend.
2. Search indexed areas from the main map view.
3. Open the indexing dashboard to:
   - select a file or folder in S3,
   - draw a bounding box,
   - optionally set a row limit,
   - start indexing.

## Caveats and known edges

- The external vector-search backend must be running separately.
- Frontend endpoints are hardcoded to localhost.
- `SearchView.tsx` references endpoints not implemented by this backend (legacy flow).

## More detail

See `explanation.md` for a deeper technical walkthrough and rationale.
