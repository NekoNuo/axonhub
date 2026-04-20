import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { getChannelProbeState, getDefaultGroupExpanded, getLatestProbeMeta, ModelHealthTree } from './model-health-tree';

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

  it('keeps display-model groups collapsed by default', () => {
    expect(getDefaultGroupExpanded()).toBe(false);
  });

  it('returns enabled when every row is probe-enabled', () => {
    expect(
      getChannelProbeState({
        channelID: 'c1',
        channelName: 'Channel A',
        channelStatus: 'enabled',
        channelType: 'openai',
        orderingWeight: 1,
        rows: [
          {
            displayModel: 'gpt-5.4',
            channelID: 'c1',
            actualModelID: 'gpt-5.4-high',
            isHealthy: true,
            manualOverride: false,
            probedAt: 100,
            channelName: 'Channel A',
            channelStatus: 'enabled',
            channelType: 'openai',
            orderingWeight: 1,
            priority: 1,
            probeEnabled: true,
            consecutiveFailures: 0,
            autoDisabledAt: null,
          },
        ],
      })
    ).toBe('enabled');
  });

  it('returns disabled when every row is probe-disabled', () => {
    expect(
      getChannelProbeState({
        channelID: 'c1',
        channelName: 'Channel A',
        channelStatus: 'enabled',
        channelType: 'openai',
        orderingWeight: 1,
        rows: [
          {
            displayModel: 'gpt-5.4',
            channelID: 'c1',
            actualModelID: 'gpt-5.4-high',
            isHealthy: true,
            manualOverride: false,
            probedAt: 100,
            channelName: 'Channel A',
            channelStatus: 'enabled',
            channelType: 'openai',
            orderingWeight: 1,
            priority: 1,
            probeEnabled: false,
            consecutiveFailures: 0,
            autoDisabledAt: null,
          },
        ],
      })
    ).toBe('disabled');
  });

  it('returns unknown when rows have mixed probe states', () => {
    expect(
      getChannelProbeState({
        channelID: 'c1',
        channelName: 'Channel A',
        channelStatus: 'enabled',
        channelType: 'openai',
        orderingWeight: 1,
        rows: [
          {
            displayModel: 'gpt-5.4',
            channelID: 'c1',
            actualModelID: 'gpt-5.4-high',
            isHealthy: true,
            manualOverride: false,
            probedAt: 100,
            channelName: 'Channel A',
            channelStatus: 'enabled',
            channelType: 'openai',
            orderingWeight: 1,
            priority: 1,
            probeEnabled: true,
            consecutiveFailures: 0,
            autoDisabledAt: null,
          },
          {
            displayModel: 'gpt-5.4',
            channelID: 'c1',
            actualModelID: 'gpt-5.4-low',
            isHealthy: false,
            manualOverride: false,
            probedAt: 90,
            channelName: 'Channel A',
            channelStatus: 'enabled',
            channelType: 'openai',
            orderingWeight: 1,
            priority: 2,
            probeEnabled: false,
            consecutiveFailures: 1,
            autoDisabledAt: null,
          },
        ],
      })
    ).toBe('unknown');
  });

  it('renders channel-level probe switch without a dedicated probe label', () => {
    const markup = renderToStaticMarkup(
      React.createElement(ModelHealthTree, {
        groups: [
          {
            displayModel: 'gpt-5.4',
            channels: [
              {
                channelID: 'c1',
                channelName: 'Channel A',
                channelStatus: 'enabled',
                channelType: 'openai',
                orderingWeight: 1,
                rows: [
                  {
                    displayModel: 'gpt-5.4',
                    channelID: 'c1',
                    actualModelID: 'gpt-5.4-high',
                    isHealthy: true,
                    manualOverride: false,
                    probedAt: 100,
                    channelName: 'Channel A',
                    channelStatus: 'enabled',
                    channelType: 'openai',
                    orderingWeight: 1,
                    priority: 1,
                    probeEnabled: true,
                    consecutiveFailures: 0,
                    autoDisabledAt: null,
                  },
                ],
              },
            ],
          },
        ],
        histories: {},
        probingKeys: {},
        locale: 'zh-CN',
        onProbeGroup: () => {},
        onProbeChannel: () => {},
        onProbeRow: () => {},
        onToggleRowProbe: () => {},
        onToggleChannelProbe: () => {},
      })
    );

    expect(markup).not.toContain('rounded-md border px-2 py-1');
  });
});
