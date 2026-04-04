import { describe, expect, it } from 'vitest';
import { applyManualProbeResult, buildModelHealthGroups } from './model-health-page';

describe('Task 12 Model Health Page', () => {
  it('builds grouped-by-display-model layout', () => {
    const groups = buildModelHealthGroups([
      {
        displayModel: 'gpt-4o',
        channelID: 'Q2hhbm5lbDox',
        actualModelID: 'gpt-4o-2024-11-20',
        isHealthy: true,
        manualOverride: false,
        probedAt: 1712310000,
      },
      {
        displayModel: 'gpt-4o-mini',
        channelID: 'Q2hhbm5lbDoy',
        actualModelID: 'gpt-4o-mini-2024-07-18',
        isHealthy: false,
        manualOverride: false,
        probedAt: 1712310060,
      },
    ]);

    expect(groups).toHaveLength(2);
    expect(groups[0].displayModel).toBe('gpt-4o');
    expect(groups[0].rows[0]).toMatchObject({
      actualModelID: 'gpt-4o-2024-11-20',
      channelID: 'Q2hhbm5lbDox',
    });
  });

  it('rows show actual model ID and channel', () => {
    const groups = buildModelHealthGroups([
      {
        displayModel: 'gpt-4o',
        channelID: 'Q2hhbm5lbDox',
        actualModelID: 'gpt-4o-2024-11-20',
        isHealthy: true,
        manualOverride: false,
        probedAt: 1712310000,
      },
    ]);

    expect(groups[0].rows[0].actualModelID).toBe('gpt-4o-2024-11-20');
    expect(groups[0].rows[0].channelID).toBe('Q2hhbm5lbDox');
  });

  it('manual probe action updates visible status', () => {
    const result = applyManualProbeResult(
      [
        {
          displayModel: 'gpt-4o',
          channelID: 'Q2hhbm5lbDox',
          actualModelID: 'gpt-4o-2024-11-20',
          isHealthy: false,
          manualOverride: false,
          probedAt: 1712310000,
        },
      ],
      {
        displayModel: 'gpt-4o',
        channelID: 'Q2hhbm5lbDox',
        actualModelID: 'gpt-4o-2024-11-20',
      }
    );

    expect(result[0]).toMatchObject({
      isHealthy: true,
      manualOverride: true,
    });
  });
});
