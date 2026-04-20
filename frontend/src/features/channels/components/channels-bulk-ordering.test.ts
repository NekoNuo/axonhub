import { describe, expect, it } from 'vitest';
import { autoSortChannelsByWeight, initializeOrderedChannels } from './channels-bulk-ordering';

describe('channels bulk ordering helpers', () => {
  it('keeps larger weights first when initializing dialog items', () => {
    const ordered = initializeOrderedChannels([
      {
        node: {
          id: 'channel-1',
          name: 'Channel 1',
          status: 'enabled',
          orderingWeight: 10,
        },
      },
      {
        node: {
          id: 'channel-2',
          name: 'Channel 2',
          status: 'enabled',
          orderingWeight: 30,
        },
      },
      {
        node: {
          id: 'channel-3',
          name: 'Channel 3',
          status: 'enabled',
          orderingWeight: 20,
        },
      },
    ]);

    expect(ordered.map((item) => item.channel.id)).toEqual(['channel-2', 'channel-3', 'channel-1']);
    expect(ordered.map((item) => item.orderingWeight)).toEqual([30, 20, 10]);
  });

  it('auto sorts by descending weight, compacts enabled weights, and resets non-enabled channels to zero', () => {
    const ordered = autoSortChannelsByWeight([
      {
        channel: {
          id: 'channel-1',
          name: 'Channel 1',
          status: 'enabled',
        },
        orderingWeight: 10,
      },
      {
        channel: {
          id: 'channel-2',
          name: 'Channel 2',
          status: 'enabled',
        },
        orderingWeight: 30,
      },
      {
        channel: {
          id: 'channel-3',
          name: 'Channel 3',
          status: 'disabled',
        },
        orderingWeight: 20,
      },
    ]);

    expect(ordered.map((item) => item.channel.id)).toEqual(['channel-2', 'channel-1', 'channel-3']);
    expect(ordered.map((item) => item.orderingWeight)).toEqual([2, 1, 0]);
  });
});
