import { describe, expect, it } from 'vitest';
import {
  buildAssociatedModelHealthRows,
  buildModelHealthTree,
  collectVisibleChannelTargets,
  collectVisiblePageProbeTargets,
  buildDiscoveredModelHealthRows,
  getDefaultProbeEnabled,
  runProbeTargetsWithLimit,
  getActualModelHistory,
  getChannelProbeTargets,
  getDisplayModelHistory,
  getModelsPendingConnectionQuery,
  getProbeEnabledModelEntries,
  getRowProbeTarget,
  getGroupProbeTargets,
} from './model-health-page';

describe('Task 12 Model Health Page', () => {
  it('builds grouped-by-display-model and channel layout', () => {
    const groups = buildModelHealthTree([
      {
        channelName: 'Channel A',
        channelStatus: 'enabled',
        channelType: 'openai',
        orderingWeight: 10,
        priority: 1,
        displayModel: 'gpt-4o',
        channelID: 'Q2hhbm5lbDox',
        actualModelID: 'gpt-4o-2024-11-20',
        isHealthy: true,
        manualOverride: false,
        probedAt: 1712310000,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
      },
      {
        channelName: 'Channel A',
        channelStatus: 'enabled',
        channelType: 'openai',
        orderingWeight: 10,
        priority: 1,
        displayModel: 'gpt-4o',
        channelID: 'Q2hhbm5lbDox',
        actualModelID: 'gpt-4o-2024-08-06',
        isHealthy: false,
        manualOverride: false,
        probedAt: 1712310300,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
      },
      {
        channelName: 'Channel B',
        channelStatus: 'enabled',
        channelType: 'openai',
        orderingWeight: 5,
        priority: 2,
        displayModel: 'gpt-4o-mini',
        channelID: 'Q2hhbm5lbDoy',
        actualModelID: 'gpt-4o-mini-2024-07-18',
        isHealthy: false,
        manualOverride: false,
        probedAt: 1712310060,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
      },
    ]);

    expect(groups).toHaveLength(2);
    expect(groups[0].displayModel).toBe('gpt-4o');
    expect(groups[0].channels).toHaveLength(1);
    expect(groups[0].channels[0].rows[0]).toMatchObject({
      actualModelID: 'gpt-4o-2024-08-06',
      channelID: 'Q2hhbm5lbDox',
    });
  });

  it('rows show actual model ID and channel', () => {
    const groups = buildModelHealthTree([
      {
        channelName: 'Channel A',
        channelStatus: 'enabled',
        channelType: 'openai',
        orderingWeight: 10,
        priority: 1,
        displayModel: 'gpt-4o',
        channelID: 'Q2hhbm5lbDox',
        actualModelID: 'gpt-4o-2024-11-20',
        isHealthy: true,
        manualOverride: false,
        probedAt: 1712310000,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
      },
    ]);

    expect(groups[0].channels[0].rows[0].actualModelID).toBe('gpt-4o-2024-11-20');
    expect(groups[0].channels[0].rows[0].channelID).toBe('Q2hhbm5lbDox');
  });

  it('skips connection queries for models already cached or in flight', () => {
    const pending = getModelsPendingConnectionQuery(
      [
        {
          modelID: 'gemini-3.1-flash-lite-preview',
          name: 'Gemini Flash Lite',
          settings: {
            associations: [{ type: 'model', disabled: false }],
          },
        },
        {
          modelID: 'gemini-3.1-flash',
          name: 'Gemini Flash',
          settings: {
            associations: [{ type: 'regex', disabled: false }],
          },
        },
        {
          modelID: 'gpt-4o',
          name: 'GPT-4o',
          settings: {
            associations: [{ type: 'model', disabled: false }],
          },
        },
      ],
      '',
      {
        'gemini-3.1-flash-lite-preview': [],
      },
      new Set(['gemini-3.1-flash'])
    );

    expect(pending.map((model) => model.modelID)).toEqual(['gpt-4o']);
  });

  it('only includes probe-enabled models', () => {
    const models = getProbeEnabledModelEntries([
      {
        modelID: 'gpt-4o',
        name: 'GPT-4o',
        settings: {
          probeEnabled: true,
          associations: [{ type: 'model', disabled: false }],
        },
      },
      {
        modelID: 'gpt-4o-mini',
        name: 'GPT-4o Mini',
        settings: {
          probeEnabled: false,
          associations: [{ type: 'model', disabled: false }],
        },
      },
      {
        modelID: 'gemini-2.5-flash',
        name: 'Gemini 2.5 Flash',
        settings: {
          probeEnabled: true,
          associations: [],
        },
      },
    ]);

    expect(models.map((model) => model.modelID)).toEqual(['gpt-4o']);
  });

  it('builds channel probe targets from grouped rows', () => {
    const [channelGroup] = buildModelHealthTree([
      {
        channelName: 'Channel B',
        channelStatus: 'enabled',
        channelType: 'openai',
        orderingWeight: 50,
        priority: 1,
        displayModel: 'gpt-4o',
        channelID: 'Q2hhbm5lbDoy',
        actualModelID: 'gpt-4o-2024-11-20',
        isHealthy: true,
        manualOverride: false,
        probedAt: 1712310000,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
      },
      {
        channelName: 'Channel B',
        channelStatus: 'enabled',
        channelType: 'openai',
        orderingWeight: 50,
        priority: 1,
        displayModel: 'gpt-4o',
        channelID: 'Q2hhbm5lbDoy',
        actualModelID: 'gpt-4o-2024-08-06',
        isHealthy: false,
        manualOverride: false,
        probedAt: 1712310300,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
      },
    ]).flatMap((group) => group.channels);

    expect(getChannelProbeTargets(channelGroup)).toEqual([
      {
        displayModel: 'gpt-4o',
        channelID: 'Q2hhbm5lbDoy',
        actualModelID: 'gpt-4o-2024-08-06',
      },
      {
        displayModel: 'gpt-4o',
        channelID: 'Q2hhbm5lbDoy',
        actualModelID: 'gpt-4o-2024-11-20',
      },
    ]);
  });

  it('builds model-level and actual-model-level histories from all rows', () => {
    const rows = [
      {
        channelName: 'Channel A',
        channelStatus: 'enabled',
        channelType: 'openai',
        orderingWeight: 10,
        priority: 1,
        displayModel: 'gpt-5.4',
        channelID: 'Q2hhbm5lbDox',
        actualModelID: 'gpt-5.4-high',
        isHealthy: true,
        manualOverride: false,
        probedAt: 1712310000,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
      },
      {
        channelName: 'Channel A',
        channelStatus: 'enabled',
        channelType: 'openai',
        orderingWeight: 10,
        priority: 2,
        displayModel: 'gpt-5.4',
        channelID: 'Q2hhbm5lbDox',
        actualModelID: 'gpt-5.4-low',
        isHealthy: false,
        manualOverride: false,
        probedAt: 1712310300,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
      },
    ];
    const groups = buildModelHealthTree(rows);
    const histories = {
      'gpt-5.4:Q2hhbm5lbDox:gpt-5.4-high': [
        {
          id: 'h1',
          displayModel: 'gpt-5.4',
          channelID: 'Q2hhbm5lbDox',
          actualModelID: 'gpt-5.4-high',
          isHealthy: true,
          manualOverride: false,
          probedAt: 1712310000,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
        },
      ],
      'gpt-5.4:Q2hhbm5lbDox:gpt-5.4-low': [
        {
          id: 'h2',
          displayModel: 'gpt-5.4',
          channelID: 'Q2hhbm5lbDox',
          actualModelID: 'gpt-5.4-low',
          isHealthy: false,
          manualOverride: false,
          probedAt: 1712310300,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
        },
      ],
    };

    expect(getDisplayModelHistory(groups[0].channels[0], histories).map((item) => item.actualModelID)).toEqual(['gpt-5.4-low', 'gpt-5.4-high']);
    expect(getActualModelHistory(groups[0].channels[0].rows[0], histories)[0].actualModelID).toBe('gpt-5.4-high');
  });

  it('builds row probe target', () => {
    expect(
      getRowProbeTarget({
        displayModel: 'gpt-4o',
        channelID: 'Q2hhbm5lbDox',
        actualModelID: 'gpt-4o-2024-11-20',
      } as any)
    ).toEqual({
      displayModel: 'gpt-4o',
      channelID: 'Q2hhbm5lbDox',
      actualModelID: 'gpt-4o-2024-11-20',
    });
  });

  it('limits manual probe concurrency', async () => {
    const targets = [
      { displayModel: 'gpt-4o', channelID: 'Q2hhbm5lbDox', actualModelID: 'm1' },
      { displayModel: 'gpt-4o', channelID: 'Q2hhbm5lbDox', actualModelID: 'm2' },
      { displayModel: 'gpt-4o', channelID: 'Q2hhbm5lbDox', actualModelID: 'm3' },
      { displayModel: 'gpt-4o', channelID: 'Q2hhbm5lbDox', actualModelID: 'm4' },
    ];
    let active = 0;
    let maxActive = 0;
    const started: string[] = [];

    await runProbeTargetsWithLimit(targets, 2, async (target) => {
      started.push(target.actualModelID);
      active++;
      maxActive = Math.max(maxActive, active);
      await new Promise((resolve) => setTimeout(resolve, 10));
      active--;
    });

    expect(maxActive).toBe(2);
    expect(started).toHaveLength(4);
  });

  it('builds discovered rows directly from snapshots without model associations', () => {
    const rows = buildDiscoveredModelHealthRows(
      [
        {
          displayModel: 'gpt-5-2',
          channelID: 'Q2hhbm5lbDox',
          actualModelID: 'gpt-5-2',
          isHealthy: true,
          manualOverride: false,
          probedAt: 1712310000,
        },
      ],
      new Map([
        [
          'Q2hhbm5lbDox',
          {
            name: 'Channel A',
            status: 'enabled',
            type: 'openai',
            orderingWeight: 100,
          },
        ],
      ]),
      new Map()
    );

    expect(rows).toEqual([
      expect.objectContaining({
        displayModel: 'gpt-5-2',
        channelName: 'Channel A',
        actualModelID: 'gpt-5-2',
      }),
    ]);
  });

  it('defaults probing to enabled when config is missing on enabled channels', () => {
    expect(getDefaultProbeEnabled(undefined, 'enabled')).toBe(true);
  });

  it('defaults probing to disabled when config is missing on disabled channels', () => {
    expect(getDefaultProbeEnabled(undefined, 'disabled')).toBe(false);
  });

  it('prefers explicit config over channel status for default probing', () => {
    expect(getDefaultProbeEnabled(false, 'enabled')).toBe(false);
    expect(getDefaultProbeEnabled(true, 'disabled')).toBe(true);
  });

  it('rebuilds associated rows from the latest probe config state', () => {
    const modelEntries = [
      {
        modelID: 'gpt-4o',
        name: 'GPT-4o',
        settings: {
          probeEnabled: true,
          associations: [{ type: 'model', disabled: false }],
        },
      },
    ];
    const connectionsByModel = {
      'gpt-4o': [
        {
          channel: {
            id: 'channel-a',
            name: 'Channel A',
            status: 'enabled',
            type: 'openai',
            orderingWeight: 100,
          },
          priority: 1,
          models: [{ actualModel: 'gpt-4o-2024-11-20' }],
        },
      ],
    };
    const channelsByID = new Map([
      [
        'channel-a',
        {
          name: 'Channel A',
          status: 'enabled',
          type: 'openai',
          orderingWeight: 100,
        },
      ],
    ]);

    const defaultRows = buildAssociatedModelHealthRows(
      modelEntries as any,
      connectionsByModel as any,
      [],
      channelsByID,
      new Map()
    );
    const updatedRows = buildAssociatedModelHealthRows(
      modelEntries as any,
      connectionsByModel as any,
      [],
      channelsByID,
      new Map([
        [
          'gpt-4o:channel-a:gpt-4o-2024-11-20',
          {
            displayModel: 'gpt-4o',
            channelID: 'channel-a',
            actualModelID: 'gpt-4o-2024-11-20',
            probeEnabled: false,
            consecutiveFailures: 0,
            autoDisabledAt: null,
          },
        ],
      ])
    );

    expect(defaultRows[0].probeEnabled).toBe(true);
    expect(updatedRows[0].probeEnabled).toBe(false);
  });

  it('collects visible channels for batch probe toggles', () => {
    const groups = buildModelHealthTree([
      {
        channelName: 'Channel A',
        channelStatus: 'enabled',
        channelType: 'openai',
        orderingWeight: 10,
        priority: 1,
        displayModel: 'gpt-4o',
        channelID: 'channel-a',
        actualModelID: 'gpt-4o-2024-11-20',
        isHealthy: true,
        manualOverride: false,
        probedAt: 1712310000,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
      },
      {
        channelName: 'Channel B',
        channelStatus: 'disabled',
        channelType: 'openai',
        orderingWeight: 5,
        priority: 1,
        displayModel: 'gpt-4o',
        channelID: 'channel-b',
        actualModelID: 'gpt-4o-2024-08-06',
        isHealthy: false,
        manualOverride: false,
        probedAt: 1712310300,
        probeEnabled: false,
        consecutiveFailures: 0,
        autoDisabledAt: null,
      },
    ]);

    expect(collectVisibleChannelTargets(groups)).toEqual([
      {
        key: 'gpt-4o:channel-a',
        displayModel: 'gpt-4o',
        channelID: 'channel-a',
        channelName: 'Channel A',
      },
      {
        key: 'gpt-4o:channel-b',
        displayModel: 'gpt-4o',
        channelID: 'channel-b',
        channelName: 'Channel B',
      },
    ]);
  });

  it('builds group probe targets from all enabled channel rows', () => {
    const [group] = buildModelHealthTree([
      {
        channelName: 'Channel A',
        channelStatus: 'enabled',
        channelType: 'openai',
        orderingWeight: 10,
        priority: 1,
        displayModel: 'gpt-4o',
        channelID: 'Q2hhbm5lbDox',
        actualModelID: 'gpt-4o-2024-11-20',
        isHealthy: true,
        manualOverride: false,
        probedAt: 1712310000,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
      },
      {
        channelName: 'Channel B',
        channelStatus: 'disabled',
        channelType: 'openai',
        orderingWeight: 5,
        priority: 1,
        displayModel: 'gpt-4o',
        channelID: 'Q2hhbm5lbDoy',
        actualModelID: 'gpt-4o-2024-08-06',
        isHealthy: false,
        manualOverride: false,
        probedAt: 1712310300,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
      },
    ]);

    expect(getGroupProbeTargets(group)).toEqual([
      {
        displayModel: 'gpt-4o',
        channelID: 'Q2hhbm5lbDox',
        actualModelID: 'gpt-4o-2024-11-20',
      },
    ]);
  });

  it('collects visible page probe targets from configured and discovered groups', () => {
    const configuredGroups = buildModelHealthTree([
      {
        channelName: 'Channel A',
        channelStatus: 'enabled',
        channelType: 'openai',
        orderingWeight: 10,
        priority: 1,
        displayModel: 'gpt-4o',
        channelID: 'channel-a',
        actualModelID: 'gpt-4o-2024-11-20',
        isHealthy: true,
        manualOverride: false,
        probedAt: 1712310000,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
      },
      {
        channelName: 'Channel B',
        channelStatus: 'disabled',
        channelType: 'openai',
        orderingWeight: 5,
        priority: 1,
        displayModel: 'gpt-4o',
        channelID: 'channel-b',
        actualModelID: 'gpt-4o-2024-08-06',
        isHealthy: false,
        manualOverride: false,
        probedAt: 1712310300,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
      },
    ]);
    const discoveredGroups = buildModelHealthTree([
      {
        channelName: 'Channel A',
        channelStatus: 'enabled',
        channelType: 'openai',
        orderingWeight: 10,
        priority: 0,
        displayModel: 'gpt-4o',
        channelID: 'channel-a',
        actualModelID: 'gpt-4o-2024-11-20',
        isHealthy: true,
        manualOverride: false,
        probedAt: 1712310400,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
      },
      {
        channelName: 'Channel C',
        channelStatus: 'enabled',
        channelType: 'openai',
        orderingWeight: 8,
        priority: 0,
        displayModel: 'gpt-5',
        channelID: 'channel-c',
        actualModelID: 'gpt-5',
        isHealthy: true,
        manualOverride: false,
        probedAt: 1712310500,
        probeEnabled: true,
        consecutiveFailures: 0,
        autoDisabledAt: null,
      },
    ]);

    expect(collectVisiblePageProbeTargets(configuredGroups, discoveredGroups)).toEqual([
      {
        displayModel: 'gpt-4o',
        channelID: 'channel-a',
        actualModelID: 'gpt-4o-2024-11-20',
      },
      {
        displayModel: 'gpt-5',
        channelID: 'channel-c',
        actualModelID: 'gpt-5',
      },
    ]);
  });
});
