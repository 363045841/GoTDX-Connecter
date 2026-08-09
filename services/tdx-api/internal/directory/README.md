# directory — 证券目录

标准证券目录：从 gotdx 加载全部品种，内存缓存并可选 SQLite 持久化，按关键字搜索。供 `v1`（品种搜索）使用。

## 职责

- **目录加载**：`GotdxLoader` 从主站 (`StockAll`) 与扩展行情 (`ExCount` / `ExList`) 拉取品种列表
- **缓存**：`Cache` 内存缓存 + TTL 过期刷新；gotdx 故障时回退过期缓存或 SQLite 快照
- **持久化**：`SQLiteStore` 快照读写，启动时从磁盘恢复目录
- **搜索**：`Search` 按代码/名称匹配度排序返回条目

## 依赖

仅依赖 `client`（目录数据源）。被 `v1` 依赖，不反向依赖其它包。

## 关键文件

| 文件 | 内容 |
|---|---|
| `types.go` | `Item`、`Loader`、`Store`、`Snapshot`、`Kind*` 常量 |
| `search.go` | `Cache` 缓存/刷新/搜索、`GotdxLoader` |
| `store.go` | `SQLiteStore` 事务式快照读写 |

## 搜索条目

`Item` 包含 `symbol / description / exchange / source / params`，其中 `params` 携带 `market|category` 与 `kind (stock|index|ex)`，前端原样带回拉 K 线。

## 设计要点

- 主站目录按市场 `0/1/2` 全量拉取；扩展行情按页拉取
- 持久化失败不影响可用性（仅记日志），无快照时首次启动直接走 gotdx
- 重试有 `retryAt` 退避，避免故障时高频冲击数据源
