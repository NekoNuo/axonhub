import { startTransition, useEffect, useMemo, useState } from 'react';
import { toast } from 'sonner';
import { Main } from '@/components/layout/main';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Search } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useQueryChannels } from '@/features/channels/data/channels';
import { useQueryAllModels, useQueryModelChannelConnections, type ModelAssociationInput, type ModelChannelConnection } from '@/features/models/data/models';
import { formatHealthTimestamp } from './channel-health-format';
import { ModelHealthTree } from './components/model-health-tree';
import { fetchModelHealthHistory, fetchModelHealthSnapshots, manualModelProbe } from './data/health';
import type { ManualModelProbeInput, ModelHealthHistory, ModelHealthSnapshot } from './data/schema';

export interface ModelHealthRow extends ModelHealthSnapshot {
  channelName: string;
  channelStatus: string;
  channelType: string;
  orderingWeight: number;
  priority: number;
}

export interface ModelHealthChannelGroup {
  channelID: string;
  channelName: string;
  channelStatus: string;
  channelType: string;
  orderingWeight: number;
  rows: ModelHealthRow[];
}

export interface ModelHealthGroup {
  displayModel: string;
  channels: ModelHealthChannelGroup[];
}

interface ConnectionQueryModelEntry {
  modelID: string;
  name: string;
  settings?: {
    probeEnabled?: boolean;
    associations?: ModelAssociationInput[];
  };
}

export function getProbeEnabledModelEntries(modelEntries: ConnectionQueryModelEntry[]) {
  return modelEntries.filter((model) => model.settings?.probeEnabled && (model.settings?.associations?.length ?? 0) > 0);
}

export function buildModelHealthTree(rows: ModelHealthRow[]): ModelHealthGroup[] {
  const modelGroups = new Map<string, Map<string, ModelHealthChannelGroup>>();

  rows.forEach((row) => {
    if (!modelGroups.has(row.displayModel)) {
      modelGroups.set(row.displayModel, new Map());
    }

    const channelGroups = modelGroups.get(row.displayModel)!;
    if (!channelGroups.has(row.channelID)) {
      channelGroups.set(row.channelID, {
        channelID: row.channelID,
        channelName: row.channelName,
        channelStatus: row.channelStatus,
        channelType: row.channelType,
        orderingWeight: row.orderingWeight,
        rows: [],
      });
    }

    channelGroups.get(row.channelID)!.rows.push(row);
  });

  return Array.from(modelGroups.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([displayModel, channelGroups]) => ({
      displayModel,
      channels: Array.from(channelGroups.values())
        .map((channelGroup) => ({
          ...channelGroup,
          rows: channelGroup.rows.sort(
            (a, b) => a.priority - b.priority || b.orderingWeight - a.orderingWeight || a.actualModelID.localeCompare(b.actualModelID)
          ),
        }))
        .sort((a, b) => b.orderingWeight - a.orderingWeight || a.channelName.localeCompare(b.channelName)),
    }));
}

export function getModelsPendingConnectionQuery(
  modelEntries: ConnectionQueryModelEntry[],
  search: string,
  connectionsByModel: Record<string, ModelChannelConnection[]>,
  pendingModelIDs: Set<string>
) {
  const normalizedSearch = search.trim().toLowerCase();

  return modelEntries.filter((model) => {
    const matchesSearch =
      normalizedSearch === '' || model.modelID.toLowerCase().includes(normalizedSearch) || model.name.toLowerCase().includes(normalizedSearch);

    if (!matchesSearch) return false;
    if (connectionsByModel[model.modelID]) return false;
    if (pendingModelIDs.has(model.modelID)) return false;

    return true;
  });
}

export function getRowProbeTarget(row: Pick<ModelHealthRow, 'displayModel' | 'channelID' | 'actualModelID'>): ManualModelProbeInput {
  return {
    displayModel: row.displayModel,
    channelID: row.channelID,
    actualModelID: row.actualModelID,
  };
}

export function getChannelProbeTargets(channelGroup: ModelHealthChannelGroup): ManualModelProbeInput[] {
  return channelGroup.rows.filter((row) => row.channelStatus === 'enabled').map(getRowProbeTarget);
}

export function getGroupProbeTargets(group: ModelHealthGroup): ManualModelProbeInput[] {
  return group.channels.flatMap(getChannelProbeTargets);
}

function getConnectionKey(displayModel: string, channelID: string, actualModelID: string) {
  return `${displayModel}:${channelID}:${actualModelID}`;
}

export function getDisplayModelHistory(
  channelGroup: ModelHealthChannelGroup,
  histories: Record<string, ModelHealthHistory[]>
): ModelHealthHistory[] {
  return channelGroup.rows
    .flatMap((row) => histories[getConnectionKey(row.displayModel, row.channelID, row.actualModelID)] || [])
    .sort((a, b) => b.probedAt - a.probedAt);
}

export function getActualModelHistory(row: ModelHealthRow, histories: Record<string, ModelHealthHistory[]>) {
  return histories[getConnectionKey(row.displayModel, row.channelID, row.actualModelID)] || [];
}

async function refreshModelHealthTarget(input: ManualModelProbeInput) {
  const [snapshots, history] = await Promise.all([
    fetchModelHealthSnapshots({ input: { displayModels: [input.displayModel] } }),
    fetchModelHealthHistory({
      input: {
        displayModel: input.displayModel,
        actualModelID: input.actualModelID,
        channelIDs: [input.channelID],
      },
    }),
  ]);

  const key = getConnectionKey(input.displayModel, input.channelID, input.actualModelID);
  const snapshot = snapshots.find(
    (item) => item.displayModel === input.displayModel && item.channelID === input.channelID && item.actualModelID === input.actualModelID
  );

  return {
    key,
    snapshot,
    history,
  };
}

export function ModelHealthPage() {
  const { t, i18n } = useTranslation();
  const locale = i18n.language.startsWith('zh') ? 'zh-CN' : 'en-US';
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'enabled' | 'disabled'>('all');
  const [histories, setHistories] = useState<Record<string, ModelHealthHistory[]>>({});
  const [snapshots, setSnapshots] = useState<ModelHealthSnapshot[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [probingKeys, setProbingKeys] = useState<Record<string, true>>({});
  const [connectionsByModel, setConnectionsByModel] = useState<Record<string, ModelChannelConnection[]>>({});
  const [pendingConnectionModelIDs, setPendingConnectionModelIDs] = useState<Record<string, true>>({});
  const { data: modelsData } = useQueryAllModels({
    where: {
      statusIn: ['enabled', 'disabled'],
    },
  });
  const { data: channelsData } = useQueryChannels({
    first: 200,
    where: {
      statusIn: ['enabled', 'disabled'],
    },
    orderBy: {
      field: 'CREATED_AT',
      direction: 'DESC',
    },
  });
  const queryConnections = useQueryModelChannelConnections();

  useEffect(() => {
    void (async () => {
      setIsLoading(true);
      try {
        const data = await fetchModelHealthSnapshots({ input: {} });
        setSnapshots(data);
      } finally {
        setIsLoading(false);
      }
    })();
  }, []);

  const channelsByID = useMemo(
    () =>
      new Map(
        (channelsData?.edges || []).map((edge) => [
          edge.node.id,
          {
            name: edge.node.name,
            status: edge.node.status,
            type: edge.node.type,
            orderingWeight: edge.node.orderingWeight ?? 0,
          },
        ])
      ),
    [channelsData?.edges]
  );

  const modelEntries = useMemo(() => getProbeEnabledModelEntries((modelsData?.edges || []).map((edge) => edge.node)), [modelsData?.edges]);

  useEffect(() => {
    const pendingModelIDs = new Set(Object.keys(pendingConnectionModelIDs));
    const modelsToQuery = getModelsPendingConnectionQuery(modelEntries, search, connectionsByModel, pendingModelIDs);

    if (modelsToQuery.length === 0) return;

    startTransition(() => {
      setPendingConnectionModelIDs((current) => {
        const next = { ...current };
        modelsToQuery.forEach((model) => {
          next[model.modelID] = true;
        });
        return next;
      });
    });

    modelsToQuery.forEach((model) => {
      void queryConnections.mutateAsync(model.settings?.associations as ModelAssociationInput[]).then((connections) => {
        startTransition(() => {
          setConnectionsByModel((current) => ({
            ...current,
            [model.modelID]: connections,
          }));
          setPendingConnectionModelIDs((current) => {
            if (!current[model.modelID]) return current;
            const next = { ...current };
            delete next[model.modelID];
            return next;
          });
        });
      });
    });
  }, [connectionsByModel, modelEntries, pendingConnectionModelIDs, queryConnections.mutateAsync, search]);

  const rows = useMemo<ModelHealthRow[]>(() => {
    const snapshotMap = new Map(snapshots.map((item) => [getConnectionKey(item.displayModel, item.channelID, item.actualModelID), item]));
    const result: ModelHealthRow[] = [];

    modelEntries.forEach((model) => {
      const connections = connectionsByModel[model.modelID] || [];
      connections.forEach((connection) => {
        connection.models.forEach((matchedModel) => {
          const key = getConnectionKey(model.modelID, connection.channel.id, matchedModel.actualModel);
          const snapshot = snapshotMap.get(key);
          const channelMeta = channelsByID.get(connection.channel.id) ?? {
            name: connection.channel.name,
            status: connection.channel.status,
            type: connection.channel.type,
            orderingWeight: connection.channel.orderingWeight ?? 0,
          };

          result.push({
            id: snapshot?.id,
            displayModel: model.modelID,
            channelID: connection.channel.id,
            actualModelID: matchedModel.actualModel,
            isHealthy: snapshot?.isHealthy ?? false,
            manualOverride: snapshot?.manualOverride ?? false,
            probedAt: snapshot?.probedAt ?? 0,
            channelName: channelMeta.name,
            channelStatus: channelMeta.status,
            channelType: channelMeta.type,
            orderingWeight: channelMeta.orderingWeight,
            priority: connection.priority ?? 0,
          });
        });
      });
    });

    return result.filter((row) => {
      const matchesSearch =
        search.trim() === '' ||
        row.displayModel.toLowerCase().includes(search.toLowerCase()) ||
        row.actualModelID.toLowerCase().includes(search.toLowerCase()) ||
        row.channelName.toLowerCase().includes(search.toLowerCase());
      const matchesStatus = statusFilter === 'all' || row.channelStatus === statusFilter;
      return matchesSearch && matchesStatus;
    });
  }, [channelsByID, connectionsByModel, modelEntries, search, snapshots, statusFilter]);

  const groups = useMemo(() => buildModelHealthTree(rows), [rows]);

  useEffect(() => {
    rows.forEach((row) => {
      const key = getConnectionKey(row.displayModel, row.channelID, row.actualModelID);
      if (histories[key]) return;

      void fetchModelHealthHistory({
        input: {
          displayModel: row.displayModel,
          actualModelID: row.actualModelID,
          channelIDs: [row.channelID],
        },
      }).then((data) => {
        startTransition(() => {
          setHistories((current) => {
            if (current[key]) return current;
            return {
              ...current,
              [key]: data,
            };
          });
        });
      });
    });
  }, [histories, rows]);

  const runProbeTargets = async (scopeKey: string, targets: ManualModelProbeInput[]) => {
    if (targets.length === 0) return;

    setProbingKeys((current) => ({ ...current, [scopeKey]: true }));
    try {
      await Promise.all(
        targets.map(async (input) => {
          await manualModelProbe(input);
          const refreshed = await refreshModelHealthTarget(input);
          if (refreshed.snapshot) {
            setSnapshots((current) => {
              const next = current.filter(
                (item) => !(item.displayModel === input.displayModel && item.channelID === input.channelID && item.actualModelID === input.actualModelID)
              );
              return [refreshed.snapshot!, ...next];
            });
          }
          setHistories((current) => ({
            ...current,
            [refreshed.key]: refreshed.history,
          }));

          const latest = refreshed.history[0] ?? refreshed.snapshot;
          if (latest) {
            toast.success(
              latest.isHealthy ? t('models.healthPage.probeSuccessHealthy') : t('models.healthPage.probeSuccessUnhealthy'),
              {
                description: `${latest.actualModelID} · ${latest.manualOverride ? t('models.healthPage.manualTag') : t('models.healthPage.autoTag')}`,
              }
            );
          } else {
            toast.success(t('common.messages.success'));
          }
        })
      );
    } catch (error) {
      toast.error(t('models.healthPage.probeError', { error: error instanceof Error ? error.message : String(error) }));
    } finally {
      setProbingKeys((current) => {
        const next = { ...current };
        delete next[scopeKey];
        return next;
      });
    }
  };

  return (
    <Main>
      <div className='space-y-6'>
        <div>
          <h1 className='text-2xl font-semibold'>{t('models.healthPage.title')}</h1>
          <p className='text-muted-foreground text-sm'>{t('models.healthPage.description')}</p>
        </div>

        {isLoading ? <div className='text-muted-foreground text-sm'>{t('models.healthPage.loading')}</div> : null}

        <div className='flex flex-col gap-3 md:flex-row'>
          <div className='relative flex-1'>
            <Search className='text-muted-foreground absolute top-2.5 left-3 h-4 w-4' />
            <Input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder={t('models.healthPage.searchPlaceholder')}
              className='pl-9'
            />
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

        <ModelHealthTree
          groups={groups}
          histories={histories}
          probingKeys={probingKeys}
          locale={locale}
          onProbeGroup={(group) => runProbeTargets(`group:${group.displayModel}`, getGroupProbeTargets(group))}
          onProbeChannel={(group, channel) => runProbeTargets(`channel:${group.displayModel}:${channel.channelID}`, getChannelProbeTargets(channel))}
          onProbeRow={(row) => runProbeTargets(`row:${row.displayModel}:${row.channelID}:${row.actualModelID}`, [getRowProbeTarget(row)])}
        />
      </div>
    </Main>
  );
}

export default ModelHealthPage;
