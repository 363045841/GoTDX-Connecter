# domain — 领域层

tdx-api 的核心行情领域逻辑，与 HTTP 协议无关，被 `v1` 协议层依赖。

## 职责

- **K 线分页与区间查询**：按日期范围分页拉取 A 股 / 指数 / 扩展行情 K 线并过滤、去重、排序
- **分时点构建**：A 股/指数与扩展行情的分时点序列，含开盘首笔补入、指数成交额换算、全零模板识别
- **昨收解析**：当日实时行情 / 历史目标日线两种基线来源，含扩展行情实时回退策略
- **品种 kind 路由**：`stock | index | ex` 判定与主市场代码辅助

## 依赖

仅依赖 `client`（gotdx 客户端）。**不依赖** `api`、`v1`、`directory`，保证上层无环。

## 关键文件

| 文件 | 内容 |
|---|---|
| `kinds.go` | `Kind*` 常量、`IsIndexKind`、`MainExchange` |
| `kline_range.go` | `paginateFromRecent`、日期过滤、分页硬上限 |
| `kline_fetch.go` | `StockKLineRange` / `IndexKLineRange` / `ExKLineRange` |
| `timeshare.go` | 分时点构建、`ResolveStockPreClose` / `ResolveExPreClose` |
| `client_execute.go` | 对 `client` 的 Main/Ex 统一查询入口 |

## 可测注入点

全局变量在生产默认、测试可替换：

- `FetchStockKLinePage` / `FetchIndexBarsPage` / `FetchExKLinePage` — 单页 K 线拉取
- `FetchStockHistoryTick` / `FetchExHistoryTick` — 历史分时拉取
- `FetchOpeningTrade` — 开盘首笔（当日逐笔 / 历史逐笔按日期切换）

## 设计要点

- 分页按**本页实际条数**推进 start，不依赖请求的 pageSize；页数有 `maxKLinePages` 硬上限兜底
- 指数深页偶发 `invalid kline datetime`：先缩小 count 重试，仍失败且已有数据则截断返回
- 扩展行情昨收以**目标日线为 SSOT**，实时行情仅作当日日线缺失时的回退（ExQuote.PreClose 对美股不可靠）
