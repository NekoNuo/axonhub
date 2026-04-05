import { describe, expect, it } from 'vitest';
import {
  buildModelHealthTree,
  getChannelProbeTargets,
  getModelsPendingConnectionQuery,
  getProbeEnabledModelEntries,
  getRowProbeTarget,
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
});
