import type { ChannelSummary } from '../data/schema';

const WEIGHT_PRECISION = 0;
const MIN_WEIGHT = 0;
const MAX_WEIGHT = 100;

export interface ChannelOrderingItem {
  channel: ChannelSummary;
  orderingWeight: number;
}

export interface ChannelSummaryEdge {
  node: ChannelSummary;
}

const formatWeight = (value: number) => Number(value.toFixed(WEIGHT_PRECISION));

export const clampWeight = (value: number) => formatWeight(Math.min(MAX_WEIGHT, Math.max(MIN_WEIGHT, value)));

export const sortChannelsByWeightDesc = <T extends { orderingWeight: number }>(items: T[]) =>
  [...items].sort((a, b) => b.orderingWeight - a.orderingWeight);

export const initializeOrderedChannels = (edges: ChannelSummaryEdge[]): ChannelOrderingItem[] =>
  sortChannelsByWeightDesc(
    edges.map((edge) => ({
      channel: edge.node,
      orderingWeight: edge.node.orderingWeight ?? 0,
    }))
  );

export const autoSortChannelsByWeight = (items: ChannelOrderingItem[]) =>
  (() => {
    const sorted = sortChannelsByWeightDesc(items);
    let nextWeight = sorted.filter((item) => item.channel.status === 'enabled').length;

    const normalized = sorted.map((item) => {
      if (item.channel.status !== 'enabled') {
        return {
          ...item,
          orderingWeight: 0,
        };
      }

      const nextItem = {
        ...item,
        orderingWeight: nextWeight,
      };
      nextWeight -= 1;
      return nextItem;
    });

    return sortChannelsByWeightDesc(normalized);
  })();

export const reassignWeightFromPosition = (items: ChannelOrderingItem[], movedItemIndex: number): ChannelOrderingItem[] => {
  if (items.length <= 1) {
    return items.map((item) => ({ ...item }));
  }

  const result = items.map((item) => ({ ...item }));
  const prevWeight = result[movedItemIndex - 1]?.orderingWeight;
  const nextWeight = result[movedItemIndex + 1]?.orderingWeight;

  if (prevWeight != null && nextWeight != null && prevWeight === nextWeight) {
    result[movedItemIndex].orderingWeight = prevWeight;
    return result;
  }

  if (prevWeight == null && nextWeight != null) {
    result[movedItemIndex].orderingWeight = clampWeight(nextWeight + 1);
    return result;
  }

  if (nextWeight == null && prevWeight != null) {
    result[movedItemIndex].orderingWeight = clampWeight(prevWeight - 1);
    return result;
  }

  if (prevWeight != null && nextWeight != null) {
    const gap = prevWeight - nextWeight;
    if (gap <= 1) {
      for (let i = movedItemIndex + 1; i < result.length; i++) {
        result[i].orderingWeight = clampWeight(result[i].orderingWeight - 1);
      }
      result[movedItemIndex].orderingWeight = clampWeight(prevWeight - 1);
    } else {
      result[movedItemIndex].orderingWeight = clampWeight(nextWeight + Math.floor(gap / 2));
    }
  }

  return result;
};
