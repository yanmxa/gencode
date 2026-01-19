/**
 * Model pricing database
 * Prices are per 1M tokens in USD
 * Updated: January 2025
 */

import { ModelPricing } from './types.js';

/**
 * Pricing data for all supported models
 * Source: Official provider pricing pages as of January 2025
 */
export const MODEL_PRICING: ModelPricing[] = [
  // Anthropic Claude Models
  {
    provider: 'anthropic',
    model: 'claude-opus-4-5',
    inputPer1M: 15.0,
    outputPer1M: 75.0,
    effectiveDate: '2025-01-01',
  },
  {
    provider: 'anthropic',
    model: 'claude-opus-4',
    inputPer1M: 15.0,
    outputPer1M: 75.0,
    effectiveDate: '2024-11-01',
  },
  {
    provider: 'anthropic',
    model: 'claude-sonnet-4-5',
    inputPer1M: 3.0,
    outputPer1M: 15.0,
    effectiveDate: '2025-01-01',
  },
  {
    provider: 'anthropic',
    model: 'claude-sonnet-4',
    inputPer1M: 3.0,
    outputPer1M: 15.0,
    effectiveDate: '2024-10-22',
  },
  {
    provider: 'anthropic',
    model: 'claude-3-5-sonnet-20241022',
    inputPer1M: 3.0,
    outputPer1M: 15.0,
    effectiveDate: '2024-10-22',
  },
  {
    provider: 'anthropic',
    model: 'claude-3-5-sonnet-20240620',
    inputPer1M: 3.0,
    outputPer1M: 15.0,
    effectiveDate: '2024-06-20',
  },
  {
    provider: 'anthropic',
    model: 'claude-haiku-3-5',
    inputPer1M: 0.8,
    outputPer1M: 4.0,
    effectiveDate: '2024-11-01',
  },
  {
    provider: 'anthropic',
    model: 'claude-3-5-haiku-20241022',
    inputPer1M: 0.8,
    outputPer1M: 4.0,
    effectiveDate: '2024-10-22',
  },
  {
    provider: 'anthropic',
    model: 'claude-3-haiku-20240307',
    inputPer1M: 0.25,
    outputPer1M: 1.25,
    effectiveDate: '2024-03-07',
  },
  {
    provider: 'anthropic',
    model: 'claude-3-opus-20240229',
    inputPer1M: 15.0,
    outputPer1M: 75.0,
    effectiveDate: '2024-02-29',
  },

  // OpenAI Models
  {
    provider: 'openai',
    model: 'gpt-4o',
    inputPer1M: 2.5,
    outputPer1M: 10.0,
    effectiveDate: '2024-08-06',
  },
  {
    provider: 'openai',
    model: 'gpt-4o-2024-11-20',
    inputPer1M: 2.5,
    outputPer1M: 10.0,
    effectiveDate: '2024-11-20',
  },
  {
    provider: 'openai',
    model: 'gpt-4o-2024-08-06',
    inputPer1M: 2.5,
    outputPer1M: 10.0,
    effectiveDate: '2024-08-06',
  },
  {
    provider: 'openai',
    model: 'gpt-4o-2024-05-13',
    inputPer1M: 5.0,
    outputPer1M: 15.0,
    effectiveDate: '2024-05-13',
  },
  {
    provider: 'openai',
    model: 'gpt-4o-mini',
    inputPer1M: 0.15,
    outputPer1M: 0.6,
    effectiveDate: '2024-07-18',
  },
  {
    provider: 'openai',
    model: 'gpt-4o-mini-2024-07-18',
    inputPer1M: 0.15,
    outputPer1M: 0.6,
    effectiveDate: '2024-07-18',
  },
  {
    provider: 'openai',
    model: 'gpt-4-turbo',
    inputPer1M: 10.0,
    outputPer1M: 30.0,
    effectiveDate: '2024-04-09',
  },
  {
    provider: 'openai',
    model: 'gpt-4-turbo-2024-04-09',
    inputPer1M: 10.0,
    outputPer1M: 30.0,
    effectiveDate: '2024-04-09',
  },
  {
    provider: 'openai',
    model: 'gpt-4',
    inputPer1M: 30.0,
    outputPer1M: 60.0,
    effectiveDate: '2023-03-14',
  },
  {
    provider: 'openai',
    model: 'gpt-3.5-turbo',
    inputPer1M: 0.5,
    outputPer1M: 1.5,
    effectiveDate: '2023-11-06',
  },
  {
    provider: 'openai',
    model: 'o1',
    inputPer1M: 15.0,
    outputPer1M: 60.0,
    effectiveDate: '2024-12-17',
  },
  {
    provider: 'openai',
    model: 'o1-2024-12-17',
    inputPer1M: 15.0,
    outputPer1M: 60.0,
    effectiveDate: '2024-12-17',
  },
  {
    provider: 'openai',
    model: 'o1-preview',
    inputPer1M: 15.0,
    outputPer1M: 60.0,
    effectiveDate: '2024-09-12',
  },
  {
    provider: 'openai',
    model: 'o1-preview-2024-09-12',
    inputPer1M: 15.0,
    outputPer1M: 60.0,
    effectiveDate: '2024-09-12',
  },
  {
    provider: 'openai',
    model: 'o1-mini',
    inputPer1M: 3.0,
    outputPer1M: 12.0,
    effectiveDate: '2024-09-12',
  },
  {
    provider: 'openai',
    model: 'o1-mini-2024-09-12',
    inputPer1M: 3.0,
    outputPer1M: 12.0,
    effectiveDate: '2024-09-12',
  },

  // Google Gemini Models
  {
    provider: 'gemini',
    model: 'gemini-2.0-flash-exp',
    inputPer1M: 0.0,
    outputPer1M: 0.0,
    effectiveDate: '2024-12-11',
  },
  {
    provider: 'gemini',
    model: 'gemini-2.0-flash',
    inputPer1M: 0.075,
    outputPer1M: 0.3,
    effectiveDate: '2025-01-01',
  },
  {
    provider: 'gemini',
    model: 'gemini-exp-1206',
    inputPer1M: 0.0,
    outputPer1M: 0.0,
    effectiveDate: '2024-12-06',
  },
  {
    provider: 'gemini',
    model: 'gemini-1.5-pro',
    inputPer1M: 1.25,
    outputPer1M: 5.0,
    effectiveDate: '2024-05-14',
  },
  {
    provider: 'gemini',
    model: 'gemini-1.5-pro-002',
    inputPer1M: 1.25,
    outputPer1M: 5.0,
    effectiveDate: '2024-09-24',
  },
  {
    provider: 'gemini',
    model: 'gemini-1.5-flash',
    inputPer1M: 0.075,
    outputPer1M: 0.3,
    effectiveDate: '2024-05-14',
  },
  {
    provider: 'gemini',
    model: 'gemini-1.5-flash-002',
    inputPer1M: 0.075,
    outputPer1M: 0.3,
    effectiveDate: '2024-09-24',
  },
  {
    provider: 'gemini',
    model: 'gemini-1.5-flash-8b',
    inputPer1M: 0.0375,
    outputPer1M: 0.15,
    effectiveDate: '2024-10-03',
  },

  // Google Vertex AI (same pricing as Gemini)
  {
    provider: 'vertex-ai',
    model: 'gemini-2.0-flash-exp',
    inputPer1M: 0.0,
    outputPer1M: 0.0,
    effectiveDate: '2024-12-11',
  },
  {
    provider: 'vertex-ai',
    model: 'gemini-2.0-flash',
    inputPer1M: 0.075,
    outputPer1M: 0.3,
    effectiveDate: '2025-01-01',
  },
  {
    provider: 'vertex-ai',
    model: 'gemini-exp-1206',
    inputPer1M: 0.0,
    outputPer1M: 0.0,
    effectiveDate: '2024-12-06',
  },
  {
    provider: 'vertex-ai',
    model: 'gemini-1.5-pro',
    inputPer1M: 1.25,
    outputPer1M: 5.0,
    effectiveDate: '2024-05-14',
  },
  {
    provider: 'vertex-ai',
    model: 'gemini-1.5-pro-002',
    inputPer1M: 1.25,
    outputPer1M: 5.0,
    effectiveDate: '2024-09-24',
  },
  {
    provider: 'vertex-ai',
    model: 'gemini-1.5-flash',
    inputPer1M: 0.075,
    outputPer1M: 0.3,
    effectiveDate: '2024-05-14',
  },
  {
    provider: 'vertex-ai',
    model: 'gemini-1.5-flash-002',
    inputPer1M: 0.075,
    outputPer1M: 0.3,
    effectiveDate: '2024-09-24',
  },
  {
    provider: 'vertex-ai',
    model: 'gemini-1.5-flash-8b',
    inputPer1M: 0.0375,
    outputPer1M: 0.15,
    effectiveDate: '2024-10-03',
  },
];

/**
 * Get pricing for a specific model
 */
export function getModelPricing(
  provider: string,
  model: string
): ModelPricing | undefined {
  return MODEL_PRICING.find(
    (p) => p.provider === provider && p.model === model
  );
}

/**
 * Get all pricing for a provider
 */
export function getProviderPricing(provider: string): ModelPricing[] {
  return MODEL_PRICING.filter((p) => p.provider === provider);
}
