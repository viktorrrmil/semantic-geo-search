# semantic-geo-search

Semantic Geo Search is an application layer for browsing spatial data, drawing geographic bounding boxes, and sending indexing/search requests to an external vector-search backend.

This repository contains two runnable parts:

- `backend/` — a Go API that proxies search and indexing requests, lists public S3 folders, and previews Parquet data with DuckDB.
- `frontend/` — a React + TypeScript + Vite UI for map search and indexing.

The actual vector-search engine is not included in this repo. The Go backend forwards the search and indexing work to that separate service.

## What this project does

- Browse spatial datasets from public S3 locations.
- Preview Parquet files locally or from S3.
- Draw a bounding box on the map and start indexing for a file or folder.
- Search indexed areas from the map UI.
- Show indexed area coverage and search results on an interactive map.

## Project layout

```text
semantic-geo-search/
├── backend/          Go API server and helper services
├── frontend/         React UI
├── explanation.md    Longer architecture and implementation notes
├── README.md         This file
```

Sample data lives under `backend/`:

- `places_dubai.json`
- `places_dubai.parquet`
- `chunks/`

## Prerequisites

You will need:

- Go 1.25 or newer
- Node.js 20 or newer
- npm
- Access to the external vector-search backend the app talks to

## Configuration

### Backend

The Go server reads these environment variables:

- `PORT` — HTTP port for the Go API, default `3001`
- `MAIN_BACKEND_URL` — URL of the external vector-search backend, default `http://localhost:8080`

### Frontend

The frontend currently uses hardcoded local endpoints:

- `http://localhost:3001` for the Go API
- `http://localhost:3001/api/v1/...` for data and indexing endpoints

If you want to point the UI at a different backend host, update the constants in `frontend/src/App.tsx`.

## Run the backend

From the repository root:

```bash
cd backend
go run .
```

By default the server starts on port `3001`.

If you need to change the port or upstream backend URL:

```bash
cd backend
PORT=3001 MAIN_BACKEND_URL=http://localhost:8080 go run .
```

## Run the frontend

In a second terminal:

```bash
cd frontend
npm install
npm run dev
```

The Vite dev server will print the local URL it is serving on.

## Typical workflow

1. Start the Go backend.
2. Start the frontend.
3. Open the app in the browser.
4. Use the main map view to search indexed areas.
5. Open the indexing dashboard to:
   - select a file or folder from S3,
   - draw a bounding box on the map,
   - enter an optional row limit,
   - click **Index selection**.

## API summary

The Go backend exposes these main routes:

- `POST /search` — semantic search request proxy
- `GET /api/v1/geo/indexed-areas` — list indexed areas
- `GET /api/v1/list-files/*url` — list public S3 folders
- `GET /api/v1/spatial-data` — preview Parquet rows
- `POST /api/v1/index-file` — start indexing for a file and bbox

## Frontend notes

- The main app lives in `frontend/src/App.tsx`.
- The map is built with deck.gl.
- The indexing dashboard lets you submit multiple index requests back to back.
- The `frontend/README.md` is still the default Vite template and is not specific to this project.

## Caveats

- The external vector-search backend must be running separately.
- The frontend currently uses localhost URLs directly instead of environment variables.
- `SearchView.tsx` references endpoints that are not implemented in this repo, so it appears to be an older or alternate flow.

## More detail

For a deeper explanation of the architecture and request flow, see `explanation.md`.
