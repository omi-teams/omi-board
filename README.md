# omi-board

Hệ thống quản lý ticket cho omi.

## Layout

- `server/` Go API (`/healthz` checks the database connection).
- `web/` Next.js frontend (`output: standalone`, no API routes).
- `Dockerfile` server image; `web/Dockerfile` web image (build with the repo root as context).
- `docker-compose.yml` postgres 17, server, web.

## Run locally

    cp .env.example .env   # set SESSION_SECRET (32+ bytes), or rely on the compose dev fallback
    docker compose up --build
    curl localhost:8080/healthz

## Test

    sh scripts/test.sh

Runs `go test ./...` in `server/` and `npm run lint && npm run build` in `web/`. If `DATABASE_URL` is unset it starts a throwaway Postgres cluster (needs `initdb` and `pg_ctl` on PATH). CI runs the same script against a Postgres service container.

## Configuration

Environment only; see `.env.example`: `DATABASE_URL`, `PORT`, `PUBLIC_API_URL`, `PUBLIC_WEB_URL`, `SESSION_SECRET`, `POSTGRES_PASSWORD`.
