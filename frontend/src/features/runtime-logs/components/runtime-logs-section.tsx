import { useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { JsonViewer } from '@/components/json-tree-view';
import { ServerSidePagination } from '@/components/server-side-pagination';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Switch } from '@/components/ui/switch';
import { usePaginationSearch } from '@/hooks/use-pagination-search';
import type { DateTimeRangeValue } from '@/utils/date-range';
import { useRuntimeLogs } from '../data';
import { buildRuntimeLogWhereClause } from '../runtime-log-filters';

interface RuntimeLogsSectionProps {
  dateRange?: DateTimeRangeValue;
}

function getLevelBadgeVariant(level: 'debug' | 'info' | 'warn' | 'error') {
  switch (level) {
    case 'error':
      return 'destructive' as const;
    case 'warn':
      return 'secondary' as const;
    default:
      return 'outline' as const;
  }
}

export function RuntimeLogsSection({ dateRange }: RuntimeLogsSectionProps) {
  const { t } = useTranslation();
  const [includeInfo, setIncludeInfo] = useState(false);
  const { pageSize, setCursors, setPageSize, resetCursor, paginationArgs, cursorHistory } = usePaginationSearch({
    defaultPageSize: 20,
    pageSizeStorageKey: 'requests-runtime-logs-page-size',
    startCursorKey: 'runtimeLogsStartCursor',
    endCursorKey: 'runtimeLogsEndCursor',
    pageSizeKey: 'runtimeLogsPageSize',
    directionKey: 'runtimeLogsDirection',
    cursorHistoryKey: 'runtimeLogsCursorHistory',
  });

  const { data, isLoading, refetch } = useRuntimeLogs({
    ...paginationArgs,
    where: buildRuntimeLogWhereClause(dateRange, includeInfo),
    orderBy: {
      field: 'CREATED_AT',
      direction: 'DESC',
    },
  });

  const logs = data?.edges?.map((edge) => edge.node) || [];

  return (
    <Card className='shadow-soft border-[var(--table-border)] bg-[var(--table-background)]'>
      <CardHeader>
        <div>
          <CardTitle>{t('requests.allLogs.runtime.title')}</CardTitle>
          <CardDescription>{t('requests.allLogs.runtime.description')}</CardDescription>
        </div>
        <CardAction className='flex items-center gap-4'>
          <div className='flex items-center gap-2'>
            <Switch
              id='runtime-logs-include-info'
              checked={includeInfo}
              onCheckedChange={(checked) => {
                setIncludeInfo(checked);
                resetCursor();
              }}
            />
            <label htmlFor='runtime-logs-include-info' className='text-muted-foreground cursor-pointer text-sm'>
              {t('requests.allLogs.runtime.includeInfo')}
            </label>
          </div>
          <Button variant='outline' size='sm' onClick={() => refetch()}>
            <RefreshCw className='mr-2 h-4 w-4' />
            {t('common.refresh')}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className='space-y-3'>
        {isLoading ? (
          Array.from({ length: Math.min(pageSize, 3) }).map((_, index) => <div key={index} className='bg-muted h-28 animate-pulse rounded-xl' />)
        ) : logs.length > 0 ? (
          logs.map((logEntry) => {
            const hasFields = Object.keys(logEntry.fieldsJSON || {}).length > 0;

            return (
              <div key={logEntry.id} className='rounded-xl border border-[var(--table-border)] bg-background/70 p-4'>
                <div className='flex flex-wrap items-center gap-2'>
                  <Badge variant={getLevelBadgeVariant(logEntry.level)}>{logEntry.level.toUpperCase()}</Badge>
                  <Badge variant='outline'>{logEntry.logger}</Badge>
                  {logEntry.channelName && <Badge variant='outline'>{logEntry.channelName}</Badge>}
                  {logEntry.modelID && <Badge variant='outline'>{logEntry.modelID}</Badge>}
                  <span className='text-muted-foreground text-xs'>{logEntry.createdAt.toLocaleString()}</span>
                </div>
                <div className='mt-3 break-words text-sm font-medium'>{logEntry.message}</div>
                <div className='text-muted-foreground mt-3 flex flex-wrap gap-x-4 gap-y-2 text-xs'>
                  {logEntry.caller && (
                    <span>
                      {t('requests.allLogs.runtime.caller')}: {logEntry.caller}
                    </span>
                  )}
                  {logEntry.operationName && (
                    <span>
                      {t('requests.allLogs.runtime.operation')}: {logEntry.operationName}
                    </span>
                  )}
                  {logEntry.traceID && (
                    <span>
                      {t('requests.allLogs.runtime.traceId')}: {logEntry.traceID}
                    </span>
                  )}
                  {logEntry.requestID && (
                    <span>
                      {t('requests.allLogs.runtime.requestId')}: {logEntry.requestID}
                    </span>
                  )}
                  {logEntry.channelID != null && (
                    <span>
                      {t('requests.allLogs.runtime.channelId')}: {logEntry.channelID}
                    </span>
                  )}
                </div>
                {hasFields && (
                  <details className='mt-3'>
                    <summary className='text-muted-foreground cursor-pointer text-sm'>
                      {t('requests.allLogs.runtime.fields')}
                    </summary>
                    <div className='mt-3 rounded-xl border border-[var(--table-border)] bg-muted/30 p-3'>
                      <JsonViewer data={logEntry.fieldsJSON} rootName='' defaultExpanded={false} className='text-xs' />
                    </div>
                  </details>
                )}
              </div>
            );
          })
        ) : (
          <div className='text-muted-foreground rounded-xl border border-dashed px-4 py-10 text-center'>{t('common.noData')}</div>
        )}
      </CardContent>
      <CardFooter>
        <ServerSidePagination
          pageInfo={data?.pageInfo}
          pageSize={pageSize}
          dataLength={logs.length}
          totalCount={data?.totalCount}
          selectedRows={0}
          onNextPage={() => {
            if (data?.pageInfo?.hasNextPage && data.pageInfo.endCursor) {
              setCursors(data.pageInfo.startCursor ?? undefined, data.pageInfo.endCursor ?? undefined, 'after');
            }
          }}
          onPreviousPage={() => {
            if (data?.pageInfo?.hasPreviousPage) {
              setCursors(data.pageInfo.startCursor ?? undefined, data.pageInfo.endCursor ?? undefined, 'before');
            }
          }}
          onPageSizeChange={(nextPageSize) => {
            setPageSize(nextPageSize);
            resetCursor();
          }}
          onFirstPage={cursorHistory.length > 0 ? resetCursor : undefined}
        />
      </CardFooter>
    </Card>
  );
}
