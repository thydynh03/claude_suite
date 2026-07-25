import { describe, expect, it } from 'vitest'
import { MODEL_PRICING, calculateEstimatedCostUSD } from './pricing'

describe('calculateEstimatedCostUSD', () => {
  // The estimate averages the input and output rates, because the caller only
  // has a single token total and no split between the two.
  it('averages the input and output rate for the model', () => {
    const p = MODEL_PRICING['claude-opus-4-8']
    const expected = (2_000_000 / 1_000_000) * ((p.inputPerM + p.outputPerM) / 2)

    expect(calculateEstimatedCostUSD(2_000_000, 'claude-opus-4-8')).toBe(`$${expected.toFixed(2)}`)
  })

  it('charges less for a cheaper model', () => {
    const opus = calculateEstimatedCostUSD(5_000_000, 'claude-opus-4-8')
    const haiku = calculateEstimatedCostUSD(5_000_000, 'claude-haiku-4-5')

    expect(parseFloat(opus.replace('$', ''))).toBeGreaterThan(parseFloat(haiku.replace('$', '')))
  })

  it('falls back to Sonnet pricing for a model it does not know', () => {
    expect(calculateEstimatedCostUSD(2_000_000, 'some-unreleased-model')).toBe(
      calculateEstimatedCostUSD(2_000_000, 'claude-sonnet-4-5'),
    )
  })

  it('defaults to Sonnet when no model is given', () => {
    expect(calculateEstimatedCostUSD(2_000_000)).toBe(
      calculateEstimatedCostUSD(2_000_000, 'claude-sonnet-4-5'),
    )
  })

  // Anything under a cent reads as "< $0.01" rather than "$0.00", so a small run
  // does not look free.
  it('reports amounts below a cent as a threshold', () => {
    expect(calculateEstimatedCostUSD(100, 'claude-haiku-4-5')).toBe('< $0.01')
    expect(calculateEstimatedCostUSD(0)).toBe('< $0.01')
  })

  it('always formats to two decimal places', () => {
    expect(calculateEstimatedCostUSD(1_000_000, 'claude-sonnet-4-5')).toMatch(/^\$\d+\.\d{2}$/)
  })
})

describe('MODEL_PRICING', () => {
  it('quotes a positive rate for every model, with output dearer than input', () => {
    for (const [model, price] of Object.entries(MODEL_PRICING)) {
      expect(price.inputPerM, `${model} input rate`).toBeGreaterThan(0)
      expect(price.outputPerM, `${model} output rate`).toBeGreaterThan(price.inputPerM)
    }
  })
})
