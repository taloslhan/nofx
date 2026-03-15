import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { StrategyStudioPage } from './StrategyStudioPage'

// Mock AuthContext
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({
    user: { id: 'test-user', email: 'test@test.com' },
    token: 'test-token',
    logout: vi.fn(),
    isLoading: false,
  }),
}))

// Mock LanguageContext
vi.mock('../contexts/LanguageContext', () => ({
  useLanguage: () => ({
    language: 'en',
    setLanguage: vi.fn(),
  }),
}))

// Mock notify
vi.mock('../lib/notify', () => ({
  confirmToast: vi.fn(),
  notify: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
  },
}))

// Default mock strategy data
const mockStrategy = {
  id: 'strat-1',
  name: 'Test Strategy',
  description: 'A test strategy',
  is_active: true,
  is_default: false,
  is_public: false,
  config_visible: true,
  config: {
    strategy_type: 'ai_trading',
    coin_source: {
      source_type: 'static',
      static_coins: ['BTCUSDT'],
      use_ai500: false,
      use_oi_top: false,
      use_oi_low: false,
    },
    indicators: {
      klines: {
        primary_timeframe: '1h',
        primary_count: 24,
        enable_multi_timeframe: false,
      },
      enable_raw_klines: true,
      enable_ema: false,
      enable_macd: false,
      enable_rsi: false,
      enable_atr: false,
      enable_boll: false,
      enable_volume: false,
      enable_oi: false,
      enable_funding_rate: false,
    },
    risk_control: {
      max_positions: 3,
      btc_eth_max_leverage: 10,
      altcoin_max_leverage: 5,
      max_margin_usage: 0.9,
      min_position_size: 10,
      min_risk_reward_ratio: 1.5,
      min_confidence: 0.7,
    },
  },
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

describe('StrategyStudioPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('renders loading state initially', () => {
    // Mock fetch to never resolve (keeps loading)
    global.fetch = vi.fn(() => new Promise(() => {}))

    const { container } = render(<StrategyStudioPage />)
    // Should show spinner (loading state)
    expect(container.querySelector('.animate-spin')).toBeTruthy()
  })

  it('renders strategy page after successful API load', async () => {
    global.fetch = vi.fn((url: string) => {
      if (url.includes('/api/strategies')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ strategies: [mockStrategy] }),
        })
      }
      if (url.includes('/api/models')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve([]),
        })
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({}),
      })
    }) as unknown as typeof fetch

    render(<StrategyStudioPage />)

    await waitFor(() => {
      // Strategy name should be visible
      expect(screen.getByDisplayValue('Test Strategy')).toBeTruthy()
    })
  })

  it('renders without crash when API returns empty strategies', async () => {
    global.fetch = vi.fn((url: string) => {
      if (url.includes('/api/strategies')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ strategies: [] }),
        })
      }
      if (url.includes('/api/models')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve([]),
        })
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({}),
      })
    }) as unknown as typeof fetch

    const { container } = render(<StrategyStudioPage />)

    await waitFor(() => {
      // Loading should finish
      expect(container.querySelector('.animate-spin')).toBeFalsy()
    })
  })

  it('renders without crash when API returns error', async () => {
    global.fetch = vi.fn(() =>
      Promise.resolve({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ error: 'Internal Server Error' }),
      })
    ) as unknown as typeof fetch

    const { container } = render(<StrategyStudioPage />)

    await waitFor(() => {
      // Loading should finish (finally block)
      expect(container.querySelector('.animate-spin')).toBeFalsy()
    })
  })

  it('renders without crash when API throws network error', async () => {
    global.fetch = vi.fn(() =>
      Promise.reject(new Error('Network error'))
    ) as unknown as typeof fetch

    const { container } = render(<StrategyStudioPage />)

    await waitFor(() => {
      expect(container.querySelector('.animate-spin')).toBeFalsy()
    })
  })

  it('renders without crash when strategy config has null/undefined fields', async () => {
    const badStrategy = {
      ...mockStrategy,
      config: {
        // Missing strategy_type, coin_source, indicators, risk_control
      },
    }

    global.fetch = vi.fn((url: string) => {
      if (url.includes('/api/strategies')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ strategies: [badStrategy] }),
        })
      }
      if (url.includes('/api/models')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve([]),
        })
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({}),
      })
    }) as unknown as typeof fetch

    // This should NOT crash
    const { container } = render(<StrategyStudioPage />)

    await waitFor(() => {
      expect(container.querySelector('.animate-spin')).toBeFalsy()
    })
  })

  it('does not get stuck in loading state when token is available', async () => {
    global.fetch = vi.fn((url: string) => {
      if (url.includes('/api/strategies')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ strategies: [] }),
        })
      }
      if (url.includes('/api/models')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve([]),
        })
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({}),
      })
    }) as unknown as typeof fetch

    const { container } = render(<StrategyStudioPage />)

    // Wait a reasonable time
    await waitFor(
      () => {
        expect(container.querySelector('.animate-spin')).toBeFalsy()
      },
      { timeout: 3000 }
    )
  })
})
