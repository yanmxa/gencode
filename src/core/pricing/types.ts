/**
 * Pricing and cost tracking types
 */

/**
 * Token usage information
 */
export interface TokenUsage {
  inputTokens: number;
  outputTokens: number;
}

/**
 * Cost estimate in USD
 */
export interface CostEstimate {
  inputCost: number;
  outputCost: number;
  totalCost: number;
  currency: string; // 'USD'
}

/**
 * Model pricing per 1M tokens (USD)
 */
export interface ModelPricing {
  provider: string;
  model: string;
  inputPer1M: number; // USD per 1M input tokens
  outputPer1M: number; // USD per 1M output tokens
  effectiveDate?: string; // When this pricing became effective
}
