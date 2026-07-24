// Model token cost estimation utility ($/1M tokens)
export const MODEL_PRICING: Record<string, { inputPerM: number; outputPerM: number }> = {
  'claude-opus-4-8': { inputPerM: 15.0, outputPerM: 75.0 },
  'claude-sonnet-4-5': { inputPerM: 3.0, outputPerM: 15.0 },
  'claude-haiku-4-5': { inputPerM: 0.25, outputPerM: 1.25 },
  'gemini-3.6-flash-high': { inputPerM: 0.35, outputPerM: 1.05 },
  'gemini-3.1-pro-high': { inputPerM: 1.25, outputPerM: 3.75 },
};

export function calculateEstimatedCostUSD(tokens: number, model: string = 'claude-sonnet-4-5'): string {
  const p = MODEL_PRICING[model] || MODEL_PRICING['claude-sonnet-4-5'];
  const avgRatePerM = (p.inputPerM + p.outputPerM) / 2;
  const cost = (tokens / 1000000) * avgRatePerM;
  if (cost < 0.01) return '< $0.01';
  return `$${cost.toFixed(2)}`;
}
