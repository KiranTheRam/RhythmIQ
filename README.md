# RhythmIQ

RhythmIQ is a full-stack Go + React Spotify analytics app that produces Wrapped-style insights year-round.

## What It Does

- Spotify OAuth authentication (Authorization Code flow)
- Collects and stores snapshot metrics over time (SQLite or Postgres)
- Wrapped-style metrics:
  - Top artists/tracks for short, medium, and long term
  - Estimated daily and yearly listening minutes
  - Genre gravity and diversity
  - Consistency, discovery, and replay scores
  - Library stats (saved tracks, playlists, followed artists)
- Historical trend analysis for dashboards and charting
- Recommendation engine:
  - Built-in heuristic recommendations
  - Optional OpenAI-generated narrative insights (`OPENAI_API_KEY`)
- Desktop-first web UI with responsive mobile layout
- Installable PWA with offline caching

## Tech Stack

- Backend: Go, Chi, SQLite (`modernc.org/sqlite`), Postgres (`pgx`)
- Frontend: React + TypeScript + Vite + Recharts
- PWA: `vite-plugin-pwa`

## Project Structure

- `cmd/server/main.go`: app entrypoint + static serving
- `internal/api`: HTTP handlers (`/api/...`)
- `internal/spotify`: OAuth + Spotify API client
- `internal/service`: metrics and recommendation logic
- `internal/db`: persistence schema + repository
- `web`: React dashboard and PWA app

## Setup

1. Create a Spotify app in the Spotify developer dashboard.
2. Add this Redirect URI in Spotify settings:
   - `http://127.0.0.1:8080/api/auth/callback`
   - This must exactly match `SPOTIFY_REDIRECT_URL` in `.env`.
   - Spotify no longer accepts `localhost` redirect URIs.
3. Configure environment:

```bash
cp .env.example .env
# edit .env with Spotify credentials
set -a; source .env; set +a
```

Security for public hosting:
- Set `RHYTHMIQ_BASE_URL` to your public `https://` origin.
- Set `SPOTIFY_REDIRECT_URL` to the matching `https://.../api/auth/callback` URL.
- Set a strong `RHYTHMIQ_SESSION_SECRET` (32+ random chars).
- Optionally set `RHYTHMIQ_ALLOWED_ORIGINS` as a comma-separated allowlist (defaults to `RHYTHMIQ_BASE_URL` for non-loopback deployments).
- Do not rotate `RHYTHMIQ_SESSION_SECRET` casually in production: it signs sessions and is also used to encrypt stored Spotify tokens.

Database selection:
- Default: `RHYTHMIQ_DB_DRIVER=postgres` with `RHYTHMIQ_DB_DSN`
- Optional SQLite override: `RHYTHMIQ_DB_DRIVER=sqlite` with `RHYTHMIQ_DB_PATH`

## Run

1. Build frontend:

```bash
cd web
npm install
npm run build
cd ..
```

2. Run backend server:

```bash
go run ./cmd/server
```

3. Open:

- `http://127.0.0.1:8080`
- Use `127.0.0.1` (not `localhost`) so auth host and redirect host stay aligned.

## One-Command Docker Startup

1. Create `.env` from template and fill Spotify credentials:

```bash
cp .env.example .env
```

2. Start everything:

```bash
docker compose up --build
```

3. Open:

- `http://127.0.0.1:8080`
- Use `127.0.0.1` consistently for Spotify auth.

Docker now starts with Postgres by default.
The Postgres database persists in the Docker volume `rhythmiq_pgdata`.
Postgres is network-isolated inside Docker by default (not published on host ports).

### Docker + SQLite Override

If you want file-based local storage instead:

```bash
cp .env.example .env
# set RHYTHMIQ_DB_DRIVER=sqlite
docker compose up --build
```

## Development Workflow

- Frontend dev server:

```bash
cd web
npm run dev
```

- Backend (separate terminal):

```bash
go run ./cmd/server
```

Vite proxies `/api` calls to `http://127.0.0.1:8080`.

## API Endpoints

- `GET /api/health`
- `GET /api/auth/status`
- `GET /api/auth/login`
- `GET /api/auth/callback`
- `POST /api/auth/logout`
- `POST /api/metrics/refresh`
- `GET /api/metrics/latest`
- `GET /api/metrics/history?days=180`
- `GET /api/recommendations/insights`

## Notes

- Historical analysis improves as more snapshots are collected.
- Auth is session-based per browser/client (multiple users can use the same deployment concurrently).
- Server auto-collects snapshots periodically for all stored users.
- OpenAI integration is optional; without `OPENAI_API_KEY`, heuristic recommendations are still generated.
- API now includes response hardening headers and endpoint rate limits for auth and costly generation/refresh paths.
- Stored Spotify access/refresh tokens are encrypted at rest in both Postgres and SQLite.
