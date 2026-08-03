# KlineChartQuantGo — Agent Guide

单一 Go module（`KlineChartQuantGo`），两个可执行服务共用依赖，目录按服务拆分。

## Quick start

在仓库**根目录**执行：

```bash
# 通达信（默认 8080）
go run . tdx
# 或: go run ./services/tdx-api

# 币安（默认 8081）
go run . binance
# 或: go run ./services/binance-api
```

根目录 `main.go` 是启动器，必须带服务名：`go run . tdx` / `go run . binance`。

## Services

| Service | Path | Port | Role |
|---|---|---|---|
| tdx-api | `./services/tdx-api` | `8080` | gotdx stock/ex/mac/host APIs |
| binance-api | `./services/binance-api` | `8081` | Binance orderbook + depth SSE |

BaoStock lives in a **separate** repo (`stockbao`, port 8000). Do not add it here.

## Key facts

- **One module**: root `go.mod` → `module KlineChartQuantGo`. No nested `go.mod` under services.
- **Go 1.26**, Gin framework, SQLite symbol directory persistence with startup warm-up, API package tests, no CI.
- Import paths: `KlineChartQuantGo/services/tdx-api/internal/...` and `KlineChartQuantGo/services/binance-api/internal/...`
- tdx-api client singleton via `client.Get()` — probes TDX hosts (2s timeout) at startup.
- binance-api defaults proxy to `http://127.0.0.1:6666` if `HTTP_PROXY` unset.
- CORS wide-open on both services.
- Binance routes are `/api/binance/*` (not the old misspelled `/api/biance/*`).

## Env vars

### tdx-api
| Var | Default | Note |
|---|---|---|
| `PORT` | `8080` | |
| `SYMBOL_DB_PATH` | `data/tdx-symbols.db` | Search directory snapshot; refreshed every 24 hours |
| `GOTDX_AUTO_SELECT` | — | `"1"` = auto-pick fastest host |
| `GOTDX_MAIN_HOSTS` | built-in | comma-separated |
| `GOTDX_EX_HOSTS` | built-in | comma-separated |
| `GOTDX_MAC_HOSTS` | built-in | comma-separated |

### binance-api
| Var | Default | Note |
|---|---|---|
| `PORT` | `8081` | |
| `SYMBOLS` | `btcusdt,ethusdt` | comma-separated |
| `HTTP_PROXY` / `HTTPS_PROXY` | `http://127.0.0.1:6666` | |

## Structure
```
go.mod                 — module KlineChartQuantGo
main.go                — launcher: go run . <tdx|binance>
services/
  tdx-api/
    main.go
    internal/client/   — gotdx singleton (probe, connect, reconnect)
    internal/api/      — handlers + router + middleware
  binance-api/
    main.go
    internal/binance/  — WS client + DepthHub
    internal/handler/  — Gin router (orderbook + SSE)
```

## Conventions
- Error responses: `{"error": "..."}` with appropriate HTTP status.
- K-line page size: `klinePageSize = 798` (`services/tdx-api/internal/api/stock.go`).
- `client.Reprobe()` reconnects if hosts change at runtime.
- Frontend caller: `KLineChartQuant` packages/core/src/data/{gotdx,binance}.ts

## Comment Style

- 每个文件必须有头部注释，说明文件用途。
- 每个函数必须有注释，说明其职责、参数和返回值；简单函数可使用简短注释。
- 关键代码必须有注释，说明实现意图、业务规则或不直观的处理逻辑。
- 每个测试用例必须有中文注释，说明验证的行为和场景。
- 注释正文使用中文，技术术语保留英文。
- 注释必须简单明了，直接说明代码是什么或为什么这样实现，尽量使用一句话，避免冗长和重复代码本身的含义。
