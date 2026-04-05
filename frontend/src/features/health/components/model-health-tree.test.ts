import { describe, expect, it } from 'vitest';
import { getLatestProbeMeta } from './model-health-tree';

describe('ModelHealthTree', () => {
  it('prefers latest history over snapshot for probe meta', () => {
    expect(
      getLatestProbeMeta(
        {
          isHealthy: true,
          probedAt: 100,
        },
        [
          {
            id: 'h1',
            displayModel: 'gpt-5.4',
            channelID: 'c1',
            actualModelID: 'gpt-5.4-high',
            isHealthy: false,
            manualOverride: true,
            probedAt: 200,
          },
        ]
      )
    ).toEqual({
      isHealthy: false,
      probedAt: 200,
    });
  });

  it('falls back to snapshot when history is empty', () => {
    expect(
      getLatestProbeMeta({
        isHealthy: true,
        probedAt: 100,
      })
    ).toEqual({
      isHealthy: true,
      probedAt: 100,
    });
  });
});
