# 反向策略影子层设计方案

> 状态: PLANNED | 日期: 2026-03-20 | 分支: personal-feature

## 问题描述

当反向策略启用时，AI 决策系统会在后续周期中对反向开仓的仓位发出 `close` 指令，因为：

1. AI 不知道反向策略的存在
2. AI 看到一个与自己分析方向相反的仓位（如 AI 看多但持有 short）
3. AI 评估该仓位"不合理"，建议关闭
4. 当前仅靠 `MinHoldMinutes`（120 分钟）的风险控制拦截，保护期过后仓位必被关闭

```
周期1: AI → "open_long BTC" → ReverseTransformer → 执行 "open_short BTC"
周期2: AI 看到 short BTC + 市场看涨 → "close_short BTC" → 透传（不反转）
       → MinHold 120分钟暂时拦截
周期N: 120分钟后 → close_short 执行成功 → 反向仓位被杀死
```

### 根因

- `transformer_reverse.go` 只反转 `open_*` 动作，`close_*` 直接透传（line 67-68）
- AI prompt 中没有反向策略信息，AI 不知道仓位是故意反向开的
- `shouldBlockActiveClose()` 的 120 分钟保护是唯一防线

## 解决方案：影子层反向策略

**核心思路：对 AI 隐藏反向仓位，让 AI 在自己的认知空间中正常运作**

```
┌─────────────────────────────────────────────┐
│              AI 的认知世界                    │
│  • 看到真实的账户权益/保证金                   │
│  • 看不到反向仓位（影子仓位）                  │
│  • 正常做分析和决策                            │
└──────────┬──────────────────────────────────┘
           ↓ AI Decision
┌─────────────────────────────────────────────┐
│         ReverseTransformer 影子层             │
│  open_long  → open_short (反向开仓)          │
│  open_short → open_long  (反向开仓)          │
│  方向翻转检测 → 关闭当前反向仓位               │
│  close_*    → 不会触发(AI 看不到反向仓位)     │
└─────────────────────────────────────────────┘
           ↓
┌─────────────────────────────────────────────┐
│            退出机制 (混合模式)                 │
│  1. 交易所侧 SL/TP 自动触发                   │
│  2. AI 方向翻转信号 → 关闭反向仓位             │
└─────────────────────────────────────────────┘
```

### AI 方向翻转退出逻辑

```
状态: 持有反向 short BTC（AI 原始信号是 open_long）

→ AI 继续说 open_long BTC → 方向未变 → 保持反向 short（跳过重复开仓）
→ AI 说 wait/hold          → 无动作 → 保持反向 short
→ AI 说 open_short BTC     → AI 观点翻转！
   → AI 不再看多 → 反向策略的做空理由消失
   → 关闭反向 short 仓位
   → 然后反向执行 open_short → open_long（开新反向仓位，如果需要）
```

### 账户数据可见性

| 数据 | 对 AI 可见 | 说明 |
|------|-----------|------|
| 账户权益 (equity) | ✅ 真实值 | AI 需要准确的资金信息 |
| 保证金使用率 | ✅ 真实值 | 包含反向仓位的保证金占用 |
| 可用余额 | ✅ 真实值 | AI 看到真实可用资金 |
| 反向仓位详情 | ❌ 隐藏 | AI 不知道反向仓位的存在 |
| 正常仓位详情 | ✅ 可见 | AI 正常管理非反向仓位 |

## 实施计划

### Task 1: 反向仓位追踪机制

**文件**: `trader/auto_trader.go`

在 `AutoTrader` 结构体中新增反向仓位追踪：

```go
// 新增字段
reversePositionKeys   map[string]bool   // key: "BTCUSDT_short", value: true
reversePositionKeysMu sync.RWMutex

// 辅助方法
func (at *AutoTrader) markReversePosition(symbol, side string)
func (at *AutoTrader) clearReversePosition(symbol, side string)
func (at *AutoTrader) isReversePosition(symbol, side string) bool
func (at *AutoTrader) hasReversePositionForSymbol(symbol string) (exists bool, side string)
```

**初始化**：在 `NewAutoTrader()` 中初始化 map。

**持久化考虑**：
- v1 使用内存 map（重启后丢失）
- 重启恢复策略：如果反向策略启用 + 存在仓位，可在启动时标记现有仓位为反向仓位
- v2 可考虑写入数据库 `trader_positions` 表添加 `is_reverse` 字段

### Task 2: AI 上下文过滤

**文件**: `trader/auto_trader_loop.go` - `buildTradingContext()`

在构建 `positionInfos` 循环中（约 line 349-424），添加反向仓位过滤：

```go
for _, pos := range positions {
    symbol := pos["symbol"].(string)
    side := pos["side"].(string)
    // ... 现有逻辑 ...

    // 计算 margin（无论是否反向，账户数据保持真实）
    marginUsed := (quantity * markPrice) / float64(leverage)
    totalMarginUsed += marginUsed

    // 隐藏反向仓位 - 不传递给 AI
    if at.isReversePosition(symbol, side) {
        currentPositionKeys[posKey] = true  // 仍然追踪 key，防止清理
        continue
    }

    // ... 正常仓位继续构建 positionInfos ...
}
```

**关键点**：
- `totalMarginUsed` 仍包含反向仓位的保证金（账户数据真实）
- `positionInfos` 不包含反向仓位（AI 看不到）
- `currentPositionKeys` 仍追踪反向仓位（防止被误清理）

### Task 3: 开仓成功后记录反向标记

**文件**: `trader/auto_trader_orders.go`

在 `executeOpenLongWithRecord()` 和 `executeOpenShortWithRecord()` 中，开仓成功后检查是否为反向决策：

```go
// 开仓成功后
if hasDecisionTransform(decision, "reverse") {
    at.markReversePosition(decision.Symbol, "long")  // 或 "short"
}
```

在 `executeCloseLongWithRecord()` 和 `executeCloseShortWithRecord()` 中，平仓成功后清除标记：

```go
// 平仓成功后
at.clearReversePosition(decision.Symbol, "long")  // 或 "short"
```

### Task 4: 方向翻转检测与执行

**文件**: `trader/auto_trader_loop.go` - 决策执行阶段

在处理经过 ReverseTransformer 转换后的开仓决策时，检测方向翻转：

```go
// 伪代码 - 在执行反向开仓决策前
if hasDecisionTransform(&transformedDecision, "reverse") {
    transformedSide := getSideFromAction(transformedDecision.Action) // "long" or "short"

    // 检查是否存在相同 symbol 的反向仓位（方向相反）
    if exists, existingSide := at.hasReversePositionForSymbol(transformedDecision.Symbol); exists {
        if existingSide != transformedSide {
            // AI 方向翻转！关闭现有反向仓位
            logger.Infof("🔄 [REVERSE] AI direction flipped for %s: closing reverse %s position",
                transformedDecision.Symbol, existingSide)

            // 构建 close 决策并执行
            closeAction := "close_" + existingSide  // "close_long" or "close_short"
            closeDecision := &kernel.Decision{
                Symbol: transformedDecision.Symbol,
                Action: closeAction,
            }
            at.executeDecision(closeDecision, ...)
        } else {
            // 方向未变，跳过重复开仓
            logger.Infof("⏭️ [REVERSE] Skipping duplicate reverse %s for %s",
                transformedSide, transformedDecision.Symbol)
            continue
        }
    }
}
```

**翻转场景示例**：

| AI 原始决策 | 反向后 | 现有反向仓位 | 动作 |
|------------|--------|-------------|------|
| open_long  | open_short | 无 | 正常开反向 short |
| open_long  | open_short | short | 跳过（重复） |
| open_short | open_long  | short | 翻转！关闭 short → 开 long |
| open_long  | open_short | long | 翻转！关闭 long → 开 short |

### Task 5: 重启恢复（可选 v1.1）

**文件**: `trader/auto_trader.go` - 启动逻辑

```go
func (at *AutoTrader) recoverReversePositions() {
    if !at.isReverseStrategyEnabled() {
        return
    }

    positions, err := at.trader.GetPositions()
    if err != nil {
        return
    }

    for _, pos := range positions {
        symbol := pos["symbol"].(string)
        side := pos["side"].(string)
        quantity := pos["positionAmt"].(float64)
        if quantity == 0 { continue }

        // 如果反向策略启用且存在仓位，假设是反向仓位
        // （保守策略：宁可隐藏不该隐藏的，也不暴露反向仓位给 AI）
        at.markReversePosition(symbol, side)
        logger.Infof("🔄 [REVERSE] Recovered reverse position: %s %s", symbol, side)
    }
}
```

### Task 6: 测试

**新增测试文件**: `trader/auto_trader_reverse_test.go`

测试场景：

1. **反向仓位追踪**
   - mark/clear/isReverse 基本操作
   - hasReversePositionForSymbol 查询

2. **AI 上下文过滤**
   - 反向仓位不出现在 positionInfos 中
   - 账户 margin 数据仍包含反向仓位

3. **方向翻转检测**
   - AI 方向未变 → 跳过重复开仓
   - AI 方向翻转 → 关闭旧仓位 + 开新仓位

4. **混合仓位场景**
   - 同时存在正常仓位和反向仓位
   - AI 正常管理正常仓位，不影响反向仓位

## 风险评估

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 反向仓位永远不被关闭 | 高 | 混合退出：SL/TP + AI翻转信号双重保障 |
| 重启后丢失反向标记 | 中 | v1.1 恢复逻辑：启动时假设现有仓位为反向 |
| 最大持仓数限制误触发 | 低 | `enforceMaxPositions` 在执行层检查，反向仓位占用会被正确计算 |
| AI 看到资金不足但无仓位 | 低 | 仅隐藏仓位详情，保证金数据真实，AI 会自然降低开仓意愿 |
| 多 symbol 反向仓位并发 | 低 | 使用 mutex 保护 map 并发安全 |

## 文件变更清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `trader/auto_trader.go` | 修改 | 新增 reversePositionKeys 字段和方法 |
| `trader/auto_trader_loop.go` | 修改 | buildTradingContext 过滤 + 翻转检测 |
| `trader/auto_trader_orders.go` | 修改 | 开仓/平仓时 mark/clear 反向标记 |
| `trader/decision_pipeline.go` | 可能修改 | 如需扩展翻转检测逻辑 |
| `trader/auto_trader_reverse_test.go` | 新增 | 反向策略影子层测试 |

## 不需要修改的文件

| 文件 | 原因 |
|------|------|
| `kernel/transformer_reverse.go` | Transform 逻辑不变，仍只反转 open_* |
| `kernel/prompt_builder.go` | 不需要向 AI 注入反向策略说明 |
| `kernel/formatter.go` | 过滤在 trader 层完成，formatter 无需改动 |
| `store/strategy.go` | ReverseStrategyConfig 无需新增字段（v1） |
| 前端文件 | 无 UI 变更 |
