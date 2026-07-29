# Index preClose 直读 gotdx Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 指数 history-tick 的 `preClose` 直接使用 gotdx 日线 `PreClose`/`LastClose`，删除「往前多看几天用前一日 Close 推昨收」的业务层计算。

**Architecture:** gotdx 新版在 `GetSecurityBars` / `GetIndexBars` / `ExGetKLine` 解析时已 `Count+1` 并用上一根收盘价填充 `PreClose`/`LastClose`。`tryIndexBars` 已把这两个字段映射进 `proto.SecurityBar`。因此 `handleStockHistoryTickWithDeps` 对指数只需把 `dailyBars` 指到 `IndexKLineRange(目标日, 目标日)`，与股票路径一样走 `securityBarPreClose`；删除多日回看与 `bars[i-1].Close` 覆盖。

**Tech Stack:** Go 1.22+、gin、gotdx（`replace` → `github.com/363045841/gotdx`）、vitest 无关（仅 tdx-api 单测）。

**Out of scope:**
- `baseUnit` / `39xx` 逐笔价格除数（已在 fork，与 preClose 无关）
- 当日实时昨收仍走 `StockQuotesDetail` / `ExQuote` 的 `PreClose`
- 前端 `packages/core` 无需改动（仍消费 API 的 `preClose` 字段）

---

## File map

| File | Role |
|------|------|
| `services/tdx-api/internal/api/stock.go` | 删除指数 preClose 推算分支；指数 `dailyBars` 只拉目标日 |
| `services/tdx-api/internal/api/stock_test.go` | 改写指数 history-tick 测例：读 bar.PreClose，不再依赖「前一日 Close」 |
| `services/tdx-api/internal/api/ex.go` | **不改**（扩展已读 `exKLinePreClose`） |
| `go.mod` | **不改**（已 pin fork，含 PreClose 字段） |

---

### Task 1: 重写指数 history-tick 失败/成功测例（TDD）

**Files:**
- Modify: `services/tdx-api/internal/api/stock_test.go:113-204`

- [ ] **Step 1: 把「缺前一日」测例改成「目标日 bar 无有效 PreClose」**

旧测例 `TestStockHistoryTickIndexRejectsMissingPreviousDailyBar` 断言文案 `previous index daily bar not found`，依赖多日推算逻辑。删除后应改为：指数 `dailyBars` 返回目标日 bar 但 `PreClose`/`LastClose` 均为 0 → 502 + `invalid historical preClose`。

用下面整段替换 `TestStockHistoryTickIndexRejectsMissingPreviousDailyBar`：

```go
func TestStockHistoryTickIndexRejectsMissingPreCloseOnDailyBar(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	fetchTick := func(uint32, uint8, string) ([]proto.HistoryMinuteTimeData, error) {
		return []proto.HistoryMinuteTimeData{}, nil
	}
	preCloseSource := timeSharePreCloseSource{
		now: func() time.Time { return time.Date(2026, 7, 27, 10, 0, 0, 0, loc) },
		quote: func(uint8, string) (float64, error) {
			return 0, errors.New("realtime quote must not be called")
		},
		dailyBars: func(market uint8, code string, date time.Time) ([]proto.SecurityBar, error) {
			if market != 0 || code != "399001" || date.Format("20060102") != "20241213" {
				t.Fatalf("daily bar request = %d/%s/%s", market, code, date.Format("20060102"))
			}
			// 模拟 gotdx 未给出昨收（PreClose/LastClose 均为 0）
			return []proto.SecurityBar{{
				Close:    10713.07,
				Year:     2024,
				Month:    12,
				Day:      13,
				DateTime: time.Date(2024, 12, 13, 15, 0, 0, 0, loc),
			}}, nil
		},
	}

	router := gin.New()
	router.POST("/api/stock/history-tick", func(c *gin.Context) {
		handleStockHistoryTickWithDeps(c, fetchTick, preCloseSource)
	})
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/stock/history-tick",
		bytes.NewBufferString(`{"date":20241213,"market":0,"code":"399001","kind":"index"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "invalid historical preClose") {
		t.Fatalf("response = %s, want invalid historical preClose", resp.Body.String())
	}
}
```

要点：
- **不再 stub `fetchIndexBarsPage`**——生产路径改完后指数 `dailyBars` 会走 `IndexKLineRange`，但 handler 测例应注入 `preCloseSource.dailyBars`，与股票测例一致，避免耦合分页。
- 若 handler 仍覆盖 `src.dailyBars`（旧逻辑），本测例会因注入被覆盖而失败或行为异常——这正是 Task 2 要修的。

- [ ] **Step 2: 把成功测例改成「直接读 bar.PreClose」**

用下面整段替换 `TestStockHistoryTickIndexUsesPreviousTradingDayClose`（可改名为 `TestStockHistoryTickIndexUsesGotdxPreClose`）：

```go
func TestStockHistoryTickIndexUsesGotdxPreClose(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	fetchTick := func(uint32, uint8, string) ([]proto.HistoryMinuteTimeData, error) {
		return []proto.HistoryMinuteTimeData{}, nil
	}
	preCloseSource := timeSharePreCloseSource{
		now: func() time.Time { return time.Date(2026, 7, 27, 10, 0, 0, 0, loc) },
		quote: func(uint8, string) (float64, error) {
			return 0, errors.New("realtime quote must not be called")
		},
		dailyBars: func(market uint8, code string, date time.Time) ([]proto.SecurityBar, error) {
			if market != 0 || code != "399001" || date.Format("20060102") != "20241213" {
				t.Fatalf("daily bar request = %d/%s/%s", market, code, date.Format("20060102"))
			}
			// gotdx 已在 bar 上填好 PreClose；只返回目标日一根
			return []proto.SecurityBar{{
				PreClose:  10957.13,
				LastClose: 10957.13,
				Close:     10713.07,
				Year:      2024,
				Month:     12,
				Day:       13,
				DateTime:  time.Date(2024, 12, 13, 15, 0, 0, 0, loc),
			}}, nil
		},
	}

	router := gin.New()
	router.POST("/api/stock/history-tick", func(c *gin.Context) {
		handleStockHistoryTickWithDeps(c, fetchTick, preCloseSource)
	})
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/stock/history-tick",
		bytes.NewBufferString(`{"date":20241213,"market":0,"code":"399001","kind":"index"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"preClose":10957.13`) {
		t.Fatalf("response = %s, want gotdx PreClose 10957.13", resp.Body.String())
	}
}
```

- [ ] **Step 3: 跑测例，确认在旧实现下行为**

```powershell
go test ./services/tdx-api/internal/api/ -run "TestStockHistoryTickIndex" -count=1
```

**预期（旧实现未改时）：**
- `TestStockHistoryTickIndexUsesGotdxPreClose`：**FAIL** 或 **意外 502**——因为 handler 仍用 `isIndexKLineRequest` 覆盖 `dailyBars`，注入的 `dailyBars` 被丢掉，走 `IndexKLineRange` + 空/默认 `fetchIndexBarsPage`。
- `TestStockHistoryTickIndexRejectsMissingPreCloseOnDailyBar`：同样因覆盖而 **不走注入**，可能 502 文案变成 `previous index daily bar not found` 或别的错��。

若本地 `fetchIndexBarsPage` 仍是生产默认且无 tdx 连接，失败形态可能是 502 + 网络/空数据错误。关键是：**在实现 Task 2 之前，新测例不应全绿**；若误全绿，检查是否忘了删旧名测例或 handler 已改。

- [ ] **Step 4: Commit 测例（允许红）**

```powershell
git add services/tdx-api/internal/api/stock_test.go
git commit -m "test(tdx): expect index history-tick preClose from bar field"
```

---

### Task 2: 删除指数 preClose 推算，统一 dailyBars

**Files:**
- Modify: `services/tdx-api/internal/api/stock.go:292-316`

- [ ] **Step 1: 替换 handler 中指数分支**

将 `handleStockHistoryTickWithDeps` 内：

```go
	src := preCloseSource
	if isIndexKLineRequest(req.Kind, req.Market, req.Code) {
		indexMarket, indexCode := req.Market, req.Code
		src.dailyBars = func(market uint8, code string, date time.Time) ([]proto.SecurityBar, error) {
			// 指数日线协议不含昨收，往前多看几天来推算昨收
			prevDate := date.AddDate(0, 0, -10)
			bars, err := IndexKLineRange(4, indexMarket, indexCode, prevDate, date)
			if err != nil {
				return nil, err
			}
			yy, mm, dd := date.Date()
			for i, b := range bars {
				by, bm, bd := b.DateTime.Date()
				if yy == by && mm == bm && dd == bd {
					if i == 0 {
						return nil, errors.New("previous index daily bar not found")
					}
					b.PreClose = bars[i-1].Close
					b.LastClose = bars[i-1].Close
					return []proto.SecurityBar{b}, nil
				}
			}
			return nil, nil
		}
	}
	preClose, err := resolveTimeSharePreClose(req.Market, req.Code, req.Date, src)
```

改为：

```go
	src := preCloseSource
	// 测例注入 dailyBars 时优先用注入；生产默认 source 无 dailyBars 时按 kind 补全。
	// gotdx 已在 IndexBar/SecurityBar 填 PreClose，不再用前一日 Close 推算。
	if src.dailyBars == nil {
		if isIndexKLineRequest(req.Kind, req.Market, req.Code) {
			indexMarket, indexCode := req.Market, req.Code
			src.dailyBars = func(_ uint8, _ string, date time.Time) ([]proto.SecurityBar, error) {
				return IndexKLineRange(4, indexMarket, indexCode, date, date)
			}
		} else {
			src.dailyBars = newDefaultTimeSharePreCloseSource().dailyBars
		}
	}
	preClose, err := resolveTimeSharePreClose(req.Market, req.Code, req.Date, src)
```

**设计说明（写进实现注释即可，勿扩 scope）：**
- `newDefaultTimeSharePreCloseSource()` 的 `dailyBars` 是 `StockKLineRange`（股票）。指数请求必须换 `IndexKLineRange`，否则 399001 等会走错协议。
- 仅当 `src.dailyBars == nil` 时补全：handler 测例注入的 `dailyBars` 不被覆盖（与 Task 1 一致）。
- `newDefaultTimeSharePreCloseSource()` 总是设置 `dailyBars`，因此生产路径里 `src.dailyBars == nil` **几乎不会**走到 `else` 的股票默认——指数生产路径会因默认 source 已有股票 `dailyBars` 而**仍走股票日线**。

**修正（必须）：** 生产默认 source 已带股票 `dailyBars`，不能只靠 `== nil`。应改为：

```go
	src := preCloseSource
	if isIndexKLineRequest(req.Kind, req.Market, req.Code) {
		// 仅当调用方未注入 dailyBars 时，用指数日线；测例注入则保留。
		// 判断：与 default 股票实现「是否同一函数」不可靠；用显式标志或：
		// 约定 WithDeps 测例总是提供完整 source，生产走 newDefault + 此处覆盖指数。
		indexMarket, indexCode := req.Market, req.Code
		// 若 preCloseSource 来自 newDefault（股票 dailyBars），必须覆盖为 IndexKLineRange。
		// 测例若要自定义，应传入非 nil dailyBars 且我们需要区分「默认股票」vs「测试注入」。
		//
		// 采用：指数路径始终设置 dailyBars 为 IndexKLineRange，
		// 但仅当调用方传入的 dailyBars 为 nil 时设置——因此
		// newDefaultTimeSharePreCloseSource 对指数不能直接用。
		_ = indexMarket
		_ = indexCode
	}
```

**最终选定实现（避免函数指针比较）：**

1. 改 `newDefaultTimeSharePreCloseSource` **不再**预设 `dailyBars`（或拆出 `quote`/`now` 与 `dailyBars` 装配）。
2. 在 handler 内统一装配：

```go
	src := preCloseSource
	if src.now == nil {
		src.now = time.Now
	}
	if src.quote == nil {
		src.quote = newDefaultTimeSharePreCloseSource().quote
	}
	if src.dailyBars == nil {
		if isIndexKLineRequest(req.Kind, req.Market, req.Code) {
			indexMarket, indexCode := req.Market, req.Code
			src.dailyBars = func(_ uint8, _ string, date time.Time) ([]proto.SecurityBar, error) {
				return IndexKLineRange(4, indexMarket, indexCode, date, date)
			}
		} else {
			src.dailyBars = func(market uint8, code string, date time.Time) ([]proto.SecurityBar, error) {
				return StockKLineRange(4, market, code, 1, 0, date, date)
			}
		}
	}
	preClose, err := resolveTimeSharePreClose(req.Market, req.Code, req.Date, src)
```

3. 同步改 `newDefaultTimeSharePreCloseSource`：去掉 `dailyBars` 字段赋值（只留 `now` + `quote`），避免「默认股票 dailyBars 挡住指数覆盖」。

`newDefaultTimeSharePreCloseSource` 改为：

```go
func newDefaultTimeSharePreCloseSource() timeSharePreCloseSource {
	return timeSharePreCloseSource{
		now: time.Now,
		quote: func(market uint8, code string) (float64, error) {
			quotes, err := mainCall(func(c client.MainQuerier) ([]proto.SecurityQuote, error) {
				return c.StockQuotesDetail([]uint8{market}, []string{code})
			})
			if err != nil {
				return 0, err
			}
			if len(quotes) == 0 {
				return 0, errors.New("empty realtime quote response")
			}
			return quotes[0].PreClose, nil
		},
		// dailyBars 由 handleStockHistoryTickWithDeps 按 kind 装配
	}
}
```

`handleStockHistoryTick` 仍：

```go
func handleStockHistoryTick(c *gin.Context) {
	handleStockHistoryTickWithDeps(c, fetchStockHistoryTick, newDefaultTimeSharePreCloseSource())
}
```

- [ ] **Step 2: 跑指数相关测例**

```powershell
go test ./services/tdx-api/internal/api/ -run "TestStockHistoryTickIndex|TestResolveTimeSharePreClose|TestStockHistoryTickResponse" -count=1
```

**Expected:** 全部 PASS。

- [ ] **Step 3: 跑 api 包全量单测**

```powershell
go test ./services/tdx-api/internal/api/ -count=1
```

**Expected:** 全部 PASS（无 integration 依赖；不启动 tdx 服务）。

- [ ] **Step 4: Commit**

```powershell
git add services/tdx-api/internal/api/stock.go services/tdx-api/internal/api/stock_test.go
git commit -m "fix(tdx): read index preClose from gotdx bar instead of prior close"
```

---

### Task 3: 可选冒烟（有本地 tdx 时）

**Files:** none（手工）

- [ ] **Step 1: 确认服务用的是 pin 后的 fork**

```powershell
# 工作目录 D:\Code\KlineChartQuantGo
go list -m github.com/bensema/gotdx
# 期望：含 => github.com/363045841/gotdx v0.0.0-...
```

- [ ] **Step 2: 重启 tdx-api 后请求指数历史分时**

```powershell
# 先停旧 tdx-api 进程，再：
go run . tdx
```

另开终端：

```powershell
curl -s -X POST http://127.0.0.1:8080/api/stock/history-tick `
  -H "Content-Type: application/json" `
  -d '{"date":20241213,"market":0,"code":"399001","kind":"index"}' |
  ConvertFrom-Json |
  Select-Object preClose, @{n='n';e={$_.data.Count}}
```

**Expected:**
- HTTP 200
- `preClose` 为合理指数昨收量级（约 1e4，与当日分时价同量级），**不是** 0
- 不出现 `previous index daily bar not found`

股票路径回归（确认未破坏）：

```powershell
curl -s -X POST http://127.0.0.1:8080/api/stock/history-tick `
  -H "Content-Type: application/json" `
  -d '{"date":20241213,"market":1,"code":"600519"}' |
  ConvertFrom-Json |
  Select-Object preClose
```

**Expected:** `preClose > 0`。

- [ ] **Step 3: 无需再 commit**（无代码变更）；若冒烟失败，回到 Task 2 查 `dailyBars` 是否仍被股票默认挡住。

---

## Self-review

| Spec 点 | Task |
|---------|------|
| 不再用前一日 Close 推指数昨收 | Task 2 删除 `bars[i-1].Close` |
| 直读 gotdx `PreClose`/`LastClose` | Task 2 + 既有 `securityBarPreClose` |
| 测例与股票一致（注入 dailyBars） | Task 1 |
| 生产指数仍走 `IndexKLineRange` 非股票 K 线 | Task 2 `dailyBars == nil` 时按 kind 装配 |
| 扩展/当日 quote 路径不动 | Out of scope，无 task 改 ex.go |
| 与 baseUnit 无关 | Out of scope |

**占位符扫描:** 无 TBD；测例与实现代码已全文给出。

**类型一致:** `timeSharePreCloseSource.dailyBars` 签名保持 `(market uint8, code string, date time.Time) ([]proto.SecurityBar, error)`；指数闭包可忽略 market/code 参数，用请求里的 `indexMarket`/`indexCode`。

---

## 风险与注意

1. **gotdx 指数 bar 的 PreClose 除数是 `/1000` 写死**（`get_index_bars.go`），与股票 raw 一致；若线上某指数 PreClose 量级异常，属 gotdx 解码问题，**本方案不修**，只保证业务层不二次推算。
2. **分页边界：** `IndexKLineRange(date, date)` 只取目标日；gotdx 单页内已用 `Count+1` 算好该根 PreClose，不依赖我们多要历史日。
3. **测例注入约定：** `handleStockHistoryTickWithDeps` 在 `dailyBars != nil` 时绝不覆盖——Task 1 测例必须自己提供完整 `dailyBars`。
