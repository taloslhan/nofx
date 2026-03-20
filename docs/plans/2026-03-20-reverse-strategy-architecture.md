# AI 反向开仓策略 - 系统架构设计报告

> 系统架构师视角 | 2026-03-20
> 核心前提: AI策略方向错误率约90%, 按AI建议反向开仓

---

## 1. 架构现状分析

### 当前数据流

```
kernel.GetFullDecisionWithStrategy(ctx, mcpClient, strategyEngine, variant)
  |
  v
AI API Call -> parseFullDecisionResponse() -> FullDecision{Decisions: []Decision}
  |
  v
trader.runCycle()
  |
  v
sortDecisionsByPriority(decisions)  // close first, then open
  |
  v
executeDecisionWithRecord(&decision, &actionRecord)
  |
  +-- executeOpenLongWithRecord()   // open_long
  +-- executeOpenShortWithRecord()  // open_short
  +-- executeCloseLongWithRecord()  // close_long
  +-- executeCloseShortWithRecord() // close_short
```

### 关键数据结构

```go
// kernel/engine.go
type Decision struct {
    Symbol          string  `json:"symbol"`
    Action          string  `json:"action"`           // open_long, open_short, close_long, close_short
    Leverage        int     `json:"leverage,omitempty"`
    PositionSizeUSD float64 `json:"position_size_usd,omitempty"`
    StopLoss        float64 `json:"stop_loss,omitempty"`
    TakeProfit      float64 `json:"take_profit,omitempty"`
    Confidence      int     `json:"confidence,omitempty"`
    Reasoning       string  `json:"reasoning"`
}
```

### 关键观察

1. **Decision 是纯值对象**: 结构体无方法, 纯数据传递, 易于变换
2. **解析与执行解耦**: `parseFullDecisionResponse()` 返回 `[]Decision`, `executeDecisionWithRecord()` 消费 `Decision`, 中间有清晰的接缝
3. **SL/TP 是绝对价格**: stop_loss 和 take_profit 都是绝对价格值, 反转时需要互换
4. **风控在执行层**: `enforceMaxPositions`, `enforceMinPositionSize` 等在 trader 层, 反转后的决策仍会经过完整风控

---

## 2. 注入点方案对比

### 方案A: kernel层 - parseFullDecisionResponse() 后立即反转

**位置**: `kernel/engine.go` 第327行, `decision` 返回前

```
parseFullDecisionResponse() -> [反转] -> return decision
```

| 维度 | 评价 |
|------|------|
| 侵入性 | **高** - 修改核心解析函数, 影响所有使用者 |
| 可测试性 | **中** - 反转逻辑与解析耦合, 需要完整AI响应来测试 |
| 可配置性 | **低** - 需要将StrategyConfig传入kernel层, 破坏层级边界 |
| 可观测性 | **低** - 原始决策被覆盖, 无法对比 |
| 审计追溯 | **差** - 丢失原始AI意图 |

**结论**: 不推荐。违反单一职责原则, 且丢失审计能力。

### 方案B: trader层 - runCycle() 中执行前反转

**位置**: `trader/auto_trader_loop.go` 第209行, `sortDecisionsByPriority` 之后

```
sortDecisionsByPriority() -> [反转] -> executeDecisionWithRecord()
```

| 维度 | 评价 |
|------|------|
| 侵入性 | **中** - 修改runCycle, 但集中在一个位置 |
| 可测试性 | **中** - 反转函数可独立测试, 但集成测试需要完整trader |
| 可配置性 | **高** - trader已持有strategyConfig引用 |
| 可观测性 | **中** - 可以在反转前后都记录日志 |
| 审计追溯 | **中** - 需手动保存原始决策 |

**结论**: 可行但不够优雅。反转逻辑嵌入执行流程, 未来扩展性受限。

### 方案C: 新增 DecisionTransformer 管道 (推荐)

**位置**: 在 `kernel/engine.go` 返回和 `trader/auto_trader_loop.go` 消费之间, 新建变换层

```
kernel返回 FullDecision
    |
    v
DecisionTransformer Pipeline (新增)
    |-- ReverseTransformer     (反向)
    |-- ConfidenceFilter       (过滤低置信度, 未来扩展)
    |-- PositionAdjuster       (仓位调整, 未来扩展)
    |
    v
trader消费 TransformedDecision (包含原始+变换后)
```

| 维度 | 评价 |
|------|------|
| 侵入性 | **低** - 新增文件, 现有代码改动最小 (runCycle仅增加一行调用) |
| 可测试性 | **高** - 纯函数变换, 输入Decision输出Decision, 完全可单测 |
| 可配置性 | **高** - 通过StrategyConfig驱动, 支持热切换 |
| 可观测性 | **高** - TransformedDecision同时保留原始和变换后决策 |
| 审计追溯 | **优** - 原始决策完整保留, 自动对比 |
| 扩展性 | **优** - Pipeline模式, 未来可插入任意变换器 |

**结论**: 强烈推荐。最小侵入, 最高可扩展性, 完美支持审计对比。

---

## 3. 推荐方案: DecisionTransformer Pipeline

### 3.1 核心接口设计

```go
// kernel/transformer.go (新文件)
package kernel

// DecisionTransformer 决策变换器接口
type DecisionTransformer interface {
    // Transform 变换单个决策, 返回变换后的决策
    // 如果返回nil, 表示该决策应被过滤掉
    Transform(original Decision) *Decision
    // Name 变换器名称 (用于日志和审计)
    Name() string
}

// TransformedFullDecision 包含原始和变换后的完整决策
type TransformedFullDecision struct {
    Original    *FullDecision  // 原始AI决策 (完整保留)
    Transformed *FullDecision  // 变换后决策 (实际执行)
    Transforms  []string       // 应用的变换器名称列表
}

// DecisionPipeline 决策变换管道
type DecisionPipeline struct {
    transformers []DecisionTransformer
}

// NewDecisionPipeline 创建空管道
func NewDecisionPipeline() *DecisionPipeline {
    return &DecisionPipeline{}
}

// Add 添加变换器到管道
func (p *DecisionPipeline) Add(t DecisionTransformer) *DecisionPipeline {
    p.transformers = append(p.transformers, t)
    return p
}

// Apply 依次应用所有变换器
func (p *DecisionPipeline) Apply(original *FullDecision) *TransformedFullDecision {
    if len(p.transformers) == 0 {
        return &TransformedFullDecision{
            Original:    original,
            Transformed: original,
        }
    }

    result := &TransformedFullDecision{
        Original: original,
    }

    // 深拷贝decisions用于变换
    transformed := make([]Decision, 0, len(original.Decisions))
    for _, d := range original.Decisions {
        current := d // 值拷贝
        for _, t := range p.transformers {
            out := t.Transform(current)
            if out == nil {
                goto skip // 被过滤
            }
            current = *out
            result.Transforms = append(result.Transforms, t.Name())
        }
        transformed = append(transformed, current)
    skip:
    }

    // 构造变换后的FullDecision
    result.Transformed = &FullDecision{
        SystemPrompt:        original.SystemPrompt,
        UserPrompt:          original.UserPrompt,
        CoTTrace:            original.CoTTrace,
        RawResponse:         original.RawResponse,
        Timestamp:           original.Timestamp,
        AIRequestDurationMs: original.AIRequestDurationMs,
        Decisions:           transformed,
    }

    return result
}
```

### 3.2 反向变换器实现

```go
// kernel/transformer_reverse.go (新文件)
package kernel

// ReverseTransformer 反向决策变换器
type ReverseTransformer struct {
    config ReverseConfig
}

// ReverseConfig 反向策略配置
type ReverseConfig struct {
    Enabled         bool `json:"enabled"`          // 是否启用反向
    ReverseOpen     bool `json:"reverse_open"`     // 反转开仓 (open_long <-> open_short)
    ReverseClose    bool `json:"reverse_close"`    // 反转平仓 (close_long <-> close_short)
    SwapSLTP        bool `json:"swap_sl_tp"`       // 互换止损/止盈价格
    AdjustLeverage  bool `json:"adjust_leverage"`  // 是否调整杠杆 (反向可能需要不同杠杆)
    LeverageScale   float64 `json:"leverage_scale"`   // 杠杆缩放因子 (1.0=不变)
    PositionScale   float64 `json:"position_scale"`   // 仓位缩放因子 (1.0=不变)
}

func NewReverseTransformer(config ReverseConfig) *ReverseTransformer {
    // 设置默认值
    if config.LeverageScale == 0 {
        config.LeverageScale = 1.0
    }
    if config.PositionScale == 0 {
        config.PositionScale = 1.0
    }
    return &ReverseTransformer{config: config}
}

func (t *ReverseTransformer) Name() string { return "reverse" }

func (t *ReverseTransformer) Transform(d Decision) *Decision {
    if !t.config.Enabled {
        return &d
    }

    reversed := d // 值拷贝

    // 反转Action
    switch d.Action {
    case "open_long":
        if t.config.ReverseOpen {
            reversed.Action = "open_short"
        }
    case "open_short":
        if t.config.ReverseOpen {
            reversed.Action = "open_long"
        }
    case "close_long":
        if t.config.ReverseClose {
            reversed.Action = "close_short"
        }
    case "close_short":
        if t.config.ReverseClose {
            reversed.Action = "close_long"
        }
    case "hold", "wait":
        return &reversed // 不变换
    }

    // 互换SL/TP (开仓时)
    if t.config.SwapSLTP && (d.Action == "open_long" || d.Action == "open_short") {
        reversed.StopLoss, reversed.TakeProfit = d.TakeProfit, d.StopLoss
    }

    // 缩放杠杆
    if t.config.AdjustLeverage && d.Leverage > 0 {
        reversed.Leverage = int(float64(d.Leverage) * t.config.LeverageScale)
        if reversed.Leverage < 1 {
            reversed.Leverage = 1
        }
    }

    // 缩放仓位
    if t.config.PositionScale != 1.0 && d.PositionSizeUSD > 0 {
        reversed.PositionSizeUSD = d.PositionSizeUSD * t.config.PositionScale
    }

    // 标注反转原因
    reversed.Reasoning = "[REVERSED] " + d.Reasoning

    return &reversed
}
```

### 3.3 SL/TP 反转的特殊考虑

反转开仓方向时, SL/TP 的处理是关键难点:

**场景**: AI建议 open_long BTC @ 87000, SL=85000, TP=92000

反转后: open_short BTC @ 87000

- **方案1 - 简单互换**: SL=92000, TP=85000
  - 逻辑: AI认为价格会涨到92000(错误), 实际会跌到85000
  - 问题: R:R比可能不合理, 且忽略了做空的止损需要在价格上方
  - 适用: 当AI的SL/TP本身就是基于技术位(支撑/阻力)分析时

- **方案2 - 镜像计算**: 基于当前价格对称翻转
  - SL_new = currentPrice + (currentPrice - SL_old) = 87000 + 2000 = 89000
  - TP_new = currentPrice - (TP_old - currentPrice) = 87000 - 5000 = 82000
  - 逻辑: 保持相同的风险距离, 但方向相反
  - 问题: 需要当前市场价格, 增加复杂度

- **推荐**: 方案1(简单互换)作为默认, 方案2作为可选配置。因为AI的SL/TP既然"方向错误", 互换后反而可能更接近正确的技术位。但需要通过 `kernel.ValidateRiskReward()` 做执行前校验, 确保R:R合理。

---

## 4. 配置系统扩展

### 4.1 StrategyConfig 新增字段

```go
// store/strategy.go - StrategyConfig 新增
type StrategyConfig struct {
    // ... 现有字段 ...

    // 决策变换配置
    DecisionTransform DecisionTransformConfig `json:"decision_transform,omitempty"`
}

// DecisionTransformConfig 决策变换配置
type DecisionTransformConfig struct {
    // 反向策略配置
    Reverse ReverseConfig `json:"reverse,omitempty"`
}
```

### 4.2 默认配置

```go
// 默认: 不启用反向 (向后兼容)
DecisionTransform: DecisionTransformConfig{
    Reverse: ReverseConfig{
        Enabled:        false,
        ReverseOpen:    true,   // 默认反转开仓
        ReverseClose:   false,  // 默认不反转平仓 (见下文分析)
        SwapSLTP:       true,   // 默认互换SL/TP
        LeverageScale:  1.0,    // 默认不调整杠杆
        PositionScale:  1.0,    // 默认不调整仓位
    },
},
```

### 4.3 关于平仓反转的架构考虑

**不建议默认反转平仓**, 原因:

1. **仓位一致性**: 反向开仓后, 实际持有的是反向仓位。AI后续的 close_long 指令是针对它认为存在的多仓, 但实际持有的是空仓, 应该执行 close_short
2. **正确逻辑**: 开仓反转 + 平仓反转 = 正确对应。即AI说close_long(它认为自己开了long), 反转成close_short(实际持有的short)
3. **结论**: 当 `ReverseOpen=true` 时, `ReverseClose` 也应该为 `true`, 它们应该是联动的

建议增加一个简化配置:

```go
type ReverseConfig struct {
    Enabled  bool `json:"enabled"`   // 总开关
    Mode     string `json:"mode"`    // "full"(全反转), "open_only"(仅开仓反转)
    // ...其他精细化配置
}
```

- `mode: "full"`: ReverseOpen=true, ReverseClose=true, SwapSLTP=true
- `mode: "open_only"`: ReverseOpen=true, ReverseClose=false, SwapSLTP=true (需手动管理平仓)

### 4.4 热配置更新

当前 `StrategyConfig` 通过数据库 JSON 字段存储, `SetActive()` 切换策略时重新加载。但 trader 在 `NewAutoTrader()` 时创建 `StrategyEngine`, 运行期间不会重新加载配置。

**实现热更新的最小改动**:

```go
// trader/auto_trader_loop.go - runCycle() 开头添加
func (at *AutoTrader) runCycle() error {
    // 检查策略配置是否有更新 (热配置)
    at.maybeReloadStrategyConfig()
    // ... 现有逻辑 ...
}

func (at *AutoTrader) maybeReloadStrategyConfig() {
    if at.store == nil {
        return
    }
    strategy, err := at.store.Strategy().GetActive(at.userID)
    if err != nil {
        return
    }
    config, err := strategy.ParseConfig()
    if err != nil {
        return
    }
    // 仅更新DecisionTransform部分, 不影响其他运行时状态
    at.config.StrategyConfig.DecisionTransform = config.DecisionTransform
}
```

---

## 5. 数据存储与审计

### 5.1 决策记录扩展

当前 `DecisionRecordDB` 已经保存:
- `decision_json`: AI决策JSON (原始)
- `raw_response`: AI原始响应
- `decisions`: 执行后的DecisionAction列表

**新增字段** (最小改动):

```go
// store/decision.go - DecisionRecordDB 新增
type DecisionRecordDB struct {
    // ... 现有字段 ...
    OriginalDecisionJSON string `gorm:"column:original_decision_json;default:''"` // 反转前的原始决策
    TransformApplied     string `gorm:"column:transform_applied;default:''"` // 应用的变换: "reverse", ""
}
```

**在 runCycle 中记录**:

```go
// 反转前: 保存原始决策
if len(pipeline.transformers) > 0 {
    originalJSON, _ := json.MarshalIndent(aiDecision.Decisions, "", "  ")
    record.OriginalDecisionJSON = string(originalJSON)
}

// 反转后: 使用变换后的决策
transformedResult := pipeline.Apply(aiDecision)
record.TransformApplied = strings.Join(transformedResult.Transforms, ",")
// 使用 transformedResult.Transformed.Decisions 执行
```

### 5.2 绩效对比数据模型

不需要新增表, 利用现有 `decision_records` 表中的 `original_decision_json` 和 `decision_json` 即可回溯对比:

```sql
-- 查询反向策略 vs 原始策略的对比
SELECT
    timestamp,
    original_decision_json AS ai_original,
    decision_json AS actually_executed,
    transform_applied
FROM decision_records
WHERE trader_id = ? AND transform_applied = 'reverse'
ORDER BY timestamp DESC;
```

### 5.3 AI准确率追踪

利用现有 `trader_positions` 表已有的盈亏数据:

```sql
-- AI原始方向 vs 实际结果 (反向策略启用时)
-- 如果反向后赚钱 = AI原始方向错误, 验证"90%错误"的假设
SELECT
    COUNT(*) AS total_trades,
    SUM(CASE WHEN realized_pnl > 0 THEN 1 ELSE 0 END) AS reverse_wins,
    SUM(CASE WHEN realized_pnl > 0 THEN 1 ELSE 0 END) * 100.0 / COUNT(*) AS reverse_win_rate
FROM trader_positions
WHERE trader_id = ? AND status = 'CLOSED'
AND created_at > (SELECT MIN(timestamp) FROM decision_records WHERE transform_applied = 'reverse');
```

---

## 6. 架构模式选择

### 对比分析

| 模式 | 适用性 | 优势 | 劣势 |
|------|--------|------|------|
| 策略模式 | 低 | 运行时切换正向/反向 | 需要抽象整个交易策略, 过度设计 |
| 装饰器模式 | 中 | 透明包装现有逻辑 | 装饰对象是Decision值, 非行为接口 |
| **管道模式** | **高** | 链式组合, 可扩展, 关注点分离 | 略增加一层间接 |

### 推荐: 管道模式 (Pipeline Pattern)

**理由**:

1. **Decision是数据对象, 非行为对象**: 策略模式和装饰器模式更适合行为抽象, 而这里是数据变换
2. **未来可扩展**: 管道可以串联多个变换器 (反向 -> 置信度过滤 -> 仓位缩放 -> 风控校验)
3. **纯函数特性**: 每个Transformer是纯函数 (输入Decision, 输出Decision), 极易测试
4. **最小侵入**: 现有代码只需在runCycle中增加一行管道调用

```go
// 未来扩展示例
pipeline := kernel.NewDecisionPipeline()
if config.DecisionTransform.Reverse.Enabled {
    pipeline.Add(kernel.NewReverseTransformer(config.DecisionTransform.Reverse))
}
// 未来可加入:
// pipeline.Add(kernel.NewConfidenceFilter(minConfidence))
// pipeline.Add(kernel.NewPositionScaler(maxPositionPct))
// pipeline.Add(kernel.NewRiskValidator(riskConfig))
```

---

## 7. 集成改动清单

### 7.1 新增文件

| 文件 | 描述 | 预计行数 |
|------|------|----------|
| `kernel/transformer.go` | DecisionTransformer接口, Pipeline, TransformedFullDecision | ~80行 |
| `kernel/transformer_reverse.go` | ReverseTransformer实现, ReverseConfig | ~100行 |
| `kernel/transformer_test.go` | 变换器单元测试 | ~200行 |

### 7.2 修改文件

| 文件 | 改动 | 影响范围 |
|------|------|----------|
| `store/strategy.go` | StrategyConfig新增DecisionTransformConfig | 仅新增字段, 向后兼容 |
| `store/decision.go` | DecisionRecordDB新增original_decision_json, transform_applied | 仅新增列, 向后兼容 |
| `trader/auto_trader_loop.go` | runCycle()中插入Pipeline调用 (~15行) | 最小改动, 在sortDecisionsByPriority前 |
| `trader/auto_trader.go` | NewAutoTrader()中初始化Pipeline | ~10行 |

### 7.3 runCycle 改动示意

```go
// trader/auto_trader_loop.go - runCycle()
// 在第209行 sortDecisionsByPriority 之前插入:

// === 新增: 决策变换管道 ===
if at.decisionPipeline != nil {
    transformResult := at.decisionPipeline.Apply(aiDecision)

    // 保存原始决策用于审计
    if transformResult.Original != transformResult.Transformed {
        originalJSON, _ := json.MarshalIndent(transformResult.Original.Decisions, "", "  ")
        record.OriginalDecisionJSON = string(originalJSON)
        record.TransformApplied = strings.Join(transformResult.Transforms, ",")

        logger.Infof("🔄 Decision transformed: %s", record.TransformApplied)
        for i, d := range transformResult.Transformed.Decisions {
            orig := transformResult.Original.Decisions[i]
            if orig.Action != d.Action {
                logger.Infof("  [%d] %s %s -> %s %s", i+1, orig.Symbol, orig.Action, d.Symbol, d.Action)
            }
        }
    }

    aiDecision = transformResult.Transformed
}

// 继续现有逻辑...
sortedDecisions := sortDecisionsByPriority(aiDecision.Decisions)
```

### 7.4 AutoTrader 初始化改动

```go
// trader/auto_trader.go - NewAutoTrader()
// 在创建strategyEngine后, return前:

// 初始化决策变换管道
var decisionPipeline *kernel.DecisionPipeline
if config.StrategyConfig.DecisionTransform.Reverse.Enabled {
    decisionPipeline = kernel.NewDecisionPipeline()
    decisionPipeline.Add(kernel.NewReverseTransformer(
        config.StrategyConfig.DecisionTransform.Reverse,
    ))
    logger.Infof("🔄 [%s] Reverse strategy enabled (mode: %s)",
        config.Name, config.StrategyConfig.DecisionTransform.Reverse.Mode)
}

return &AutoTrader{
    // ... 现有字段 ...
    decisionPipeline: decisionPipeline, // 新增字段
}, nil
```

---

## 8. 向后兼容与渐进式部署

### 8.1 向后兼容保障

1. **默认关闭**: `ReverseConfig.Enabled` 默认 `false`, 现有用户完全不受影响
2. **数据库兼容**: 新增字段使用 `default:''`, 不需要数据迁移
3. **JSON兼容**: `DecisionTransformConfig` 使用 `omitempty`, 旧配置JSON解析不会报错

### 8.2 渐进式部署计划

```
Phase 1: 影子模式 (Shadow Mode) - 1周
  - 启用Pipeline但不执行反转
  - 同时记录原始决策和假设反转后的决策
  - 回测分析: 如果反转会怎样?

Phase 2: 部分反转 - 1周
  - mode: "open_only" (仅反转开仓)
  - 缩小仓位: position_scale: 0.5 (半仓试水)
  - 密切监控胜率和回撤

Phase 3: 完全反转 - 持续运行
  - mode: "full" (全反转)
  - 正常仓位: position_scale: 1.0
  - 对比分析确认后全量切换
```

### 8.3 A/B测试方案

利用现有多Trader架构, 可以同时运行正向和反向两个Trader:

```yaml
# config.yaml
traders:
  - id: "trader-forward"
    strategy_config:
      decision_transform:
        reverse:
          enabled: false

  - id: "trader-reverse"
    strategy_config:
      decision_transform:
        reverse:
          enabled: true
          mode: "full"
          position_scale: 0.5  # 反向策略用小仓位
```

两个Trader共享同一个AI输出, 但一个正向执行, 一个反向执行。通过 `trader_positions` 表的 `trader_id` 区分绩效。

### 8.4 回滚方案

1. **配置回滚**: 将 `reverse.enabled` 设为 `false`, 下一个cycle即生效
2. **热回滚**: 利用4.4节的热配置更新, 通过Web UI修改策略配置即可, 无需重启
3. **紧急回滚**: 停止反向Trader (`POST /api/traders/{id}/stop`), 切换到正向Trader

---

## 9. 风险评估

### 9.1 技术风险

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|----------|
| SL/TP互换后R:R不满足 | 高 | 中 | 执行前ValidateRiskReward校验已存在 |
| 平仓方向错配 | 中 | 高 | 单元测试覆盖所有action组合 |
| 管道性能开销 | 低 | 低 | 纯内存操作, 纳秒级 |
| 配置解析兼容性 | 低 | 中 | omitempty + 默认值 |

### 9.2 业务风险

| 风险 | 描述 | 缓解措施 |
|------|------|----------|
| "90%错误"假设不成立 | 如果AI只是50%错误, 反转也是50% | Phase 1影子模式验证假设 |
| 市场环境变化 | AI从错误变正确, 反转反而亏损 | 监控反转胜率, 低于阈值自动停用 |
| 反转后风控失效 | SL/TP互换后可能不符合风控规则 | 现有enforceXxx检查仍然生效 |

---

## 10. 总结

### 推荐架构

采用 **Pipeline 管道模式**, 在 kernel 层新增 `DecisionTransformer` 接口和 `DecisionPipeline`, 在 trader 层的 `runCycle()` 中插入管道调用。总改动量约 400 行代码 (含测试), 影响文件 4 个, 新增文件 3 个。

### 关键设计决策

1. **注入点**: trader层 runCycle() 中 sortDecisionsByPriority() 之前 (方案C)
2. **架构模式**: Pipeline (管道模式)
3. **SL/TP处理**: 默认简单互换, 依赖现有 ValidateRiskReward 校验
4. **平仓反转**: 默认跟随开仓反转 (mode: "full")
5. **审计**: 原始决策和变换后决策同时记录到 decision_records 表
6. **部署**: 影子模式 -> 部分反转 -> 完全反转, 三阶段渐进

### 改动影响总结

```
新增文件:
  kernel/transformer.go          (~80行)   核心接口和管道
  kernel/transformer_reverse.go  (~100行)  反向变换器
  kernel/transformer_test.go     (~200行)  单元测试

修改文件:
  store/strategy.go              (+15行)   新增配置字段
  store/decision.go              (+5行)    新增审计字段
  trader/auto_trader.go          (+10行)   初始化Pipeline
  trader/auto_trader_loop.go     (+20行)   插入Pipeline调用

总计: 新增 ~380行, 修改 ~50行
```
