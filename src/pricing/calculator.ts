/**
 * Cost calculation utilities
 */

import { getModelPricing } from './models.js';
import { CostEstimate, TokenUsage } from './types.js';

/**
 * Calculate cost from token usage
 */
export function calculateCost(
  provider: string,
  model: string,
  tokens: TokenUsage
): CostEstimate {
  const pricing = getModelPricing(provider, model);

  // If no pricing found, return zero cost
  if (!pricing) {
    return {
      inputCost: 0,
      outputCost: 0,
      totalCost: 0,
      currency: 'USD',
    };
  }

  // Calculate cost per token type
  const inputCost = (tokens.inputTokens / 1_000_000) * pricing.inputPer1M;
  const outputCost = (tokens.outputTokens / 1_000_000) * pricing.outputPer1M;

  return {
    inputCost,
    outputCost,
    totalCost: inputCost + outputCost,
    currency: 'USD',
  };
}

/**
 * Format cost for display
 */
export function formatCost(cost: number): string {
  if (cost === 0) {
    return '$0.00';
  }
  if (cost < 0.01) {
    return '<$0.01';
  }
  return `$${cost.toFixed(2)}`;
}

/**
 * Format token count for display
 */
export function formatTokens(count: number): string {
  if (count >= 1_000_000) {
    return `${(count / 1_000_000).toFixed(1)}M`;
  }
  if (count >= 1_000) {
    return `${(count / 1_000).toFixed(1)}K`;
  }
  return count.toString();
}

/**
 * Format cost estimate for display
 */
export function formatCostEstimate(estimate: CostEstimate): string {
  return formatCost(estimate.totalCost);
}
