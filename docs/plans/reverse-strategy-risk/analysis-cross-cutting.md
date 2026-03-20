# 跨维度风控架构决策 -- 反向策略核心矛盾

## 1. 反向操作的根本矛盾

反向策略与现有nofx风控体系存在结构性冲突：

**矛盾1: R:R验证逻辑**
- 现有代码 `kernel.ValidateRiskReward()` 验证 TP/SL 比率 >= `MinRiskRewardRatio`
- AI给出的SL/TP是基于其"正向"判断。反向后，原SL变TP、原TP变SL
- 原始R:R 3:1 反向后变 1:3，会被代码强制拒绝
- **决策**: 反向策略需要独立的R:R计算路径，或在反向层完成SL/TP互换后重新计算

**矛盾2: 方向语义**
- AI输出 `open_long` 反向后执行 `open_short`
- 但AI的 `Reasoning` 和 `Confidence` 仍然基于正向判断
- 反向后confidence含义反转：AI说"90%确信做多" = 我们90%确信做空

**矛盾3: 平仓信号**
- AI说 `close_long` 可能是正确的（止损/止盈判断可能无方向偏差）
- 盲目反向平仓（不平仓 or 反向操作）可能造成更大风险
- **决策建议**: 平仓信号不反向，独立止损机制为主

## 2. 反向层的架构位置

```
AI Decision → [反向层] → Risk Control → Exchange Execution
                ↓
        仅反向 action + 互换SL/TP
        不改变: symbol, leverage, position_size, confidence
```

反向层应位于AI决策输出之后、风控检查之前。具体操作：

| AI输出 | 反向后 | SL/TP处理 |
|--------|--------|-----------|
| open_long | open_short | SL ↔ TP 互换 |
| open_short | open_long | SL ↔ TP 互换 |
| close_long | **不反向** | 保持原样 |
| close_short | **不反向** | 保持原样 |
| hold/wait | **不反向** | 保持原样 |

## 3. 关键决策: 平仓是否反向？

**建议: 平仓不反向**，理由如下：

1. **AI的方向判断错误 != 平仓时机判断错误**
   - AI可能对"什么时候该平仓"的判断相对准确（趋势耗尽、动量减弱等信号与方向无关）
   - 反向平仓等于"AI说该平仓时不平仓"，这违反了风险管理原则

2. **独立止损更可靠**
   - 交易所端SL/TP订单是硬保护，不依赖AI
   - 追踪止损、时间止损提供额外保护层

3. **防止仓位僵死**
   - 如果平仓也反向，AI说close时不close，可能导致亏损仓位长期持有

## 4. 风控参数总体调整方向

| 参数 | 现有值 | 反向策略建议值 | 理由 |
|------|--------|--------------|------|
| MaxPositions | 3 | 2 | 反向策略不确定性更高，减少同时暴露 |
| BTCETHMaxLeverage | 5-10 | 3-5 | 降低杠杆缓冲反向错误时的损失 |
| AltcoinMaxLeverage | 5 | 3 | 同上 |
| MinRiskRewardRatio | 3.0 | 0.5 (反向后计算) | 反向后R:R天然较低，靠高胜率补偿 |
| MaxMarginUsage | 90% | 60% | 预留更多安全垫 |
| MinPositionSize | 12 USDT | 12 USDT | 保持不变 |
| BTCETHMaxPositionValueRatio | 5.0 | 2.0 | 降低单笔风险敞口 |
| AltcoinMaxPositionValueRatio | 1.0 | 0.5 | 降低山寨币风险 |
| MinHoldMinutes | 120 | 60 | 反向策略可能需要更快的反应 |
| CooldownMinutes | 180 | 240 | 增加冷却避免频繁反向交易 |
| EmergencyCloseLossPct | -20% | -15% | 更早触发紧急平仓 |

## 5. 新增风控维度

反向策略需要引入现有代码中不存在的风控维度：

1. **AI准确率监控器** (AIAccuracyMonitor)
   - 滑动窗口跟踪AI原始方向的正确率
   - 当正确率超过30%时报警，超过40%时暂停策略

2. **反向策略效果追踪器** (ReversePerformanceTracker)
   - 独立追踪反向策略的胜率、R:R、期望值
   - 实时计算滑动窗口内的期望值

3. **每日亏损熔断** (DailyLossCircuitBreaker)
   - 现有 `dailyPnL` 追踪需要增加硬停止功能
   - 日亏损达到账户权益5%时自动停止交易至次日

4. **周亏损上限** (WeeklyLossLimit)
   - 周累计亏损达到账户权益10%时停止至下周一
