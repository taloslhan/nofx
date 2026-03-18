import type { TestModelConnectionResponse } from '../../types'
import type { Language } from '../../i18n/translations'
import { t } from '../../i18n/translations'

export type ConnectionTestStatus = 'idle' | 'testing' | 'success' | 'error'

export interface ConnectionTestState {
  status: ConnectionTestStatus
  result: TestModelConnectionResponse | null
}

interface ConnectionTestFeedbackProps {
  state: ConnectionTestState
  language: Language
  compact?: boolean
}

export function ConnectionTestFeedback({
  state,
  language,
  compact = false,
}: ConnectionTestFeedbackProps) {
  if (state.status === 'idle') {
    return null
  }

  const isTesting = state.status === 'testing'
  const isSuccess = state.status === 'success'
  const accent = isTesting ? '#60A5FA' : isSuccess ? '#00E096' : '#F6465D'
  const background = isTesting
    ? 'rgba(96, 165, 250, 0.12)'
    : isSuccess
      ? 'rgba(0, 224, 150, 0.12)'
      : 'rgba(246, 70, 93, 0.12)'
  const border = isTesting
    ? '1px solid rgba(96, 165, 250, 0.3)'
    : isSuccess
      ? '1px solid rgba(0, 224, 150, 0.3)'
      : '1px solid rgba(246, 70, 93, 0.3)'
  const title = isTesting
    ? t('testingConnection', language)
    : isSuccess
      ? t('testSuccess', language)
      : t('testFailed', language)
  const detail = isTesting
    ? ''
    : isSuccess
      ? state.result?.model
        ? `${state.result.message || 'OK'} · ${state.result.model}`
        : state.result?.message || 'OK'
      : state.result?.error || t('testFailed', language)

  return (
    <div
      className={`rounded-xl flex items-center justify-between gap-3 ${
        compact ? 'px-3 py-2' : 'px-4 py-3'
      }`}
      style={{ background, border }}
    >
      <div className="flex items-center gap-3 min-w-0">
        <div
          className="w-5 h-5 rounded-full flex items-center justify-center shrink-0"
          style={{ background: `${accent}22`, color: accent }}
        >
          {isTesting ? (
            <div className="w-2.5 h-2.5 rounded-full border-2 border-current border-t-transparent animate-spin" />
          ) : isSuccess ? (
            <span className="text-xs">✓</span>
          ) : (
            <span className="text-xs">!</span>
          )}
        </div>
        <div className="min-w-0">
          <div
            className={
              compact ? 'text-xs font-semibold' : 'text-sm font-semibold'
            }
            style={{ color: accent }}
          >
            {title}
          </div>
          {!isTesting && (
            <div
              className={`truncate ${compact ? 'text-[11px]' : 'text-xs'}`}
              style={{ color: '#C5CFD8' }}
            >
              {detail}
            </div>
          )}
        </div>
      </div>
      {!isTesting &&
        typeof state.result?.latency_ms === 'number' &&
        state.result.latency_ms > 0 && (
          <div
            className={compact ? 'text-[11px] shrink-0' : 'text-xs shrink-0'}
            style={{ color: '#A0AEC0' }}
          >
            {t('testLatency', language, { ms: state.result.latency_ms })}
          </div>
        )}
    </div>
  )
}
