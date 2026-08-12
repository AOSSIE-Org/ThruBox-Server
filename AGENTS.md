# AGENTS.md

Instructions for AI coding agents working in this repository.

## Project Overview

ThruBox Server is a self-hostable relay server acting as a "dumb encrypted mailbox": it stores and forwards opaque encrypted payloads via a REST API. It never sees plaintext — all encryption/decryption happens client-side in the [ThruBox Client SDK](https://github.com/AOSSIE-Org/ThruBox-Client) or other consumers.

## Repository Layout

- `cmd/relay/` — server entrypoint
- Storage: embedded SQLite (WAL mode) via `mattn/go-sqlite3`
- `public/` — logo assets referenced by README
- `brand/` — logo, favicons, and brand guidelines (see `brand/Brand.md`)

## Build, Test & Lint

```bash
go mod download
go build -o relay-server ./cmd/relay
go vet ./...
go test ./...
./relay-server   # run in a separate terminal — this blocks
```

Docker:

```bash
docker compose up -d
```

**Note:** there are no `_test.go` files in the repository yet. If you add functionality, add tests alongside it — don't rely on this note as an excuse to skip tests.

## Hard Constraints

- Keep runtime dependencies minimal: standard library `net/http` plus `mattn/go-sqlite3`, `google/uuid`, and `yaml.v3` (see `go.mod`) — don't add a web framework, ORM, or other new dependency without discussing it in an issue first.
- All SQL must be parameterized (no string-concatenated queries) — this is a relay storing arbitrary payloads, so injection surface must stay closed.
- The server must never decrypt or inspect message payloads; it only stores/forwards opaque blobs.

## Conventions

- Configuration is read from `config.yaml` or environment variables (see the table in `README.md` "Configuration") — don't hardcode values that are already configurable.
