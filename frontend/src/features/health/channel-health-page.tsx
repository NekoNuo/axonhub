import { useMemo, useState } from 'react';
import { Search } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Main } from '@/components/layout/main';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { ChannelHealthCell } from '@/features/channels/components/channel-health-cell';
import { useChannelHealthSnapshots, useChannelProbeData, useQueryChannels } from '@/features/channels/data/channels';
import type { ChannelHealthSnapshot, ChannelProbeData } from '@/features/channels/data/schema';
import { formatHealthTimestamp } from './channel-health-format';

interface ChannelHealthPageChannel {
  id: string;
  name: string;
  orderingWeight: number;
}

interface ChannelHealthRow {
  id: string;
  name: string;
  orderingWeight: number;
  points: ChannelProbeData['points'];
  snapshot?: ChannelHealthSnapshot;
}

interface ChannelSnapshotDetails {
  probeTimestamp: number;
  activeProbeLatencyMs?: number | null;
  probeModelLatencyMs?: number | null;
  observedTimestamp: number;
  observedLatencyMs?: number | null;
}

export function buildChannelHealthRows(
  channels: ChannelHealthPageChannel[],
  probeData: ChannelProbeData[],
  snapshots: ChannelHealthSnapshot[]
): ChannelHealthRow[] {
  const probeMap = new Map(probeData.map((item) => [item.channelID, item.points]));
  const snapshotMap = new Map(snapshots.map((item) => [item.channelID, item]));

  return channels
    .map((channel) => ({
      id: channel.id,
      name: channel.name,
      points: probeMap.get(channel.id) || [],
      snapshot: snapshotMap.get(channel.id),
      orderingWeight: channel.orderingWeight,
    }))
    .sort((a, b) => b.orderingWeight - a.orderingWeight || a.name.localeCompare(b.name));
}

export function getLatestSnapshotDetails(snapshot?: ChannelHealthSnapshot | null): ChannelSnapshotDetails | null {
  if (!snapshot) {
    return null;
  }

  return {
    probeTimestamp: snapshot.probeTimestamp,
    activeProbeLatencyMs: snapshot.activeProbeLatencyMs,
    probeModelLatencyMs: snapshot.probeModelLatencyMs,
    observedTimestamp: snapshot.observedTimestamp,
    observedLatencyMs: snapshot.observedLatencyMs,
  };
}

export function ChannelHealthPage() {
  const { t, i18n } = useTranslation();
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'enabled' | 'disabled'>('all');
  const locale = i18n.language.startsWith('zh') ? 'zh-CN' : 'en-US';
  const { data } = useQueryChannels({
    first: 100,
    where: {
      statusIn: ['enabled', 'disabled'],
    },
    orderBy: {
      field: 'CREATED_AT',
      direction: 'DESC',
    },
  });

  const channels = useMemo(
    () =>
      (data?.edges || []).map((edge) => ({
        id: edge.node.id,
        name: edge.node.name,
        orderingWeight: edge.node.orderingWeight ?? 0,
      })),
    [data?.edges]
  );

  const channelIDs = useMemo(() => channels.map((channel) => channel.id), [channels]);
  const { data: probeData = [] } = useChannelProbeData(channelIDs);
  const { data: snapshots = [] } = useChannelHealthSnapshots(channelIDs);

  const rows = useMemo(
    () =>
      buildChannelHealthRows(channels, probeData, snapshots).filter((row) => {
        const channel = data?.edges?.find((edge) => edge.node.id === row.id)?.node;
        const matchesSearch = search.trim() === '' || row.name.toLowerCase().includes(search.toLowerCase()) || row.id.toLowerCase().includes(search.toLowerCase());
        const matchesStatus = statusFilter === 'all' || channel?.status === statusFilter;
        return matchesSearch && matchesStatus;
      }),
    [channels, data?.edges, probeData, search, snapshots, statusFilter]
  );

  return (
    <Main>
      <div className='space-y-6'>
        <div>
          <h1 className='text-2xl font-semibold'>{t('channels.healthPage.title')}</h1>
          <p className='text-muted-foreground text-sm'>{t('channels.healthPage.description')}</p>
        </div>

        <div className='flex flex-col gap-3 md:flex-row'>
          <div className='relative flex-1'>
            <Search className='text-muted-foreground absolute top-2.5 left-3 h-4 w-4' />
            <Input value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t('channels.filters.filterByName')} className='pl-9' />
          </div>
          <Select value={statusFilter} onValueChange={(value) => setStatusFilter(value as 'all' | 'enabled' | 'disabled')}>
            <SelectTrigger className='w-full md:w-52'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='all'>{t('models.healthPage.filters.allStatuses')}</SelectItem>
              <SelectItem value='enabled'>{t('channels.status.enabled')}</SelectItem>
              <SelectItem value='disabled'>{t('channels.status.disabled')}</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div className='space-y-3'>
          {rows.map((row) => {
            const details = getLatestSnapshotDetails(row.snapshot);

            return (
              <div key={row.id} className='rounded-xl border p-4'>
                <div className='mb-3 flex items-center justify-between gap-4'>
                  <div>
                    <div className='font-medium'>{row.name}</div>
                    <div className='text-muted-foreground flex flex-wrap gap-3 text-xs'>
                      <span>{row.id}</span>
                      <span>{t('channels.healthPage.summary.orderingWeight', { weight: row.orderingWeight })}</span>
                    </div>
                  </div>
                  <ChannelHealthCell points={row.points} snapshot={row.snapshot} />
                </div>
                {details ? (
                  <div className='text-muted-foreground flex flex-wrap gap-4 text-xs'>
                    <span>{t('channels.healthPage.summary.probeTime', { val: formatHealthTimestamp(details.probeTimestamp, locale) })}</span>
                    <span>{t('channels.healthPage.summary.activeProbeLatency', { val: details.activeProbeLatencyMs ?? '-' })}</span>
                    <span>{t('channels.healthPage.summary.probeModelLatency', { val: details.probeModelLatencyMs ?? '-' })}</span>
                    <span>{t('channels.healthPage.summary.observedLatency', { val: details.observedLatencyMs ?? '-' })}</span>
                  </div>
                ) : (
                  <div className='text-muted-foreground text-xs'>{t('channels.healthPage.summary.empty')}</div>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </Main>
  );
}

export default ChannelHealthPage;
