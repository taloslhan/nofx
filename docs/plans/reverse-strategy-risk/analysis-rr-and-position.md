# R:R比率适配与仓位风控

## 1. SL/TP互换后的R:R变化分析

### 1.1 反向前后的R:R对比

**场景: AI建议 open_long BTCUSDT @ 85000**
```
AI原始:  SL = 84000 (1.18%)  TP = 88000 (3.53%)  R:R = 3:1
反向后:  open_short @ 85000
         新SL = 88000 (3.53%)  新TP = 84000 (1.18%)  R:R = 1:3
```

反向后R:R从3:1变为1:3。这意味着：
- 每次亏损的金额是每次盈利金额的3倍
- 需要胜率 > 75% 才能保持正期望 (breakeven = 3/(1+3) = 75%)
- 在假设90%胜率下: EV = 0.9 x 1R - 0.1 x 3R = +0.6R（仍然正期望）

### 1.2 ValidateRiskReward 函数适配

现有 `kernel.ValidateRiskReward()` 的逻辑:
```go
// open_long: risk = (entry - SL) / entry, reward = (TP - entry) / entry
// R:R = reward / risk >= MinRiskRewardRatio
```

反向策略有两个方案：

**方案A: 在反向层互换SL/TP后，使用独立的R:R阈值**
```
反向后R:R阈值: MinRiskRewardRatio = 0.3 (即接受1:3的R:R)
理由: 90%胜率下，1:3的R:R仍有 EV = 0.9x1 - 0.1x3 = +0.6R
```

**方案B: 不使用AI的SL/TP，自行计算**
```
反向开仓后，基于ATR自行设定SL/TP:
SL距离 = 1.5 x ATR（更宽的止损，给反向仓位更多空间）
TP距离 = 1.0 x ATR（更近的止盈，快速获利了结）
R:R = 1:1.5（更合理的比率）
```

**推荐: 方案B**，理由：
- AI的SL/TP是基于其错误的方向判断设定的，互换后不一定合理
- 基于波动率(ATR)的SL/TP更客观
- 可以控制R:R在一个合理范围内

### 1.3 建议的R:R参数

| 指标 | 值 | 说明 |
|------|-----|------|
| SL距离 | 1.5 x ATR(14) | 给仓位足够呼吸空间 |
| TP距离 | 1.0 x ATR(14) | 快速获利了结 |
| 最小R:R | 0.5:1 | 反向策略容忍的最低R:R |
| 理想R:R | 0.67:1 (1:1.5) | 基于ATR计算的目标 |
| MinSLDistancePct | 0.5% | 保持不变（最小距离保护） |

## 2. 杠杆适配

### 2.1 反向后杠杆是否需要降低？

**是的，建议降低。** 原因：

1. **反向错误时损失放大**
   - 10%的情况AI是对的（我们反向后亏损），高杠杆放大这10%的损失
   - 5x杠杆 + 3%价格波动 = 15%保证金损失
   - 10x杠杆 + 3%价格波动 = 30%保证金损失

2. **强平风险**
   - 反向后SL距离可能更远（原TP变成新SL）
   - 高杠杆下更容易触及强平价
   - 现有 `enforcePositionValueRatio` 已做限制，但杠杆本身需要降低

### 2.2 建议杠杆配置

| 币种 | 现有杠杆 | 反向策略建议 | 理由 |
|------|---------|-------------|------|
| BTC | 5-10x | 3-5x | 波动较小但错误时损失显著 |
| ETH | 5-10x | 3-5x | 同BTC |
| Altcoin | 5x | 2-3x | 山寨币波动大，反向错误时损失更大 |

### 2.3 与PositionValueRatio的交互

现有代码中 `enforcePositionValueRatio` 限制了仓位价值：
- BTC/ETH: positionValue <= equity x 5.0
- Altcoin: positionValue <= equity x 1.0

反向策略建议：
- BTC/ETH: positionValue <= equity x 2.0
- Altcoin: positionValue <= equity x 0.5
- 实际保证金占用 = positionValue / leverage，更低的ratio + 更低的杠杆 = 更安全

## 3. 仓位大小优化

### 3.1 Kelly公式计算最优仓位

假设反向策略参数:
- 胜率 p = 0.90
- 赢时R:R = 0.67 (win/loss ratio b = 0.67)

Kelly fraction: f* = (p x b - (1-p)) / b = (0.90 x 0.67 - 0.10) / 0.67 = 0.753

Kelly建议用75%的资金交易。但实际使用中应打折:
- **Half Kelly**: f = 37.5%
- **Quarter Kelly**: f = 18.75%
- **建议使用Quarter Kelly (约20%)**: 更保守，考虑到模型不确定性

### 3.2 反向策略仓位建议

| 指标 | 现有值 | 反向策略建议 | 理由 |
|------|--------|-------------|------|
| 单笔最大仓位(BTC/ETH) | equity x 5.0 | equity x 2.0 | 降低单笔风险暴露 |
| 单笔最大仓位(Altcoin) | equity x 1.0 | equity x 0.5 | 山寨币更保守 |
| MaxMarginUsage | 90% | 60% | 预留40%安全垫 |
| 单笔风险(占权益) | 无明确限制 | 最大2% | Kelly/4的保守策略 |
| MaxPositions | 3 | 2 | 减少同时暴露 |

### 3.3 分批建仓是否适用？

**不建议分批建仓**，原因：
1. 反向策略的信号是"一次性的"（AI给出方向，反向执行），没有"信号逐步加强"的概念
2. 分批建仓会增加复杂度和滑点成本
3. 高胜率策略应该一次性建仓，快速获利了结
4. 唯一例外：大仓位时为减少市场冲击可以拆单执行（但这是执行层面而非策略层面）

### 3.4 MaxMarginUsage 影响分析

现有60%占用上限下的场景分析（假设equity = 1000 USDT）：
```
可用保证金: 1000 x 60% = 600 USDT
BTC 3x杠杆: 仓位价值 2000 USDT, 保证金 667 USDT → 超限
BTC 3x杠杆: 仓位价值 1800 USDT, 保证金 600 USDT → 刚好
Altcoin 2x: 仓位价值 500 USDT, 保证金 250 USDT → 可以开2个
```

这样在保证安全的同时，仍有足够的交易空间。
