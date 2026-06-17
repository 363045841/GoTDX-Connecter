# KlineChartQuantGo — Agent Guide

## Quick start
```bash
go build -o kline.exe .
PORT=8080 ./kline.exe
```

## Key facts
- **Go 1.26** (`go.mod`), standard library `net/http` only — no framework, no DB, no tests.
- **Single package** (`module KlineChartQuantGo`), no sub-modules.
- Repo replaces `github.com/bensema/gotdx` → `github.com/363045841/gotdx` (fork). `go mod tidy` may fail if the fork is unreachable.
- Client conn singleton created at startup via `client.Get()` — probes TDX hosts (2s timeout). Errors are fatal.
- All API handlers are `POST` (JSON body) except `GET /api/hosts/list`. CORS wide-open (any origin, GET/POST/OPTIONS).
- **No Makefile, no CI, no linter, no tests, no migrations, no generated code.** Minimal setup.

## Env vars
| Var | Default | Note |
|---|---|---|
| `PORT` | `8080` | |
| `GOTDX_AUTO_SELECT` | — | Set to `"1"` to auto-pick fastest host |
| `GOTDX_MAIN_HOSTS` | built-in | Override main TDX hosts (comma-separated) |
| `GOTDX_EX_HOSTS` | built-in | Override ex TDX hosts (comma-separated) |

## Build & run
```
go build -o kline.exe .
```
Single binary, no extra steps. No dev server or hot-reload.

## Structure
```
main.go              — entrypoint, starts HTTP server
internal/client/     — gotdx client singleton (probe, connect, reconnect)
internal/api/        — request handlers + router + middleware
```

## Conventions
- Error responses: `{"error": "..."}` with appropriate HTTP status.
- K-line page size constant: `klinePageSize = 798` (in `internal/api/stock.go`).
- `client.Reprobe()` reconnects if hosts change at runtime (used by `/api/hosts/probe`).
