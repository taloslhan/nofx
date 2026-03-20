# OI/NetFlow Prompt 动态周期适配与解读公式修复计划

> 日期: 2026-03-20
> 分支: personal-feature
> 状态: 已执行

## 背景

OI 排行和资金流向的 duration 已从 `1h` 改为 `24h`（用户可配置），但代码中多处硬编码了"1小时"/"1 hour"的描述，会误导 AI 对数据周期的理解。同时，OI 和 NetFlow 的解读公式存在逻辑不完整、自相矛盾的问题。

## 核对结论

- 原计划主体准确，列出的文案误导、解读公式不完整、散户展示信息不对称等问题均在代码中存在。
- 原计划第 10 项需要扩展：不仅 `engine.go:275` 的 `OITopDataMap` 初始化仍走 legacy `1h` 接口，`getOITopCoins()` 与 `getOILowCoins()` 也同样走硬编码 `1h` 路径；执行时已一并修复。
- 执行策略采用“保留 legacy 包装函数、补充带 duration 的新方法、让主流程全部改走配置周期”的方式，避免破坏兼容性。

---

## 改动清单

### 1. [Critical] schema.go — OIChange 描述硬编码"1小时"

**文件**: `kernel/schema.go:214-215`

**现状**:
```go
"OIChange": {
    DescZH: "1小时内持仓量的变化。用于判断市场真实资金流向",
    DescEN: "OI change in 1 hour. Used to determine real capital flow direction",
},
```

**问题**: duration 已可配置为 1h/4h/24h，描述写死"1小时"会误导 AI。

**方案**: DataDictionary 是静态 map，不适合动态描述。改为去掉具体时间，使用通用描述：
```go
"OIChange": {
    DescZH: "指定周期内持仓量的变化（具体周期见数据标题）。用于判断市场真实资金流向",
    DescEN: "OI change within the specified period (see data header for duration). Used to determine real capital flow direction",
},
```

---

### 2. [Critical] schema.go — OIChangeThreshold 硬编码"1小时"

**文件**: `kernel/schema.go:297-298`

**现状**:
```go
"OIChangeThreshold": {
    Value:    0.02,
    DescZH:   "持仓量1小时内变化超过2%视为显著变化",
    DescEN:   "OI change >2% in 1 hour is considered significant",
},
```

**问题**: 同上，周期描述硬编码。且 2% 阈值在 24h 周期下意义完全不同（24h 内 2% 变化很正常，1h 内 2% 才算显著）。

**方案**: 去掉具体时间，并补充说明阈值随周期调整：
```go
"OIChangeThreshold": {
    Value:    0.02,
    DescZH:   "持仓量在观察周期内变化超过2%视为显著变化（该阈值基于1h周期，更长周期应适当放宽）",
    DescEN:   "OI change >2% in the observation period is significant (threshold based on 1h, adjust upward for longer periods)",
},
```

---

### 3. [Critical] engine.go — OITopData 注释硬编码"1 hour"

**文件**: `kernel/engine.go:77`

**现状**:
```go
OIDeltaPercent    float64 // Open interest change percentage (1 hour)
```

**方案**: 改为通用描述：
```go
OIDeltaPercent    float64 // Open interest change percentage (period depends on config)
```

---

### 4. [Critical] oi.go — OI 排行解读公式不完整

**文件**: `provider/nofxos/oi.go:200`

**现状**:
```go
sb.WriteString("**解读**: OI增+价涨=多头主导 | OI增+价跌=空头主导 | OI减+价涨=空头平仓 | OI减+价跌=多头平仓\n\n")
```

**问题**: 公式本身四种情况是完整的，但缺少"信号强度"和"操作建议"的维度。AI 看到这个公式后无法判断哪种情况更值得关注。

**方案**: 增强解读，加入信号强度和操作暗示：
```go
sb.WriteString("**解读**:\n")
sb.WriteString("- OI增+价涨=多头主导（新多开仓，趋势延续信号，可顺势做多）\n")
sb.WriteString("- OI增+价跌=空头主导（新空开仓，下跌趋势信号，谨慎做多）\n")
sb.WriteString("- OI减+价涨=空头平仓（非真实买盘，上涨可能不持续）\n")
sb.WriteString("- OI减+价跌=多头平仓（恐慌性平仓，可能接近底部）\n")
```

同样修改英文版 `oi.go:235`。

---

### 5. [Critical] netflow.go — 资金流向解读公式不完整

**文件**: `provider/nofxos/netflow.go:194`

**现状**:
```go
sb.WriteString("**解读**: 机构买入+散户卖出=强烈看多 | 机构卖出+散户买入=强烈看空\n\n")
```

**问题**: 只覆盖了 2/4 种组合，缺少"机构买入+散户买入"和"机构卖出+散户卖出"的解读。实际数据中经常出现这两种情况（如 BTCUSDT 两方同时买入）。

**方案**: 补全四种组合：
```go
sb.WriteString("**解读**:\n")
sb.WriteString("- 机构买入+散户卖出=强烈看多（Smart Money 吸筹，散户恐慌抛售）\n")
sb.WriteString("- 机构卖出+散户买入=强烈看空（Smart Money 出货，散户接盘）\n")
sb.WriteString("- 机构买入+散户买入=市场共识看多（但需警惕过热）\n")
sb.WriteString("- 机构卖出+散户卖出=市场共识看空（恐慌蔓延，可能接近超卖）\n")
```

同样修改英文版 `netflow.go:261`。

---

### 6. [High] netflow.go — 散户数据只展示 Top 3，信息不对称

**文件**: `provider/nofxos/netflow.go:168` (ZH), `netflow.go:234` (EN)

**现状**: 机构展示完整 Top 10 表格，散户只展示 3 个币种的一行摘要。

**方案**: 将散户数据也改为表格格式，展示 Top 5（不需要和机构一样多，但至少是结构化的）：
```go
// 散户资金流入
if len(data.PersonalFutureTop) > 0 {
    sb.WriteString("### 散户资金流入\n\n")
    sb.WriteString("| 排名 | 币种 | 流入金额(USDT) | 价格 |\n")
    sb.WriteString("|------|------|----------------|------|\n")
    for i, pos := range data.PersonalFutureTop {
        if i >= 5 { break }
        sb.WriteString(fmt.Sprintf("| %d | %s | %s | $%.4f |\n",
            pos.Rank, pos.Symbol, formatValue(pos.Amount), pos.Price))
    }
    sb.WriteString("\n")
}
// 散户资金流出同理
```

---

### 7. [High] formatter.go — 候选币种 OI 数据无周期标注

**文件**: `kernel/formatter.go:292-297` (ZH), `kernel/formatter.go:555-560` (EN)

**现状**:
```go
sb.WriteString(fmt.Sprintf("**持仓量变化**: OI排名 #%d | 变化 %+.2f%% (%+.2fM USDT) | 价格变化 %+.2f%%\n\n", ...))
```

**问题**: 没有标注这是哪个周期的 OI 变化，AI 无法准确判断。

**方案**: 需要将 duration 信息传递到 Context 或 OITopData 中。两种选择：

**方案 A（推荐）**: 在 `OITopData` struct 中增加 `Duration` 字段：
```go
type OITopData struct {
    Rank              int
    OIDeltaPercent    float64
    OIDeltaValue      float64
    PriceDeltaPercent float64
    Duration          string  // "1h", "4h", "24h"
}
```

然后在 formatter 中使用：
```go
sb.WriteString(fmt.Sprintf("**持仓量变化(%s)**: OI排名 #%d | ...", oiData.Duration, ...))
```

**方案 B**: 在 Context 中增加 `OIDuration` 字段，formatter 从 ctx 读取。

---

### 8. [High] coin.go — QuantData OI Delta 硬编码取 "1h" key

**文件**: `provider/nofxos/coin.go:144` (ZH), `coin.go:194` (EN)

**现状**:
```go
if delta, ok := oiData.Delta["1h"]; ok && delta != nil {
    sb.WriteString(fmt.Sprintf("- 1h变化: %s (%.2f%%)\n", ...))
}
```

**问题**: QuantData 的 OI delta 硬编码只取 "1h" key。如果用户配置了 24h，这里仍然显示 1h 数据（如果 API 返回了的话），或者什么都不显示。

**方案**: 改为遍历所有可用的 duration，或优先取配置的 duration：
```go
// 按优先级显示可用的 OI delta
durations := []string{"24h", "4h", "1h"}
for _, d := range durations {
    if delta, ok := oiData.Delta[d]; ok && delta != nil {
        sb.WriteString(fmt.Sprintf("- %s变化: %s (%.2f%%)\n", d, formatValue(delta.OIDeltaValue), delta.OIDeltaPercent))
    }
}
```

---

### 9. [Medium] prompt_builder.go — 示例中硬编码"1小时"

**文件**: `kernel/prompt_builder.go:171` (ZH), `prompt_builder.go:306` (EN)

**现状**: 决策示例中写死了"持仓量1小时内增加"。

**问题**: 示例会引导 AI 认为 OI 数据总是 1h 周期的。

**方案**: 将示例改为通用表述：
```
ZH: "...持仓量在观察周期内增加+1.57M (+0.89%)..."
EN: "...OI increased +1.57M (+0.89%) in the observation period..."
```

---

### 10. [Medium] oi.go — Legacy 函数硬编码 "1h"

**文件**: `provider/nofxos/oi.go:109`, `oi.go:134`

**现状**:
```go
func (c *Client) GetOITopPositions() ([]OIPosition, error) {
    positions, _, err := c.fetchOIRanking("top", "1h", 20)
```

**问题**: 遗留函数硬编码 1h。虽然主流程不走这里，但 `engine.go:275` 的 OITopDataMap 初始化仍在调用 `GetOITopPositions()`。

**方案**: 废弃这些 legacy 函数，或改为接受 duration 参数。`engine.go:275` 处应改用配置的 duration：
```go
// engine.go:275 - 使用配置的 duration 而非 legacy 函数
duration := engine.config.Indicators.OIRankingDuration
if duration == "" { duration = "1h" }
oiData, err := engine.nofxosClient.GetOIRanking(duration, 20)
if err == nil {
    for _, pos := range oiData.TopPositions {
        ctx.OITopDataMap[pos.Symbol] = &OITopData{...}
    }
}
```

---

### 11. [Low] netflow.go / oi.go — "Smart Money"/"信号"措辞过于绝对

**文件**:
- `provider/nofxos/netflow.go:139,152,206,219`
- `provider/nofxos/oi.go:176,211`

**现状**: "Smart Money买入信号"、"资金流入，趋势延续或新仓建立信号"

**方案**: 降低语气确定性，改为参考性描述：
```
"机构资金流入（仅供参考，非确定性信号）"
"资金流入趋势，可能表示趋势延续或新仓建立"
```

---

## 执行顺序

```
Phase 1 (核心修复 - 消除误导):
  1. schema.go — OIChange 描述 (#1)
  2. schema.go — OIChangeThreshold (#2)
  3. engine.go — OITopData 注释 (#3)
  4. engine.go:275 — OITopDataMap 改用配置 duration (#10)

Phase 2 (解读公式完善):
  5. oi.go — OI 解读公式增强 (#4)
  6. netflow.go — NetFlow 解读公式补全 (#5)

Phase 3 (数据展示优化):
  7. formatter.go — 候选币种 OI 加周期标注 (#7)
  8. coin.go — QuantData OI delta 动态取 key (#8)
  9. netflow.go — 散户数据表格化 (#6)

Phase 4 (措辞和示例):
  10. prompt_builder.go — 示例通用化 (#9)
  11. netflow.go/oi.go — 措辞降温 (#11)
```

## 影响范围

- **prompt 输出变化**: AI 接收到的 prompt 内容会更准确、更完整
- **无 API 变更**: 不涉及 API 接口改动
- **无数据库变更**: 不涉及数据结构改动
- **向后兼容**: 所有改动对现有配置向后兼容
- **需要测试**: `kernel/prompt_builder_test.go` 中的 snapshot 可能需要更新
