import { useState } from 'react'
import { ChevronDown, ChevronRight, RotateCcw, FileText } from 'lucide-react'
import type { PromptSectionsConfig } from '../../types'
import {
  promptSections as promptSectionsI18n,
  ts,
} from '../../i18n/strategy-translations'

interface PromptSectionsEditorProps {
  config: PromptSectionsConfig | undefined
  onChange: (config: PromptSectionsConfig) => void
  disabled?: boolean
  language: string
}

const getDefaultSections = (language: string): PromptSectionsConfig => {
  if (language === 'zh') {
    return {
      role_definition: `# 你是一个专业的加密货币交易AI

你的任务是根据提供的市场数据做出交易决策。你是一个经验丰富的量化交易员，擅长技术分析和风险管理。`,

      trading_frequency: `# ⏱️ 交易频率意识

- 优秀交易员：每天2-4笔 ≈ 每小时0.1-0.2笔
- 每小时超过2笔 = 过度交易
- 单笔持仓时间 ≥ 30-60分钟
如果你发现自己每个周期都在交易 → 标准太低；如果持仓不到30分钟就平仓 → 太冲动。`,

      entry_standards: `# 🎯 入场标准（严格）

只在多个信号共振时入场。自由使用任何有效的分析方法，但必须遵守时间框架层级：
- 4H：判断趋势方向，只做顺 4H 趋势的交易
- 1H：确认入场时机，是主信号来源
- 15m：仅用于优化入场点，不可单独决定方向

禁止行为：
- 禁止在 4H 趋势向下时开多（反之亦然）
- 禁止仅凭 15m 信号开仓
- 禁止单一指标、信号矛盾、横盘震荡、平仓后立即重开等低质量行为`,

      decision_process: `# 📋 决策流程

1. 检查持仓 → 是否止盈/止损
2. 扫描候选币种 + 多时间框架 → 是否存在强信号
3. 先写思维链，再输出结构化JSON`,
    }
  }

  return {
    role_definition: `# You are a professional cryptocurrency trading AI

Your task is to make trading decisions based on the provided market data. You are an experienced quantitative trader skilled in technical analysis and risk management.`,

    trading_frequency: `# ⏱️ Trading Frequency Awareness

- Excellent trader: 2-4 trades per day ≈ 0.1-0.2 trades per hour
- >2 trades per hour = overtrading
- Single position holding time ≥ 30-60 minutes
If you find yourself trading every cycle → standards are too low; if closing positions in <30 minutes → too impulsive.`,

    entry_standards: `# 🎯 Entry Standards (Strict)

Only enter positions when multiple signals resonate. Freely use any effective analysis methods, but respect the timeframe hierarchy:
- 4H: determine trend direction, only trade with the 4H trend
- 1H: confirm entry timing, this is the primary signal source
- 15m: refine execution only, never decide direction by itself

Forbidden behaviors:
- Do not open longs against a bearish 4H trend (and vice versa)
- Do not open positions based only on 15m signals
- Avoid low-quality behaviors such as single indicators, contradictory signals, sideways chop, or reopening immediately after closing`,

    decision_process: `# 📋 Decision Process

1. Check positions → whether to take profit/stop loss
2. Scan candidate coins + multi-timeframe → whether strong signals exist
3. Write chain of thought first, then output structured JSON`,
  }
}

export function PromptSectionsEditor({
  config,
  onChange,
  disabled,
  language,
}: PromptSectionsEditorProps) {
  const defaultSections = getDefaultSections(language)

  const [expandedSections, setExpandedSections] = useState<
    Record<string, boolean>
  >({
    role_definition: false,
    trading_frequency: false,
    entry_standards: false,
    decision_process: false,
  })

  const sections = [
    {
      key: 'role_definition',
      label: ts(promptSectionsI18n.roleDefinition, language),
      desc: ts(promptSectionsI18n.roleDefinitionDesc, language),
    },
    {
      key: 'trading_frequency',
      label: ts(promptSectionsI18n.tradingFrequency, language),
      desc: ts(promptSectionsI18n.tradingFrequencyDesc, language),
    },
    {
      key: 'entry_standards',
      label: ts(promptSectionsI18n.entryStandards, language),
      desc: ts(promptSectionsI18n.entryStandardsDesc, language),
    },
    {
      key: 'decision_process',
      label: ts(promptSectionsI18n.decisionProcess, language),
      desc: ts(promptSectionsI18n.decisionProcessDesc, language),
    },
  ]

  const currentConfig = config || {}

  const updateSection = (key: keyof PromptSectionsConfig, value: string) => {
    if (!disabled) {
      onChange({ ...currentConfig, [key]: value })
    }
  }

  const resetSection = (key: keyof PromptSectionsConfig) => {
    if (!disabled) {
      onChange({ ...currentConfig, [key]: defaultSections[key] })
    }
  }

  const toggleSection = (key: string) => {
    setExpandedSections((prev) => ({ ...prev, [key]: !prev[key] }))
  }

  const getValue = (key: keyof PromptSectionsConfig): string => {
    return currentConfig[key] || defaultSections[key] || ''
  }

  return (
    <div className="space-y-4">
      <div className="flex items-start gap-2 mb-4">
        <FileText className="w-5 h-5 mt-0.5" style={{ color: '#a855f7' }} />
        <div>
          <h3 className="font-medium" style={{ color: '#EAECEF' }}>
            {ts(promptSectionsI18n.promptSections, language)}
          </h3>
          <p className="text-xs mt-1" style={{ color: '#848E9C' }}>
            {ts(promptSectionsI18n.promptSectionsDesc, language)}
          </p>
        </div>
      </div>

      <div className="space-y-2">
        {sections.map(({ key, label, desc }) => {
          const sectionKey = key as keyof PromptSectionsConfig
          const isExpanded = expandedSections[key]
          const value = getValue(sectionKey)
          const isModified =
            currentConfig[sectionKey] !== undefined &&
            currentConfig[sectionKey] !== defaultSections[sectionKey]

          return (
            <div
              key={key}
              className="rounded-lg overflow-hidden"
              style={{ background: '#0B0E11', border: '1px solid #2B3139' }}
            >
              <button
                onClick={() => toggleSection(key)}
                className="w-full flex items-center justify-between px-3 py-2.5 hover:bg-white/5 transition-colors text-left"
              >
                <div className="flex items-center gap-2">
                  {isExpanded ? (
                    <ChevronDown
                      className="w-4 h-4"
                      style={{ color: '#848E9C' }}
                    />
                  ) : (
                    <ChevronRight
                      className="w-4 h-4"
                      style={{ color: '#848E9C' }}
                    />
                  )}
                  <span
                    className="text-sm font-medium"
                    style={{ color: '#EAECEF' }}
                  >
                    {label}
                  </span>
                  {isModified && (
                    <span
                      className="px-1.5 py-0.5 text-[10px] rounded"
                      style={{
                        background: 'rgba(168, 85, 247, 0.15)',
                        color: '#a855f7',
                      }}
                    >
                      {ts(promptSectionsI18n.modified, language)}
                    </span>
                  )}
                </div>
                <span className="text-[10px]" style={{ color: '#848E9C' }}>
                  {value.length} {ts(promptSectionsI18n.chars, language)}
                </span>
              </button>

              {isExpanded && (
                <div className="px-3 pb-3">
                  <p className="text-xs mb-2" style={{ color: '#848E9C' }}>
                    {desc}
                  </p>
                  <textarea
                    value={value}
                    onChange={(e) => updateSection(sectionKey, e.target.value)}
                    disabled={disabled}
                    rows={6}
                    className="w-full px-3 py-2 rounded-lg resize-y font-mono text-xs"
                    style={{
                      background: '#1E2329',
                      border: '1px solid #2B3139',
                      color: '#EAECEF',
                      minHeight: '120px',
                    }}
                  />
                  <div className="flex justify-end mt-2">
                    <button
                      onClick={() => resetSection(sectionKey)}
                      disabled={disabled || !isModified}
                      className="flex items-center gap-1 px-2 py-1 rounded text-xs transition-colors hover:bg-white/5 disabled:opacity-30"
                      style={{ color: '#848E9C' }}
                    >
                      <RotateCcw className="w-3 h-3" />
                      {ts(promptSectionsI18n.resetToDefault, language)}
                    </button>
                  </div>
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
