import { Repeat, Scale, ShieldAlert, ToggleLeft } from 'lucide-react'
import type { ReverseStrategyConfig } from '../../types'
import { reverseStrategy, ts } from '../../i18n/strategy-translations'

type SLTPMode = NonNullable<ReverseStrategyConfig['sltp_mode']>

interface ReverseStrategyEditorProps {
  config?: ReverseStrategyConfig
  onChange: (config: ReverseStrategyConfig) => void
  disabled?: boolean
  language: string
}

const defaultReverseStrategyConfig: ReverseStrategyConfig = {
  enabled: false,
  sltp_mode: 'recalculate',
  leverage_scale: 1,
  position_scale: 1,
  min_risk_reward_ratio: 0.5,
}

function resolveSLTPMode(config?: ReverseStrategyConfig): SLTPMode {
  if (config?.sltp_mode) {
    return config.sltp_mode
  }
  if (typeof config?.swap_sl_tp === 'boolean') {
    return config.swap_sl_tp ? 'swap' : 'none'
  }
  return 'recalculate'
}

export function ReverseStrategyEditor({
  config,
  onChange,
  disabled,
  language,
}: ReverseStrategyEditorProps) {
  const merged = {
    ...defaultReverseStrategyConfig,
    ...config,
    sltp_mode: resolveSLTPMode(config),
  }

  const updateField = <K extends keyof ReverseStrategyConfig>(
    key: K,
    value: ReverseStrategyConfig[K]
  ) => {
    if (!disabled) {
      onChange({ ...merged, [key]: value })
    }
  }

  const parseDecimal = (value: string, fallback: number) => {
    const parsed = Number.parseFloat(value)
    return Number.isFinite(parsed) ? parsed : fallback
  }

  const updateSLTPMode = (mode: SLTPMode) => {
    if (!disabled) {
      onChange({
        ...merged,
        sltp_mode: mode,
        swap_sl_tp: mode === 'swap',
      })
    }
  }

  const modeOptions: Array<{
    value: SLTPMode
    title: typeof reverseStrategy.modeSwapTitle
    description: typeof reverseStrategy.modeSwapDesc
  }> = [
    {
      value: 'recalculate',
      title: reverseStrategy.modeRecalculateTitle,
      description: reverseStrategy.modeRecalculateDesc,
    },
    {
      value: 'swap',
      title: reverseStrategy.modeSwapTitle,
      description: reverseStrategy.modeSwapDesc,
    },
    {
      value: 'none',
      title: reverseStrategy.modeNoneTitle,
      description: reverseStrategy.modeNoneDesc,
    },
  ]

  return (
    <div className="space-y-6">
      <div
        className="rounded-lg p-4"
        style={{ background: '#0B0E11', border: '1px solid #2B3139' }}
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <div className="flex items-center gap-2 mb-1">
              <Repeat className="w-5 h-5" style={{ color: '#60a5fa' }} />
              <h3 className="font-medium" style={{ color: '#EAECEF' }}>
                {ts(reverseStrategy.enabled, language)}
              </h3>
            </div>
            <p className="text-xs" style={{ color: '#848E9C' }}>
              {ts(reverseStrategy.enabledDesc, language)}
            </p>
          </div>
          <label className="inline-flex items-center cursor-pointer">
            <input
              type="checkbox"
              className="sr-only"
              checked={merged.enabled ?? false}
              onChange={(e) => updateField('enabled', e.target.checked)}
              disabled={disabled}
            />
            <div
              className="relative w-12 h-7 rounded-full transition-colors"
              style={{
                background: merged.enabled ? '#60a5fa' : '#2B3139',
                opacity: disabled ? 0.5 : 1,
              }}
            >
              <div
                className="absolute top-1 left-1 w-5 h-5 rounded-full bg-white transition-transform"
                style={{
                  transform: merged.enabled
                    ? 'translateX(20px)'
                    : 'translateX(0)',
                }}
              />
            </div>
          </label>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div
          className="rounded-lg p-4"
          style={{ background: '#0B0E11', border: '1px solid #2B3139' }}
        >
          <div className="flex items-center gap-2 mb-2">
            <ToggleLeft className="w-4 h-4" style={{ color: '#F0B90B' }} />
            <label className="text-sm font-medium" style={{ color: '#EAECEF' }}>
              {ts(reverseStrategy.sltpMode, language)}
            </label>
          </div>
          <p className="text-xs mb-3" style={{ color: '#848E9C' }}>
            {ts(reverseStrategy.sltpModeDesc, language)}
          </p>
          <div className="space-y-2">
            {modeOptions.map((option) => {
              const isSelected = merged.sltp_mode === option.value
              return (
                <button
                  key={option.value}
                  type="button"
                  onClick={() => updateSLTPMode(option.value)}
                  disabled={disabled || !merged.enabled}
                  className="w-full rounded-lg px-3 py-3 text-left transition-colors"
                  style={{
                    background: isSelected ? '#1E2A3A' : '#1E2329',
                    border: `1px solid ${isSelected ? '#60a5fa' : '#2B3139'}`,
                    opacity: disabled || !merged.enabled ? 0.5 : 1,
                  }}
                >
                  <div
                    className="text-sm font-medium"
                    style={{ color: '#EAECEF' }}
                  >
                    {ts(option.title, language)}
                  </div>
                  <div className="text-xs mt-1" style={{ color: '#848E9C' }}>
                    {ts(option.description, language)}
                  </div>
                </button>
              )
            })}
          </div>
        </div>

        <div
          className="rounded-lg p-4"
          style={{ background: '#0B0E11', border: '1px solid #2B3139' }}
        >
          <div className="flex items-center gap-2 mb-2">
            <ShieldAlert className="w-4 h-4" style={{ color: '#F6465D' }} />
            <label className="text-sm font-medium" style={{ color: '#EAECEF' }}>
              {ts(reverseStrategy.minRiskRewardRatio, language)}
            </label>
          </div>
          <p className="text-xs mb-3" style={{ color: '#848E9C' }}>
            {ts(reverseStrategy.minRiskRewardRatioDesc, language)}
          </p>
          <input
            type="number"
            min={0.1}
            max={3}
            step={0.1}
            value={merged.min_risk_reward_ratio ?? 0.5}
            onChange={(e) =>
              updateField(
                'min_risk_reward_ratio',
                parseDecimal(e.target.value, 0.5)
              )
            }
            disabled={disabled || !merged.enabled}
            className="w-32 px-3 py-2 rounded"
            style={{
              background: '#1E2329',
              border: '1px solid #2B3139',
              color: '#EAECEF',
            }}
          />
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div
          className="rounded-lg p-4"
          style={{ background: '#0B0E11', border: '1px solid #2B3139' }}
        >
          <div className="flex items-center gap-2 mb-2">
            <Scale className="w-4 h-4" style={{ color: '#0ECB81' }} />
            <label className="text-sm font-medium" style={{ color: '#EAECEF' }}>
              {ts(reverseStrategy.leverageScale, language)}
            </label>
          </div>
          <p className="text-xs mb-3" style={{ color: '#848E9C' }}>
            {ts(reverseStrategy.leverageScaleDesc, language)}
          </p>
          <div className="flex items-center gap-3">
            <input
              type="range"
              min={0.1}
              max={2}
              step={0.1}
              value={merged.leverage_scale ?? 1}
              onChange={(e) =>
                updateField('leverage_scale', parseDecimal(e.target.value, 1))
              }
              disabled={disabled || !merged.enabled}
              className="flex-1 accent-green-500"
            />
            <span
              className="w-12 text-center font-mono"
              style={{ color: '#0ECB81' }}
            >
              {(merged.leverage_scale ?? 1).toFixed(1)}x
            </span>
          </div>
        </div>

        <div
          className="rounded-lg p-4"
          style={{ background: '#0B0E11', border: '1px solid #2B3139' }}
        >
          <div className="flex items-center gap-2 mb-2">
            <Scale className="w-4 h-4" style={{ color: '#60a5fa' }} />
            <label className="text-sm font-medium" style={{ color: '#EAECEF' }}>
              {ts(reverseStrategy.positionScale, language)}
            </label>
          </div>
          <p className="text-xs mb-3" style={{ color: '#848E9C' }}>
            {ts(reverseStrategy.positionScaleDesc, language)}
          </p>
          <div className="flex items-center gap-3">
            <input
              type="range"
              min={0.1}
              max={1}
              step={0.1}
              value={merged.position_scale ?? 1}
              onChange={(e) =>
                updateField('position_scale', parseDecimal(e.target.value, 1))
              }
              disabled={disabled || !merged.enabled}
              className="flex-1 accent-blue-400"
            />
            <span
              className="w-12 text-center font-mono"
              style={{ color: '#60a5fa' }}
            >
              {(merged.position_scale ?? 1).toFixed(1)}x
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}
