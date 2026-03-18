import type { TraderInfo } from '../../types'

export interface ModelUsageInfo {
  runningCount: number
  totalCount: number
  usingTraders: TraderInfo[]
}

export function traderUsesModel(
  trader: Pick<TraderInfo, 'ai_model' | 'fallback_ai_model'>,
  modelId: string
): boolean {
  return trader.ai_model === modelId || trader.fallback_ai_model === modelId
}

export function getTradersUsingModel(
  traders: TraderInfo[] | undefined,
  modelId: string
): TraderInfo[] {
  return traders?.filter((trader) => traderUsesModel(trader, modelId)) ?? []
}

export function getModelUsageInfo(
  traders: TraderInfo[] | undefined,
  modelId: string
): ModelUsageInfo {
  const usingTraders = getTradersUsingModel(traders, modelId)

  return {
    runningCount: usingTraders.filter((trader) => trader.is_running).length,
    totalCount: usingTraders.length,
    usingTraders,
  }
}

export function isModelInUse(
  traders: TraderInfo[] | undefined,
  modelId: string
): boolean {
  return (
    traders?.some(
      (trader) => traderUsesModel(trader, modelId) && Boolean(trader.is_running)
    ) ?? false
  )
}

export function isModelUsedByAnyTrader(
  traders: TraderInfo[] | undefined,
  modelId: string
): boolean {
  return getTradersUsingModel(traders, modelId).length > 0
}
