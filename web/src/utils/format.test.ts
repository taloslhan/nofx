import { afterEach, describe, expect, it, vi } from 'vitest'
import { formatChartTimestamp, utcToChartTimestamp } from './format'

describe('format utils', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('utcToChartTimestamp should apply timezone offset from the target timestamp', () => {
    const getTimezoneOffsetMock = vi
      .spyOn(Date.prototype, 'getTimezoneOffset')
      .mockReturnValue(-480)

    expect(utcToChartTimestamp(Date.UTC(2026, 2, 19, 12, 0, 0))).toBe(1773950400)
    expect(getTimezoneOffsetMock).toHaveBeenCalled()
  })

  it('formatChartTimestamp should avoid applying local timezone twice', () => {
    expect(
      formatChartTimestamp(1742414400, 'en-US', {
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false,
      })
    ).toBe('03/19, 20:00')
  })
})
