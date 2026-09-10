# 任务书：计价支持模型 ID 家族回退（pro/flash）+ `dscli models --refresh`

本文档由架构师编写，记录本任务的需求与既定决策。

## 背景与已定案决策（不要自由发挥这些点）

DeepSeek 定价页（https://api-docs.deepseek.com/zh-cn/quick_start/pricing）只有精确列 ID（deepseek-v4-flash / deepseek-v4-pro / deepseek-v4-flash-vision-exp），但实际使用的模型 ID 经常是临时/别名形式（`deepseek-flash`、`deepseek-v4.1-flash-expires-on-0910`），导致 `dscli models` 价格空白、`dscli chat` 成本显示 0。

维护者已定案（按此实现，勿改语义）：
1. **在"查询层"做家族回退**（不是解析层，不加新解析逻辑，页面布局解析完全不动）。
2. **pro 优先**：ID 同时含 flash 和 pro 时优先取 pro 家族价；pro 家族不可用（无列或不一致）且 ID 也含 flash 时回退 flash。
3. **不做 2026-09-14 特判**（那是 V4 Pro 下线路由公告，本次不处理；单含 "pro" 的 ID 在 pro 家族不可用时就是"无价/空白"）。
4. 新增 `--refresh` 标志。
5. 内置兜底快照更新为调价后 flash 价（公告：flash 系列 2026-09-10 12:00 起，空闲 0.02/1/4，高峰 2 倍）。

公告原文（供代码注释引用）："我们将于北京时间 2026 年 9 月 10 日 12:00 起，调整 flash 系列定价：空闲时段输入缓存命中单价 0.02 元、输入缓存未命中单价 1 元、输出单价 4 元；高峰时段价格为空闲时段价格的 2 倍。"

工作方式：小步执行，每次工具调用只做一件事；若中途被打断，从下一个小步继续。

## 需要实现的内容（按文件）

### 1. internal/price/price.go

**A. 新增导出函数 `GetPriceFor` 与两个内部 helper**

```go
// GetPriceFor returns the effective price for model at the current time.
// It prefers the exact pricing-table column for the model ID and falls back
// to the price of the model's family when there is no exact column: a model
// ID containing "pro" or "flash" (case-insensitive) uses that family's
// price, provided every model of the family agrees. Families are tried pro
// first; flash is used only when the ID also contains it and pro yields no
// reliable price. It reports false when neither lookup produces a price.
func GetPriceFor(model string) (Price, bool)

// lookupPrice resolves model against a resolved price map (see resolve):
// exact match first, then the family fallback described on GetPriceFor.
func lookupPrice(prices map[string]Price, model string) (Price, bool)

// familyPrice returns the price shared by every entry of prices whose
// lowercased model ID contains family, reporting false unless at least one
// entry matches and all of them agree - a divergent family yields no price,
// because a wrong price is worse than a missing one.
func familyPrice(prices map[string]Price, family string) (Price, bool)
```

语义细则（逐条锁定）：
- 精确匹配 = 现有大小写敏感、原样字符串匹配，**优先于一切家族回退**（即使家族不一致，精确列仍返回）。
- 家族匹配基于 `getPrice(time.Now())` 的解析结果 map（即 `resolve` 输出，只含当前计价格下有效的列）；**不要**去动 `resolve/priceFor/GetPrice` 现有实现与语义。
- 家族词判断：`strings.Contains(strings.ToLower(model), family)`；family 顺序 `[]string{"pro", "flash"}`，先 pro 后 flash，取第一个能给出"可靠价"的家族。
- "可靠价" = map 中所有（小写后）含该词的条目数量 ≥1 且**全部 Price 完全相等**（三字段结构体比较；不一致则该家族不参与）。familyPrice 草案：

```go
func familyPrice(prices map[string]Price, family string) (Price, bool) {
	var (
		price Price
		found bool
	)
	for m, p := range prices {
		if !strings.Contains(strings.ToLower(m), family) {
			continue
		}
		if found && p != price {
			return Price{}, false
		}
		price, found = p, true
	}
	return price, found
}
```

- `GetPriceFor(model)` = `lookupPrice(getPrice(time.Now()), model)`。
- 行为用例表（写测试照此）：

| 输入（以上页面三列一致为前提） | 结果 |
|---|---|
| `deepseek-v4-flash` / `deepseek-v4-pro` / `deepseek-v4-flash-vision-exp` | 各自精确列价 |
| `deepseek-flash` | flash 家族价 |
| `deepseek-v4.1-flash-expires-on-0910` | flash 家族价 |
| `DeepSeek-Flash`（任意大小写） | flash 家族价 |
| `deepseek-pro-0813`（单含 pro） | pro 家族价 |
| `deepseek-flash-pro`（双词，两家族均可用） | **pro 家族价（pro 优先）** |
| `deepseek-flash-pro`（双词，pro 家族无列或不一致） | flash 家族价 |
| `deepseek-pro`（单含 pro，pro 家族无列） | 无价 false |
| `deepseek-chat`（无家族词） | 无价 false |
| flash 家族不一致（如 vision-exp 价格漂移） | `deepseek-flash` → false；但 `deepseek-v4-flash` 精确仍返回值 |

**B. `ForceRefresh` + 小重构**

```go
// ForceRefresh fetches the pricing page immediately, ignoring the daily TTL
// and the hourly failure backoff, and replaces the cached prices on success.
// A failed fetch keeps the existing cache and returns the error. The attempt
// is recorded so the automatic path keeps its backoff behavior.
func ForceRefresh() error
```

- 内部把现有 `refresh` 拆出 `doFetch(now time.Time) error`（设置 lastFetch=now、fetchPage、失败返回 err、成功 `c.FetchedAt=now` + `theCache=c` + `saveCacheFile(c)`；调用方需已持有 `theCacheMu`），`refresh(now)` 保留退避检查后调 `doFetch` 并忽略错误；`ForceRefresh` 自己加锁后直接调 `doFetch(time.Now())`。现有 `refresh` 的外部行为（含失败时也更新 lastFetch、1 小时退避）不得改变。

**C. 内置兜底快照更新（builtinCache）**

- `flashNew` 改为：
  - OffPeak: `Price{PromptCacheHit: 0.02, PromptCacheMiss: 1, Completion: 4}`
  - Peak: `Price{PromptCacheHit: 0.04, PromptCacheMiss: 2, Completion: 8}`
- `deepseek-v4-flash-vision-exp` 共用 `flashNew`，自动跟随，不用单独改。
- `proNew` 与 `Current` map **保持原值不动**。
- 更新函数/变量注释：说明 flash 数值反映 2026-09-10 12:00 生效的公告调价（页面自 2026-08 起为峰谷定价）。

### 2. internal/price/usage.go

`Usage.Cost` 改用 `GetPriceFor`：

```go
p, ok := GetPriceFor(model)
if !ok {
	return 0
}
```

其余计算不变；`GetCost` 不用动（它委托 `Usage.Cost`）。

### 3. models.go

- 包级变量加 `var modelsRefresh bool`（挨着 `modelsFormat`）。
- init 里加 flag：`modelsCmd.Flags().BoolVar(&modelsRefresh, "refresh", false, "force refresh the price cache (ignore the 24h TTL)")`。
- `ModelsRun` 里 `DeepseekClient.Models` 成功后、构造 rows 前：

```go
if modelsRefresh {
	if err := price.ForceRefresh(); err != nil {
		fmt.Fprintf(os.Stderr, "pricing refresh failed: %v (using cached prices)\n", err)
	}
}
```

- 删掉 `prices := price.GetPrice()`，循环内改为 `if p, ok := price.GetPriceFor(m.ID); ok { ... }`。

### 4. 测试（internal/price/）

新增（都自己做隔离：临时 cachePath + 覆写 fetchPage + resetPriceState + t.Cleanup 复原；不得读写真实 `~/.dscli/price.json`）：

1. `TestLookupPriceFamilyFallback`：纯函数表驱动，覆盖上面用例表的每一行（含大小写、双词 pro 优先、双词 pro 家族缺失→flash、家族不一致、空 map、无家族词）。
2. `TestGetPriceFor`：用 `setTestPrices` 注入 `{"deepseek-v4-flash": {0.02, 1, 4}}`，断言 `GetPriceFor("deepseek-flash")` 命中家族价、`GetPriceFor("deepseek-chat")` 为 false。
3. `TestForceRefresh`（单一流程多断言）：
   - 装一个"新鲜"的内存缓存（FetchedAt=now，旧值 OLD）+ `lastFetch=now`（退避活跃）；`fetchPage` 计数并返回新值 NEW；调 `ForceRefresh()` 无错、抓取次数 +1、`getPrice(time.Now())` 已是 NEW（同时证明绕过 TTL 与退避）、`readCacheFile()!=nil`（磁盘已写）。
   - `fetchPage` 改为返回 error：`ForceRefresh()` 返回非 nil；旧缓存值仍可服务（"失败保留缓存"）；`lastFetch` 已被刷新（可用"把 theCache.FetchedAt 改成过期后调 getPrice，1 小时内不重抓"来断言）。
4. usage_test.go 新增 `TestGetCostFamilyFallback`：注入 `{"deepseek-v4-flash": {0.02, 1, 4}}`，`theUsage` 设定后 `GetCost("deepseek-flash")` 等于按 flash 价算出的成本（证明 Cost 链路已走家族回退）。

修改（builtin 数值变化的连带影响，机械更新期望值）：
- `TestResolvePrices`：`flashOff` → `{0.02, 1, 4}`、`flashPeak` → `{0.04, 2, 8}`（vision 别名变量自动跟随）；pro 与 pre-8-17 的 flashOld/proOld 不变。
- `TestGetPriceCaching`：注释"谷价 0.05"→"0.02"；三处 `0.05` 断言 → `0.02`（302 行那处 pre-effective 的 `0.02` 本来就不变，别改错）。
- `TestGetPriceFallbackBuiltin`：两处 `0.10` → `0.04`。
- 页面解析类测试（TestParseNewPrices* / TestParsePrice）保持原 fixture 不动（它们是历史快照回归）。

### 5. 文档

- `AGENTS.md` 第 75 行 `internal/price/` 行改为（保持一行表格格式）：
  `| `internal/price/` | Token usage tracking & cost calculation; time-aware pricing (peak/off-peak after 2026-08-17, daily cache in ~/.dscli/price.json); unknown model IDs fall back to their pro/flash family price (`GetPriceFor`, all family columns must agree); `dscli models --refresh` forces a refetch |`
- `README.md` 第 8 节 "View Models and Balance" 代码块中、`dscli models` 行后加两行：

  ```bash
  # Force refresh cached prices (ignores the 24h cache)
  dscli models --refresh
  ```

- `README-zh.md` 对应小节（252 行附近）同样加中文版本：

  ```bash
  # 强制刷新价格缓存（忽略 24 小时缓存）
  dscli models --refresh
  ```

## 约束

- 不改 `resolve/priceFor/GetPrice` 的现有语义（唯一例外是 builtinCache 的 flash 数值）。
- 不加 9/14 特判、不加其它模型名特殊规则。
- 不碰 `internal/dsc`、`chat.go`（成本显示经 `Usage.Cost` 已覆盖）。
- 提交前：`make gofmt`、`make fmt-check`、`go test ./...` 全绿；`make build`。
- 手动抽查（本机有真实配置，可直接跑）：
  - `build/dscli models`（`deepseek-flash` 不应再空白；`deepseek-v4-pro` 精确列价）
  - `build/dscli models --refresh`（触发抓取，`~/.dscli/price.json` 的 `fetched_at` 应更新；这是正常缓存行为）
  - `build/dscli models --format json` 正常
  - 注意：定价页在 12:00（北京时间）前仍显示 flash 旧价 0.05/1.5/4.5，值本身以页面为准、无需纠结；本任务的验收点是"家族回退生效 + --refresh 触发抓取"，不是具体数值。
- **不要** `make install`、**不要** push、不要碰 `sqlite.db`/`dscli.env`。
- 单个 commit（含代码、测试、AGENTS.md、README 改动、以及本任务文档 docs/task-price-fallback.md），英文，建议：
  `feat(price): add pro/flash family fallback and models --refresh`
  （正文可补一句说明内置快照同步 2026-09-10 flash 调价）。工作树返回时必须干净。

## 报告要求

完成后报告：实现了什么、测试命令与结果（go test ./... 输出摘要）、手动验证证据（models / --refresh 输出要点、price.json fetched_at 变化）、commit hash、遗留问题（若有）。
