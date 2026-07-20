# KlineChartQuantGo

多数据源行情代理 — 单一 Go module，为 [KLineChartQuant](https://github.com/363045841/KLineChartQuant) 提供本地后端。

| 服务 | 包路径 | 默认端口 | 说明 |
|---|---|---|---|
| **tdx-api** | `./services/tdx-api` | `8080` | 通达信 (gotdx) 股票/期货/MAC 行情 |
| **binance-api** | `./services/binance-api` | `8081` | 币安 L2 订单簿 + SSE 深度流 |

> BaoStock 等其它数据源由独立仓库提供（如 `stockbao`，端口 `8000`），不在本仓库。

## 快速开始

在仓库根目录执行（module: `KlineChartQuantGo`）：

```bash
# 通达信（默认 8080）
go run . tdx
# 或: go run ./services/tdx-api

# 币安（默认 8081）
go run . binance
# 或: go run ./services/binance-api
```

或构建二进制：

```bash
go build -o tdx-api.exe ./services/tdx-api
go build -o binance-api.exe ./services/binance-api
```

## 环境变量

### tdx-api

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `8080` | HTTP 服务端口 |
| `GOTDX_AUTO_SELECT` | — | 设为 `"1"` 自动选择最快服务器 |
| `GOTDX_MAIN_HOSTS` | 内置列表 | 覆盖主站探测地址（逗号分隔） |
| `GOTDX_EX_HOSTS` | 内置列表 | 覆盖行情探测地址（逗号分隔） |
| `GOTDX_MAC_HOSTS` | 内置列表 | 覆盖 MAC 探测地址（逗号分隔） |

### binance-api

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `8081` | HTTP 服务端口 |
| `SYMBOLS` | `btcusdt,ethusdt` | 订阅交易对（逗号分隔） |
| `HTTP_PROXY` / `HTTPS_PROXY` | `http://127.0.0.1:6666` | 访问币安的代理 |

## API 概览

所有接口返回 JSON，已全局开启 CORS。

### tdx-api — 股票 (Stock)

| 方法 | 路由 | 说明 |
|---|---|---|
| POST | `/api/stock/quotes` | 批量查询行情 |
| POST | `/api/stock/kline` | K 线数据（按起始索引+数量） |
| POST | `/api/stock/kline-by-date` | K 线数据（按日期范围） |
| POST | `/api/stock/kline-count` | K 线记录总数 |
| POST | `/api/stock/tick` | 分笔成交 |
| POST | `/api/stock/history-tick` | 历史分时 |
| POST | `/api/stock/list` | 股票列表（分页） |
| POST | `/api/stock/count` | 股票数量 |
| POST | `/api/stock/index-info` | 指数信息 |
| POST | `/api/stock/transaction` | 实时成交 |
| POST | `/api/stock/history-transaction` | 历史成交 |

### tdx-api — 期货/期权 (Exchange)

| 方法 | 路由 | 说明 |
|---|---|---|
| POST | `/api/ex/count` | 品种数量 |
| POST | `/api/ex/list` | 品种列表（分页） |
| POST | `/api/ex/quote` | 单品种行情 |
| POST | `/api/ex/quotes` | 批量行情 |
| POST | `/api/ex/kline` | K 线数据 |
| POST | `/api/ex/kline-by-date` | K 线数据（按日期） |
| POST | `/api/ex/tick` | 分笔成交 |
| POST | `/api/ex/history-transaction` | 历史成交 |
| POST | `/api/ex/table` | 合约表 |

### tdx-api — 行情分析中心 (MAC)

| 方法 | 路由 | 说明 |
|---|---|---|
| POST | `/api/mac/board-list` | 板块列表 |
| POST | `/api/mac/board-members` | 板块成分股 |
| POST | `/api/mac/board-members-quotes` | 成分股行情（静态） |
| POST | `/api/mac/board-members-quotes-dynamic` | 成分股行情（动态，可排序） |
| POST | `/api/mac/symbol-quotes` | 批量代码行情 |
| POST | `/api/mac/quotes` | 单代码行情 |
| POST | `/api/mac/transactions` | 成交记录 |
| POST | `/api/mac/auction` | 竞价数据 |
| POST | `/api/mac/tick-charts` | 分时图 |
| GET | `/api/mac/server-info` | 服务器交易日信息 |
| POST | `/api/mac/symbol-info` | 代码信息 |
| POST | `/api/mac/capital-flow` | 资金流向 |
| POST | `/api/mac/market-monitor` | 市场监控 |

### tdx-api — 主机探测

| 方法 | 路由 | 说明 |
|---|---|---|
| POST | `/api/hosts/probe` | 探测指定类型服务器可达性 |
| GET | `/api/hosts/list` | 列出所有已知服务器地址 |

### binance-api

| 方法 | 路由 | 说明 |
|---|---|---|
| GET | `/api/binance/orderbook?symbol=btcusdt` | 订单簿快照（前 20 档） |
| GET | `/api/binance/depth-events?symbol=btcusdt` | L2 深度 SSE 流（snapshot + delta） |

## 项目结构

```
├── go.mod                       # 唯一 module: KlineChartQuantGo
├── go.sum
├── main.go                      # 启动器: go run . <tdx|binance>
├── README.md
├── AGENTS.md
└── services/
    ├── tdx-api/                 # 通达信行情代理
    │   ├── main.go
    │   └── internal/
    │       ├── client/          # gotdx 客户端单例
    │       └── api/             # 路由 + handlers
    └── binance-api/             # 币安深度代理
        ├── main.go
        └── internal/
            ├── binance/         # WS 订单簿 + DepthHub
            └── handler/         # HTTP / SSE 路由
```

## 技术栈

- **Go 1.26** + Gin
- **gotdx** — 通达信协议 Go 实现（tdx-api）
- **gorilla/websocket** — 币安 WebSocket（binance-api）
- 无数据库，纯实时代理转发
