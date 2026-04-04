import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Main } from '@/components/layout/main';
import { ChannelHealthCell } from '@/features/channels/components/channel-health-cell';
import { useChannelHealthSnapshots, useChannelProbeData, useQueryChannels } from '@/features/channels/data/channels';
import type { ChannelHealthSnapshot, ChannelProbeData } from '@/features/channels/data/schema';

interface ChannelHealthPageChannel {
  id: string;
  name: string;
}

interface ChannelHealthRow {
  id: string;
  name: string;
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

  return channels.map((channel) => ({
    id: channel.id,
    name: channel.name,
    points: probeMap.get(channel.id) || [],
    snapshot: snapshotMap.get(channel.id),
  }));
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
  const { t } = useTranslation();
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
      })),
    [data?.edges]
  );

  const channelIDs = useMemo(() => channels.map((channel) => channel.id), [channels]);
  const { data: probeData = [] } = useChannelProbeData(channelIDs);
  const { data: snapshots = [] } = useChannelHealthSnapshots(channelIDs);

  const rows = useMemo(() => buildChannelHealthRows(channels, probeData, snapshots), [channels, probeData, snapshots]);

  return (
    <Main>
      <div className='space-y-6'>
        <div>
          <h1 className='text-2xl font-semibold'>{t('channels.healthPage.title')}</h1>
          <p className='text-muted-foreground text-sm'>{t('channels.healthPage.description')}</p>
        </div>

        <div className='space-y-3'>
          {rows.map((row) => {
            const details = getLatestSnapshotDetails(row.snapshot);

            return (
              <div key={row.id} className='rounded-xl border p-4'>
                <div className='mb-3 flex items-center justify-between gap-4'>
                  <div>
                    <div className='font-medium'>{row.name}</div>
                    <div className='text-muted-foreground text-xs'>{row.id}</div>
                  </div>
                  <ChannelHealthCell points={row.points} snapshot={row.snapshot} />
                </div>
                {details ? (
                  <div className='text-muted-foreground flex flex-wrap gap-4 text-xs'>
                    <span>{t('channels.healthPage.summary.probeTime', { val: details.probeTimestamp })}</span>
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
