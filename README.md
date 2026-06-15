# KlineChartQuantGo

通达信金融数据 REST API 服务 — 使用 Go 语言实现，通过 [`gotdx`](https://github.com/bensema/gotdx) 协议连接通达信行情服务器，提供股票、期货、期权等市场数据的 HTTP 接口。

## 快速开始

```bash
go build -o kline.exe .
./kline.exe
```

服务默认监听 `8080` 端口，可通过 `PORT` 环境变量修改。

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `8080` | HTTP 服务端口 |
| `GOTDX_AUTO_SELECT` | — | 设为 `"1"` 自动选择最快服务器 |
| `GOTDX_MAIN_HOSTS` | 内置列表 | 覆盖主站探测地址（逗号分隔） |
| `GOTDX_EX_HOSTS` | 内置列表 | 覆盖行情探测地址（逗号分隔） |

## API 概览

所有接口返回 JSON，POST 接口接受 JSON 请求体，已全局开启 CORS。

### 股票 (Stock)

| 方法 | 路由 | 说明 |
|---|---|---|
| POST | `/api/stock/quotes` | 批量查询行情 |
| POST | `/api/stock/kline` | K 线数据（按起始索引+数量） |
| POST | `/api/stock/kline-by-date` | K 线数据（按日期范围） |
| POST | `/api/stock/kline-count` | K 线记录总数 |
| POST | `/api/stock/tick` | 分笔成交 |
| POST | `/api/stock/list` | 股票列表（分页） |
| POST | `/api/stock/count` | 股票数量 |
| POST | `/api/stock/index-info` | 指数信息 |
| POST | `/api/stock/transaction` | 实时成交 |
| POST | `/api/stock/history-transaction` | 历史成交 |

### 期货/期权 (Exchange)

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

### 行情分析中心 (MAC)

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
| POST | `/api/mac/symbol-info` | 代码信息 |
| POST | `/api/mac/capital-flow` | 资金流向 |
| POST | `/api/mac/market-monitor` | 市场监控 |

### 主机探测

| 方法 | 路由 | 说明 |
|---|---|---|
| POST | `/api/hosts/probe` | 探测指定类型服务器可达性 |
| GET | `/api/hosts/list` | 列出所有已知服务器地址 |

## 项目结构

```
├── main.go                 # 入口，启动 HTTP 服务
├── internal/
│   ├── client/client.go    # TDX 客户端单例
│   └── api/
│       ├── router.go       # 路由注册与中间件
│       ├── stock.go        # 股票接口处理
│       ├── ex.go           # 期货接口处理
│       ├── mac.go          # MAC 接口处理
│       └── host.go         # 主机探测接口
├── go.mod
└── go.sum
```

## 技术栈

- **Go 1.26** — 标准库 `net/http`，无第三方框架
- **gotdx** — 通达信协议 Go 实现
- 无数据库，纯实时代理转发