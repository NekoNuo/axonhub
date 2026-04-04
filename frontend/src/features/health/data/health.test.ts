import { describe, expect, it } from 'vitest';
import { parseModelHealthHistory, parseModelHealthSnapshots, buildManualModelProbeVariables } from './health';
import { modelHealthHistorySchema, modelHealthSnapshotSchema } from './schema';

describe('Task 8 Model Health Data', () => {
  it('parses model health rows and history', () => {
    const snapshots = parseModelHealthSnapshots([
      {
        displayModel: 'gpt-4o',
        channelID: 'Q2hhbm5lbDox',
        actualModelID: 'gpt-4o-2024-11-20',
        isHealthy: true,
        manualOverride: false,
        probedAt: 1712310000,
      },
    ]);

    const history = parseModelHealthHistory([
      {
        displayModel: 'gpt-4o',
        channelID: 'Q2hhbm5lbDox',
        actualModelID: 'gpt-4o-2024-11-20',
        isHealthy: false,
        manualOverride: true,
        probedAt: 1712310300,
      },
    ]);

    expect(modelHealthSnapshotSchema.array().parse(snapshots)[0].actualModelID).toBe('gpt-4o-2024-11-20');
    expect(modelHealthHistorySchema.array().parse(history)[0].manualOverride).toBe(true);
  });

  it('parses enableModelProbe from system settings', async () => {
    const { modelSettingsSchema } = await import('@/features/health/data/schema');

    const parsed = modelSettingsSchema.parse({
      enableModelProbe: true,
    });

    expect(parsed.enableModelProbe).toBe(true);
  });

  it('builds manual probe mutation payloads', () => {
    expect(
      buildManualModelProbeVariables({
        displayModel: 'gpt-4o',
        actualModelID: 'gpt-4o-2024-11-20',
        channelID: 'Q2hhbm5lbDox',
      })
    ).toEqual({
      input: {
        displayModel: 'gpt-4o',
        actualModelID: 'gpt-4o-2024-11-20',
        channelID: 'Q2hhbm5lbDox',
      },
    });
  });
});
