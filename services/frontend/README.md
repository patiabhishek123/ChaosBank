# ChaosBank Frontend

Minimal React UI for ChaosBank operations with a black/white terminal aesthetic.

## Pages

- Transaction log (live polling)
- Chaos toggle button
- Replay button
- System stats

## Prerequisites

- Node.js 18+

## Run (dev)

```bash
cd services/frontend
npm install
npm run dev
```

Open http://localhost:5173

The app proxies `/api/*` to worker-service (`http://localhost:8082`) by default.

## Optional environment variables

- `VITE_API_BASE_URL` (default: `/api`)
- `VITE_WORKER_BASE_URL` for dev proxy target in `vite.config.js`

## Backend endpoints used

- `GET /health`
- `GET /transactions/log?limit=100`
- `GET /chaos`
- `POST /chaos`
- `POST /replay` (header `X-Replay-Confirm`)
- `GET /stats`
