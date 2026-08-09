# client — gotdx 客户端管理

通达信 (gotdx) 客户端单例与生命周期管理，为 `domain`、`v1`、`directory` 提供主站与扩展行情两个域的统一查询入口。

## 职责

- **客户端单例**：`DefaultManager()` 懒加载，管理主站、扩展行情两域连接（MAC 域仅保留心跳/状态）
- **连接生命周期**：自动重连、故障恢复、优雅关闭
- **查询分发**：`QueryMain` / `QueryEx` 泛型入口，隔离 `gotdx` 客户端细节
- **心跳监控**：周期性心跳，连续失败超阈值触发重连
- **健康状态**：`Status` / `DomainStatus`，供就绪探针与 V1 探测使用

## 关键文件

| 文件 | 内容 |
|---|---|
| `client.go` | 单例、`MainQuerier` / `ExQuerier` 接口、查询入口 |
| `manager.go` | `Manager` / `clientSlot`：连接、重连、状态与健康报告 |
| `heartbeat.go` | 心跳监控：`StartHeartbeat`、失败阈值与重连 |

## 查询接口

- `MainQuerier` — 股票/指数行情、K 线、分时、逐笔、目录等主站能力
- `ExQuerier` — 扩展行情（期货/期权/港股/美股）能力

## 设计要点

- 两个行情域相互独立，单域故障不阻塞其它域；就绪状态按域聚合
- `Status.Ready` 由全部配置域共同决定，供 `/health/ready` 与 V1 probe 使用
- 旧 `/api/mac/*` 接口已删除，MAC 域只保留心跳与状态上报
