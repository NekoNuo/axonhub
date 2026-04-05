import { useEffect, useState } from 'react';
import { ChevronDown, ChevronRight, Loader2, Radar } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import type { ModelHealthHistory } from '../data/schema';
import { formatHealthTimestamp } from '../channel-health-format';
import { ModelHealthCell } from './model-health-cell';
import { getActualModelHistory, getDisplayModelHistory, type ModelHealthChannelGroup, type ModelHealthGroup, type ModelHealthRow } from '../model-health-page';

interface ModelHealthTreeProps {
  groups: ModelHealthGroup[];
  histories: Record<string, ModelHealthHistory[]>;
  probingKeys: Record<string, true>;
  locale: string;
  onProbeGroup: (group: ModelHealthGroup) => void;
  onProbeChannel: (group: ModelHealthGroup, channel: ModelHealthChannelGroup) => void;
  onProbeRow: (row: ModelHealthRow) => void;
}

function getConnectionKey(displayModel: string, channelID: string, actualModelID: string) {
  return `${displayModel}:${channelID}:${actualModelID}`;
}

function getChannelSummary(channel: ModelHealthChannelGroup) {
  const healthyCount = channel.rows.filter((row) => row.isHealthy).length;
  return {
    healthyCount,
    totalCount: channel.rows.length,
    isHealthy: channel.rows.length > 0 && healthyCount === channel.rows.length,
  };
}

export function ModelHealthTree({ groups, histories, probingKeys, locale, onProbeGroup, onProbeChannel, onProbeRow }: ModelHealthTreeProps) {
  const { t } = useTranslation();
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({});
  const [expandedChannels, setExpandedChannels] = useState<Record<string, boolean>>({});

  useEffect(() => {
    setExpandedGroups((current) => {
      const next = { ...current };
      groups.forEach((group) => {
        if (next[group.displayModel] == null) next[group.displayModel] = true;
      });
      return next;
    });
  }, [groups]);

  return (
    <div className='space-y-4'>
      {groups.map((group) => (
        <Collapsible
          key={group.displayModel}
          open={expandedGroups[group.displayModel] ?? true}
          onOpenChange={(open) => setExpandedGroups((current) => ({ ...current, [group.displayModel]: open }))}
        >
          <section className='rounded-xl border'>
            <div className='flex items-center justify-between gap-4 p-4'>
              <CollapsibleTrigger asChild>
                <button className='flex flex-1 items-center gap-3 text-left'>
                  {(expandedGroups[group.displayModel] ?? true) ? <ChevronDown className='h-4 w-4' /> : <ChevronRight className='h-4 w-4' />}
                  <div>
                    <h2 className='text-lg font-medium'>{group.displayModel}</h2>
                    <div className='text-muted-foreground text-xs'>{t('models.healthPage.groupSummary', { count: group.channels.length })}</div>
                  </div>
                </button>
              </CollapsibleTrigger>
              <div className='flex items-center gap-3'>
                <ModelHealthCell
                  snapshot={group.channels[0]?.rows[0]}
                  history={group.channels.flatMap((channel) => getDisplayModelHistory(channel, histories))}
                  locale={locale}
                />
                <Button
                  size='icon'
                  variant='outline'
                  title={t('models.healthPage.manualProbe')}
                  onClick={() => onProbeGroup(group)}
                  disabled={probingKeys[`group:${group.displayModel}`] || group.channels.every((channel) => channel.channelStatus !== 'enabled')}
                >
                  {probingKeys[`group:${group.displayModel}`] ? <Loader2 className='h-4 w-4 animate-spin' /> : <Radar className='h-4 w-4' />}
                </Button>
              </div>
            </div>
            <CollapsibleContent className='space-y-3 px-4 pb-4'>
              {group.channels.map((channel) => {
                const channelKey = `${group.displayModel}:${channel.channelID}`;
                const summary = getChannelSummary(channel);
                return (
                  <Collapsible
                    key={channelKey}
                    open={expandedChannels[channelKey] ?? false}
                    onOpenChange={(open) => setExpandedChannels((current) => ({ ...current, [channelKey]: open }))}
                  >
                    <div className='rounded-lg border'>
                      <div className='flex items-center justify-between gap-4 p-3'>
                        <CollapsibleTrigger asChild>
                          <button className='flex flex-1 items-center gap-3 text-left'>
                            {(expandedChannels[channelKey] ?? false) ? <ChevronDown className='h-4 w-4' /> : <ChevronRight className='h-4 w-4' />}
                            <div>
                              <div className='font-medium'>{channel.channelName}</div>
                              <div className='text-muted-foreground text-xs'>
                                {t(`channels.status.${channel.channelStatus}`)} · {t('models.healthPage.channelSummary', { healthy: summary.healthyCount, total: summary.totalCount })}
                              </div>
                            </div>
                          </button>
                        </CollapsibleTrigger>
                        <div className='flex items-center gap-3'>
                          <ModelHealthCell
                            snapshot={channel.rows[0]}
                            history={getDisplayModelHistory(channel, histories)}
                            locale={locale}
                          />
                          <Button
                            size='icon'
                            variant='ghost'
                            title={t('models.healthPage.manualProbe')}
                            onClick={() => onProbeChannel(group, channel)}
                            disabled={probingKeys[`channel:${group.displayModel}:${channel.channelID}`] || channel.channelStatus !== 'enabled'}
                          >
                            {probingKeys[`channel:${group.displayModel}:${channel.channelID}`] ? (
                              <Loader2 className='h-4 w-4 animate-spin' />
                            ) : (
                              <Radar className='h-4 w-4' />
                            )}
                          </Button>
                        </div>
                      </div>
                      <CollapsibleContent className='space-y-2 px-3 pb-3'>
                        {channel.rows.map((row) => {
                          const rowKey = getConnectionKey(row.displayModel, row.channelID, row.actualModelID);
                          return (
                            <div key={rowKey} className='flex items-center justify-between gap-4 rounded-md border p-3'>
                              <div className='space-y-1'>
                                <div className='font-medium'>{row.actualModelID}</div>
                                <div className='text-muted-foreground text-xs'>
                                  {t('models.healthPage.priorityWeight', { priority: row.priority, weight: row.orderingWeight })}
                                </div>
                                <div className='text-muted-foreground text-xs'>{t('models.healthPage.lastProbe', { val: formatHealthTimestamp(row.probedAt, locale) })}</div>
                              </div>
                              <div className='flex items-center gap-2'>
                                <ModelHealthCell snapshot={row} history={getActualModelHistory(row, histories)} locale={locale} />
                                <Button
                                  size='icon'
                                  variant='ghost'
                                  title={t('models.healthPage.manualProbe')}
                                  onClick={() => onProbeRow(row)}
                                  disabled={probingKeys[`row:${row.displayModel}:${row.channelID}:${row.actualModelID}`] || row.channelStatus !== 'enabled'}
                                >
                                  {probingKeys[`row:${row.displayModel}:${row.channelID}:${row.actualModelID}`] ? (
                                    <Loader2 className='h-4 w-4 animate-spin' />
                                  ) : (
                                    <Radar className='h-4 w-4' />
                                  )}
                                </Button>
                              </div>
                            </div>
                          );
                        })}
                      </CollapsibleContent>
                    </div>
                  </Collapsible>
                );
              })}
            </CollapsibleContent>
          </section>
        </Collapsible>
      ))}
    </div>
  );
}
