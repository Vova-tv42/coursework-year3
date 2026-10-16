# Commit Radar

Analysis of student Git repositories.

## Setup

Requirements: Docker, Go 1.27, bun 1.3.

1. Create the env file and set your own Postgres password (in `POSTGRES_PASSWORD` and `DATABASE_URL`):
   ```sh
   cp .env.example .env
   ```
   Change `POSTGRES_HOST_PORT` if default port is already in use.
2. Install web dependencies:
   ```sh
   cd web && bun install
   ```

## Development

```sh
make dev
```

This starts Postgres in Docker, the Go server on `:8080` and the Vite dev server.
Open http://localhost:5173.

- Web changes are applied automatically in the browser.
- After Go changes stop `make dev` with Ctrl-C and run it again.

To run the full app the same way as on the server:

```sh
make up
```

Open http://localhost:8080. Run `make up` again after code changes to rebuild.

## Other commands

| Command | What it does |
|---|---|
| `make test` | Go tests |
| `make lint` | golangci-lint and oxlint |
| `docker compose restart app` | restart the server container |
| `make down` | stop all containers after `make up` or `make dev` (data is kept) |
| `docker compose down -v` | stop and delete the database and cloned repositories |
