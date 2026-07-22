# RhythmIQ

RhythmIQ is a Go + React dashboard for your Spotify listening, read three ways: this week, this month, and this year.

## What It Shows

Each time window has its own top artists, top tracks, and genre mix. Alongside them:

- **On repeat** — the single most repeated track in your recent plays
- **When you listened** — a 24-hour ridgeline and weekday breakdown, bucketed in your own timezone
- **New to you** — artists in the current window that are absent from your year
- **In your library** — saved tracks, playlists, artists followed

The page takes its accent colour from the photograph of whoever is at no.1, so switching
periods re-tints the whole spread.

### Where the numbers come from

Spotify does not expose true listening minutes, so nothing here extrapolates one.

| Window | Source | What is measurable |
| --- | --- | --- |
| Week | `recently-played` (last 50 plays) | Real play counts, real timestamps, real minutes |
| Month | `top/*?time_range=short_term` (~4 weeks) | Rank order only |
| Year | `top/*?time_range=long_term` (~12 months) | Rank order only |

Play counts appear only on the week, because it is the only window built from
actual playback events. The month and year views show rank and track duration.

## Tech Stack

- Backend: Go, Chi, SQLite (`modernc.org/sqlite`), Postgres (`pgx`)
- Frontend: React + TypeScript + Vite (no charting library; the visuals are hand-built SVG and CSS)
- PWA: `vite-plugin-pwa`

## Project Structure

- `cmd/server/main.go`: app entrypoint + static serving
- `internal/api`: HTTP handlers (`/api/...`)
- `internal/spotify`: OAuth + Spotify API client
- `internal/service`: dashboard assembly
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
- `GET /api/dashboard`
- `POST /api/dashboard/refresh`

## Notes

- Auth is session-based per browser/client (multiple users can use the same deployment concurrently).
- Each user's dashboard is cached in the database and refreshed in the background every 6 hours, so page loads never wait on Spotify.
- API includes response hardening headers and endpoint rate limits for auth and refresh paths.
- Stored Spotify access/refresh tokens are encrypted at rest in both Postgres and SQLite.
