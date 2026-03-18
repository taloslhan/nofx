import { describe, expect, it } from 'vitest'
import {
  getModelUsageInfo,
  getTradersUsingModel,
  isModelInUse,
  isModelUsedByAnyTrader,
  traderUsesModel,
} from './model-usage'
import type { TraderInfo } from '../../types'

describe('model usage helpers', () => {
  const traders: TraderInfo[] = [
    {
      trader_id: 'trader-primary',
      trader_name: 'Primary Trader',
      ai_model: 'gpt-4',
      is_running: true,
    },
    {
      trader_id: 'trader-fallback',
      trader_name: 'Fallback Trader',
      ai_model: 'claude-3',
      fallback_ai_model: 'gpt-4',
      is_running: false,
    },
    {
      trader_id: 'trader-other',
      trader_name: 'Other Trader',
      ai_model: 'deepseek',
      fallback_ai_model: 'claude-3',
      is_running: true,
    },
  ]

  it('matches trader when model is primary', () => {
    expect(traderUsesModel(traders[0], 'gpt-4')).toBe(true)
  })

  it('matches trader when model is fallback', () => {
    expect(traderUsesModel(traders[1], 'gpt-4')).toBe(true)
  })

  it('collects traders using a model across primary and fallback roles', () => {
    expect(
      getTradersUsingModel(traders, 'gpt-4').map((trader) => trader.trader_id)
    ).toEqual(['trader-primary', 'trader-fallback'])
  })

  it('counts running and total usage across primary and fallback roles', () => {
    expect(getModelUsageInfo(traders, 'gpt-4')).toEqual({
      runningCount: 1,
      totalCount: 2,
      usingTraders: [traders[0], traders[1]],
    })
  })

  it('treats fallback references as used even when not running', () => {
    expect(isModelUsedByAnyTrader(traders, 'claude-3')).toBe(true)
  })

  it('reports model in use when any matching trader is running', () => {
    expect(isModelInUse(traders, 'gpt-4')).toBe(true)
    expect(isModelInUse(traders, 'claude-3')).toBe(true)
  })
})
