import { describe, expect, it } from 'vitest';
import { buildChannelHealthRows, getLatestSnapshotDetails } from './channel-health-page';
import { formatHealthTimestamp } from './channel-health-format';

describe('Task 11 Channel Health Page', () => {
  it('channel health page renders probe bars', () => {
    const rows = buildChannelHealthRows(
      [
        {
          id: 'Q2hhbm5lbDox',
          name: 'Primary OpenAI',
        },
      ],
      [
        {
          channelID: 'Q2hhbm5lbDox',
          points: [
            {
              timestamp: 1712310000,
              totalRequestCount: 10,
              successRequestCount: 10,
              avgTokensPerSecond: 42,
              avgTimeToFirstTokenMs: 120,
              activeProbeLatencyMs: 180,
              probeModelLatencyMs: 220,
            },
          ],
        },
      ],
      []
    );

    expect(rows).toHaveLength(1);
    expect(rows[0].points).toHaveLength(1);
  });

  it('page shows latest snapshot details', () => {
    expect(
      getLatestSnapshotDetails({
        probeTimestamp: 1712310300,
        activeProbeLatencyMs: 180,
        probeModelLatencyMs: 220,
        observedTimestamp: 1712310360,
        observedLatencyMs: 150,
      })
    ).toEqual({
      probeTimestamp: 1712310300,
      activeProbeLatencyMs: 180,
      probeModelLatencyMs: 220,
      observedTimestamp: 1712310360,
      observedLatencyMs: 150,
    });
  });

  it('formats probe timestamps for display', () => {
    expect(formatHealthTimestamp(1712310300, 'en-US')).toMatch(/2024/);
  });
});
