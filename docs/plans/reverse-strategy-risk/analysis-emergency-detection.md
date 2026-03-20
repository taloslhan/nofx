# 紧急机制设计与AI准确率监控

## 1. 反向策略紧急停止条件

### 1.1 分级紧急响应

| 级别 | 触发条件 | 响应动作 | 自动恢复 |
|------|---------|---------|---------|
| L1 警告 | 日亏损 > 3% 权益 OR 连续2笔亏损 | 仓位减半，发送告警 | 次日自动恢复 |
| L2 暂停 | 日亏损 > 5% 权益 OR 周亏损 > 8% | 停止新开仓，保留现有仓位 | 次日自动恢复 |
| L3 停止 | 日亏损 > 8% OR AI准确率 > 33% | 平掉所有仓位，停止交易 | 需人工审核 |
| L4 紧急 | 账户权益下降 > 20%(从峰值) | 全面停止，通知管理员 | 需手动重启 |

### 1.2 与现有机制的集成

现有的 `stopUntil` 机制可以直接用于L1/L2的自动恢复:
```
L1: stopUntil = now + 残余交易日时间
L2: stopUntil = next day UTC 00:00
L3: 需要新增 manualReviewRequired flag
L4: 需要新增 emergencyShutdown flag
```

现有的 `EmergencyCloseLossPct: -20%` 建议调整为 `-15%`:
- 反向策略的SL距离更远，需要更早的紧急退出
- 配合降低的杠杆，-15%保证金PnL对应更大的价格波动

## 2. AI准确率监控 -- 滑动窗口方案

### 2.1 核心设计

```
AIAccuracyMonitor {
    windowSize:     50     // 最近50笔交易
    trades:         []TradeResult  // 滑动窗口
    aiCorrectCount: int    // AI原始方向正确的次数

    // AI原始决策记录
    OnAIDecision(symbol, aiDirection, aiSL, aiTP)

    // 交易结果记录（反向后的实际结果）
    OnTradeClose(symbol, pnl, exitReason)

    // 判断AI原始方向是否正确
    // 如果反向后交易亏损 → AI原始方向正确
    IsAICorrect(tradeResult) bool

    // 获取当前AI准确率
    GetAIAccuracy() float64
}
```

### 2.2 判断AI准确率的方法

**方法: 基于反向交易的盈亏**
```
如果反向后交易盈利 → AI原始方向错误（期望情况）
如果反向后交易亏损 → AI原始方向正确（异常情况）

AI准确率 = 反向后亏损笔数 / 总笔数
```

这种方法简洁可靠，直接反映了反向策略的有效性。

### 2.3 滑动窗口参数

| 参数 | 值 | 理由 |
|------|-----|------|
| 窗口大小 | 50笔 | 足够统计意义，又不会太滞后 |
| 最小样本 | 20笔 | 少于20笔不做准确率判断 |
| 检查频率 | 每笔交易结束后 | 实时更新 |
| 半衰期 | 无（均等权重） | 简单可靠 |

### 2.4 报警阈值

```
窗口50笔中:
- AI准确率 > 20% (10/50): 信息级日志
- AI准确率 > 25% (13/50): L1 警告
- AI准确率 > 30% (15/50): L2 暂停 + 减半仓位
- AI准确率 > 33% (17/50): L3 停止反向策略
```

### 2.5 连续亏损快速检测

除了滑动窗口，还需要连续亏损的快速检测:
```
连续2笔亏损: 下一笔仓位减半
连续3笔亏损: L2 暂停
连续4笔亏损: L3 停止（不等窗口统计）
```

这比滑动窗口更灵敏，可以在AI突然"变聪明"时快速反应。

## 3. 自动切换回正向策略的触发条件

### 3.1 切换逻辑

**建议: 不自动切换回正向，而是暂停等人工审核。** 理由:

1. 如果AI"变聪明了"（准确率提高），不确定是暂时的还是永久的
2. 自动切换可能导致策略在正向/反向之间频繁摆动
3. AI准确率变化可能是市场regime变化导致的，需要人工判断

### 3.2 如果确实需要自动切换

触发条件（所有条件必须同时满足）:
```
1. 最近50笔AI准确率 > 60% (持续高准确率)
2. 最近20笔AI准确率 > 65% (近期趋势确认)
3. 正向策略模拟收益 > 反向策略实际收益 (对比确认)
4. 持续时间 > 48小时 (不是短期波动)
```

### 3.3 切换后的风控

切换回正向后，应该使用更保守的参数（因为不确定AI是否真的变好了）:
- 仓位减至正常的50%
- 3天内逐步恢复到正常仓位
- 如果切换后表现不佳，立即停止而非再次反向

## 4. 数据结构设计

### 4.1 需要存储的数据

```go
type ReverseStrategyState struct {
    // 滑动窗口
    RecentTrades      []ReverseTradeResult  // 最近N笔交易
    WindowSize        int                    // 窗口大小

    // AI准确率统计
    AICorrectCount    int      // 窗口内AI正确次数
    AIAccuracyPct     float64  // 当前AI准确率

    // 连续计数
    ConsecutiveLosses int      // 当前连续亏损次数
    MaxConsecLosses   int      // 历史最大连续亏损

    // 每日统计
    DailyPnL          float64  // 当日PnL
    DailyTradeCount   int      // 当日交易次数
    DailyLossCount    int      // 当日亏损次数

    // 警戒状态
    AlertLevel         int     // 当前警戒级别 0-4
    PositionSizeMultiplier float64 // 仓位倍数（减仓用）

    // 策略状态
    IsActive           bool    // 反向策略是否激活
    PausedUntil        time.Time // 暂停至何时
    RequiresManualReview bool  // 是否需要人工审核
}

type ReverseTradeResult struct {
    Symbol          string
    AIDirection     string    // AI原始建议方向
    ActualDirection string    // 实际执行方向（反向后）
    EntryPrice      float64
    ExitPrice       float64
    PnL             float64
    PnLPct          float64
    AIWasCorrect    bool      // AI原始方向是否正确
    ExitReason      string    // sl/tp/trailing/time/manual
    Timestamp       time.Time
}
```

### 4.2 与现有Store的集成

可以在现有 `store` 包中新增:
- `ReverseStrategyStore`: 持久化反向策略状态
- 或者简单地在 `AutoTrader` 中添加内存状态 + 定期持久化到 JSON 文件

建议先用内存 + JSON文件的轻量方案，验证策略后再考虑数据库持久化。
