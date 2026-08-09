# api — HTTP 接入层

tdx-api 的旧版 HTTP 接口与路由装配，负责请求解析、领域委托与响应序列化。V1 协议由 `v1` 包提供，`api` 仅负责注册。

## 职责

- **路由装配**：`NewRouter()` 组装健康检查、旧接口（stock / ex / mac / host / symbol-search）与 `v1.RegisterRoutes`
- **旧接口 handler**：股票/指数/扩展行情的直连查询与按日期范围查询
- **分时旧接口**：`/history-tick` 委托 `domain` 构建分时点与解析昨收，输出统一契约 `{ preClose, data }`
- **搜索旧接口**：`/api/symbol/search` 包装 `directory.Cache.Search`

## 依赖

`api → v1`（注册 V1 路由）、`api → domain`（领域查询）、`api → directory`（目录缓存）、`api → client`（直连查询与状态）。

## 关键文件

| 文件 | 内容 |
|---|---|
| `router.go` | 中间件、健康检查、全部路由装配 |
| `stock.go` | `/api/stock/*` 旧接口 handler |
| `ex.go` | `/api/ex/*` 旧接口 handler |
| `mac.go` | `/api/mac/*` 行情分析中心接口 |
| `host.go` | `/api/hosts/*` 服务器探测与列表 |
| `symbol_search.go` | `/api/symbol/search` |
| `client_execute.go` | 对 `client` 的 Main/Ex/MAC 直连调用 |

## 设计要点

- 本层为**旧协议**，新前端统一走 `v1`；新旧共用同一 `domain`，避免行为分叉
- 分时旧接口与 V1 `timeshare` 共用 `domain.Build*TimeSharePoints` / `Resolve*PreClose`
- JSON 请求体结构（含 `json` tag）保留在本层，`domain` 使用与协议解耦的请求类型
