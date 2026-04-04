import { describe, expect, it } from 'vitest';
import { getHealthNavGroup } from '@/sidebar';
import { getRouteConfig } from '@/config/route-permission';
import { Route as HealthRoute } from './route';
import { Route as HealthChannelsRoute } from './channels';
import { Route as HealthModelsRoute } from './models';

describe('Task 10 Health Navigation and Routes', () => {
  it('sidebar displays health section', () => {
    const group = getHealthNavGroup((key: string) => key);

    expect(group.title).toBe('sidebar.groups.health');
    expect(group.items).toHaveLength(2);
    expect(group.items.map((item) => ('url' in item ? item.url : null))).toEqual(['/health/channels', '/health/models']);
  });

  it('route permissions include health pages', () => {
    expect(getRouteConfig('/health/channels')).toMatchObject({
      path: '/health/channels',
      requiredScopes: ['read_channels'],
    });
    expect(getRouteConfig('/health/models')).toMatchObject({
      path: '/health/models',
      requiredScopes: ['read_channels'],
    });
  });

  it('routes resolve correctly', () => {
    expect(HealthRoute.options.component).toBeTypeOf('function');
    expect(HealthChannelsRoute.options.component).toBeTypeOf('function');
    expect(HealthModelsRoute.options.component).toBeTypeOf('function');
  });
});
