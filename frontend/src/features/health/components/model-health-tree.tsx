import { useEffect, useState } from 'react';
import { ChevronDown, ChevronRight, Loader2, Radar } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { Switch } from '@/components/ui/switch';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import type { ModelHealthHistory } from '../data/schema';
import { formatHealthTimestamp } from '../channel-health-format';
import { ModelHealthCell } from './model-health-cell';
import { getActualModelHistory, getDisplayModelHistory, type ModelHealthChannelGroup, type ModelHealthGroup, type ModelHealthRow } from '../model-health-page';

interface ModelHealthTreeProps {
  groups: ModelHealthGroup[];
  histories: Record<string, ModelHealthHistory[]>;
  probingKeys: Record<string, true>;
  updatingProbeKeys?: Record<string, true>;
  selectedChannelKeys?: Record<string, true>;
  locale: string;
  showChannelSelection?: boolean;
  onToggleSelectChannel?: (group: ModelHealthGroup, channel: ModelHealthChannelGroup, checked: boolean) => void;
  onProbeGroup: (group: ModelHealthGroup) => void;
  onProbeChannel: (group: ModelHealthGroup, channel: ModelHealthChannelGroup) => void;
  onProbeRow: (row: ModelHealthRow) => void;
  onToggleRowProbe: (row: ModelHealthRow, next: boolean) => void;
  onToggleChannelProbe: (group: ModelHealthGroup, channel: ModelHealthChannelGroup) => void;
}

export function getDefaultGroupExpanded() {
  return false;
}

export function getChannelProbeState(channel: ModelHealthChannelGroup): 'enabled' | 'disabled' | 'unknown' {
  const enabledCount = channel.rows.filter((row) => row.probeEnabled).length;
  if (channel.rows.length === 0) {
    return 'unknown';
  }

  if (enabledCount === channel.rows.length) {
    return 'enabled';
  }

  if (enabledCount === 0) {
    return 'disabled';
  }

  return 'unknown';
}

export function getLatestProbeMeta(snapshot: Pick<ModelHealthHistory, 'isHealthy' | 'probedAt'> | undefined, history?: ModelHealthHistory[]) {
  const latest = history?.[0] ?? snapshot;
  if (!latest) {
    return null;
  }

  return {
    isHealthy: latest.isHealthy,
    probedAt: latest.probedAt,
  };
}

function getConnectionKey(displayModel: string, channelID: string, actualModelID: string) {
  return `${displayModel}:${channelID}:${actualModelID}`;
}

function getChannelSummary(channel: ModelHealthChannelGroup) {
  const healthyCount = channel.rows.filter((row) => row.isHealthy).length;
  const enabledCount = channel.rows.filter((row) => row.probeEnabled).length;
  const probeState = getChannelProbeState(channel);
  return {
    healthyCount,
    totalCount: channel.rows.length,
    isHealthy: channel.rows.length > 0 && healthyCount === channel.rows.length,
    enabledCount,
    allEnabled: channel.rows.length > 0 && enabledCount === channel.rows.length,
    noneEnabled: enabledCount === 0,
    probeState,
  };
}

export function ModelHealthTree({
  groups,
  histories,
  probingKeys,
  updatingProbeKeys = {},
  selectedChannelKeys = {},
  locale,
  showChannelSelection = false,
  onToggleSelectChannel,
  onProbeGroup,
  onProbeChannel,
  onProbeRow,
  onToggleRowProbe,
  onToggleChannelProbe,
}: ModelHealthTreeProps) {
  const { t } = useTranslation();
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({});
  const [expandedChannels, setExpandedChannels] = useState<Record<string, boolean>>({});

  useEffect(() => {
    setExpandedGroups((current) => {
      const next = { ...current };
      groups.forEach((group) => {
        if (next[group.displayModel] == null) next[group.displayModel] = getDefaultGroupExpanded();
      });
      return next;
    });
  }, [groups]);

  return (
    <div className='space-y-4'>
      {groups.map((group) => (
        <Collapsible
          key={group.displayModel}
          open={expandedGroups[group.displayModel] ?? getDefaultGroupExpanded()}
          onOpenChange={(open) => setExpandedGroups((current) => ({ ...current, [group.displayModel]: open }))}
        >
          <section className='rounded-xl border'>
            <div className='flex items-center justify-between gap-4 p-4'>
              <CollapsibleTrigger asChild>
                <button className='flex flex-1 items-center gap-3 text-left'>
                  {(expandedGroups[group.displayModel] ?? getDefaultGroupExpanded()) ? (
                    <ChevronDown className='h-4 w-4' />
                  ) : (
                    <ChevronRight className='h-4 w-4' />
                  )}
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
                const channelHistory = getDisplayModelHistory(channel, histories);
                const channelProbeMeta = getLatestProbeMeta(channel.rows[0], channelHistory);
                const channelToggleKey = `channel:${group.displayModel}:${channel.channelID}`;
                const isChannelUpdating = Boolean(updatingProbeKeys[channelToggleKey]);
                const channelProbeStateTitle =
                  summary.probeState === 'enabled'
                    ? t('models.healthPage.channelProbeAllOn')
                    : summary.probeState === 'disabled'
                      ? t('models.healthPage.channelProbeAllOff')
                      : t('models.healthPage.channelProbePartial', { enabled: summary.enabledCount, total: summary.totalCount });
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
                              <div className='flex items-center gap-2'>
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <span
                                      aria-hidden='true'
                                      className={`inline-block h-2.5 w-2.5 rounded-full ${
                                        summary.probeState === 'enabled'
                                          ? 'bg-emerald-500'
                                          : summary.probeState === 'disabled'
                                            ? 'bg-rose-500'
                                            : 'bg-slate-400'
                                      }`}
                                    />
                                  </TooltipTrigger>
                                  <TooltipContent>{channelProbeStateTitle}</TooltipContent>
                                </Tooltip>
                                <div className='font-medium'>{channel.channelName}</div>
                              </div>
                              <div className='text-muted-foreground text-xs'>
                                {t(`channels.status.${channel.channelStatus}`)} · {t('models.healthPage.channelSummary', { healthy: summary.healthyCount, total: summary.totalCount })}
                              </div>
                              {channelProbeMeta ? (
                                <div className='text-muted-foreground text-xs'>
                                  {channelProbeMeta.isHealthy ? t('models.healthPage.healthy') : t('models.healthPage.unhealthy')}
                                  {' · '}
                                  {t('models.healthPage.lastProbe', { val: formatHealthTimestamp(channelProbeMeta.probedAt, locale) })}
                                </div>
                              ) : null}
                            </div>
                          </button>
                        </CollapsibleTrigger>
                        <div className='flex items-center gap-3'>
                          <ModelHealthCell snapshot={channel.rows[0]} history={channelHistory} locale={locale} />
                          {showChannelSelection ? (
                            <Checkbox
                              checked={Boolean(selectedChannelKeys[channelKey])}
                              onCheckedChange={(checked) => onToggleSelectChannel?.(group, channel, checked === true)}
                              aria-label={t('models.healthPage.channelSelectAria')}
                            />
                          ) : null}
                          <Switch
                            checked={summary.allEnabled}
                            aria-checked={summary.allEnabled ? true : summary.noneEnabled ? false : 'mixed'}
                            onCheckedChange={() => onToggleChannelProbe(group, channel)}
                            aria-label={t('models.healthPage.channelProbeToggleAria')}
                            disabled={isChannelUpdating}
                            title={channelProbeStateTitle}
                          />
                          {isChannelUpdating ? <Loader2 className='text-muted-foreground h-4 w-4 animate-spin' /> : null}
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
                          const rowHistory = getActualModelHistory(row, histories);
                          const rowProbeMeta = getLatestProbeMeta(row, rowHistory);
                          const isRowUpdating = Boolean(updatingProbeKeys[`row:${row.displayModel}:${row.channelID}:${row.actualModelID}`]);
                          return (
                            <div
                              key={rowKey}
                              className={`flex items-center justify-between gap-4 rounded-md border p-3 ${row.probeEnabled ? '' : 'opacity-70'}`}
                            >
                              <div className='space-y-1'>
                                <div className='flex items-center gap-2'>
                                  <div className='font-medium'>{row.actualModelID}</div>
                                  {row.autoDisabledAt ? (
                                    <Tooltip>
                                      <TooltipTrigger asChild>
                                        <Badge variant='secondary'>{t('models.healthPage.autoDisabledBadge')}</Badge>
                                      </TooltipTrigger>
                                      <TooltipContent>
                                        {t('models.healthPage.autoDisabledTooltip', { val: formatHealthTimestamp(row.autoDisabledAt, locale) })}
                                      </TooltipContent>
                                    </Tooltip>
                                  ) : null}
                                </div>
                                <div className='text-muted-foreground text-xs'>
                                  {t('models.healthPage.priorityWeight', { priority: row.priority, weight: row.orderingWeight })}
                                </div>
                                {rowProbeMeta ? (
                                  <div className='text-muted-foreground text-xs'>
                                    {rowProbeMeta.isHealthy ? t('models.healthPage.healthy') : t('models.healthPage.unhealthy')}
                                    {' · '}
                                    {t('models.healthPage.lastProbe', { val: formatHealthTimestamp(rowProbeMeta.probedAt, locale) })}
                                  </div>
                                ) : null}
                              </div>
                              <div className='flex items-center gap-2'>
                                <ModelHealthCell snapshot={row} history={rowHistory} locale={locale} />
                                <Switch
                                  checked={row.probeEnabled}
                                  onCheckedChange={(checked) => onToggleRowProbe(row, checked)}
                                  aria-label={t('models.healthPage.probeToggleAria')}
                                  disabled={isRowUpdating}
                                />
                                {isRowUpdating ? <Loader2 className='text-muted-foreground h-4 w-4 animate-spin' /> : null}
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
