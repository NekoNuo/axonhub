import { useState, useCallback } from 'react';
import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { buildDateRangeWhereClause, type DateTimeRangeValue } from '@/utils/date-range';
import { useDebounce } from '@/hooks/use-debounce';
import { usePaginationSearch } from '@/hooks/use-pagination-search';
import { Button } from '@/components/ui/button';
import { DateRangePicker } from '@/components/date-range-picker';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import useInterval from '@/hooks/useInterval';
import { Header } from '@/components/layout/header';
import { Main } from '@/components/layout/main';
import { TracesTable } from '@/features/traces';
import { useTraces } from '@/features/traces/data';
import { RuntimeLogsSection } from '@/features/runtime-logs';
import { RequestsTable } from './components';
import { RequestsProvider } from './context';
import { useRequests } from './data';
import { getRequestLogsViewMeta, isRequestLogsViewMode, REQUEST_LOGS_VIEW_MODES, type RequestLogsViewMode } from './view-mode';

function RequestsContent() {
  const { pageSize, setCursors, setPageSize, resetCursor, paginationArgs, cursorHistory } = usePaginationSearch({
    defaultPageSize: 20,
    pageSizeStorageKey: 'requests-table-page-size',
  });
  const [statusFilter, setStatusFilter] = useState<string[]>([]);
  const [sourceFilter, setSourceFilter] = useState<string[]>([]);
  const [channelFilter, setChannelFilter] = useState<string[]>([]);
  const [apiKeyFilter, setApiKeyFilter] = useState<string[]>([]);
  const [modelIDFilter, setModelIDFilter] = useState<string>('');
  const debouncedModelIDFilter = useDebounce(modelIDFilter, 300);
  const [dateRange, setDateRange] = useState<DateTimeRangeValue | undefined>();
  const [autoRefresh, setAutoRefresh] = useState(false);

  // Build where clause with filters
  const whereClause = (() => {
    const where: { [key: string]: any } = {
      ...buildDateRangeWhereClause(dateRange),
    };
    if (statusFilter.length > 0) {
      where.statusIn = statusFilter;
    }
    if (sourceFilter.length > 0) {
      where.sourceIn = sourceFilter;
    }
    if (channelFilter.length > 0) {
      where.channelIDIn = channelFilter;
    }
    if (apiKeyFilter.length > 0) {
      where.apiKeyIDIn = apiKeyFilter;
    }
    if (debouncedModelIDFilter) {
      where.modelIDContainsFold = debouncedModelIDFilter;
    }
    return Object.keys(where).length > 0 ? where : undefined;
  })();

  const { data, isLoading, refetch } = useRequests({
    ...paginationArgs,
    where: whereClause,
    orderBy: {
      field: 'CREATED_AT',
      direction: 'DESC',
    },
  });

  const requests = data?.edges?.map((edge) => edge.node) || [];
  const pageInfo = data?.pageInfo;

  const isFirstPage = !paginationArgs.after && cursorHistory.length === 0;

  useInterval(
    () => {
      refetch();
    },
    autoRefresh && isFirstPage ? 10000 : null
  );

  const handleNextPage = () => {
    if (data?.pageInfo?.hasNextPage && data?.pageInfo?.endCursor) {
      setCursors(data.pageInfo.startCursor ?? undefined, data.pageInfo.endCursor ?? undefined, 'after');
    }
  };

  const handlePreviousPage = () => {
    if (data?.pageInfo?.hasPreviousPage) {
      setCursors(data.pageInfo.startCursor ?? undefined, data.pageInfo.endCursor ?? undefined, 'before');
    }
  };

  const handlePageSizeChange = (newPageSize: number) => {
    setPageSize(newPageSize);
    resetCursor();
  };

  const handleStatusFilterChange = useCallback(
    (filters: string[]) => {
      setStatusFilter(filters);
      resetCursor();
    },
    [resetCursor]
  );

  const handleSourceFilterChange = useCallback(
    (filters: string[]) => {
      setSourceFilter(filters);
      resetCursor();
    },
    [resetCursor]
  );

  const handleChannelFilterChange = useCallback(
    (filters: string[]) => {
      setChannelFilter(filters);
      resetCursor();
    },
    [resetCursor]
  );

  const handleApiKeyFilterChange = useCallback(
    (filters: string[]) => {
      setApiKeyFilter(filters);
      resetCursor();
    },
    [resetCursor]
  );

  const handleModelIDFilterChange = useCallback(
    (filter: string) => {
      setModelIDFilter(filter);
      resetCursor();
    },
    [resetCursor]
  );

  const handleDateRangeChange = useCallback(
    (range: DateTimeRangeValue | undefined) => {
      setDateRange(range);
      resetCursor();
    },
    [resetCursor]
  );

  return (
    <div className='flex flex-1 flex-col overflow-hidden'>
      <RequestsTable
        data={requests}
        loading={isLoading}
        pageInfo={pageInfo}
        pageSize={pageSize}
        totalCount={data?.totalCount}
        statusFilter={statusFilter}
        sourceFilter={sourceFilter}
        channelFilter={channelFilter}
        apiKeyFilter={apiKeyFilter}
        dateRange={dateRange}
        queryWhere={whereClause}
        onNextPage={handleNextPage}
        onPreviousPage={handlePreviousPage}
        onPageSizeChange={handlePageSizeChange}
        onStatusFilterChange={handleStatusFilterChange}
        onSourceFilterChange={handleSourceFilterChange}
        onChannelFilterChange={handleChannelFilterChange}
        onApiKeyFilterChange={handleApiKeyFilterChange}
        onModelIDFilterChange={handleModelIDFilterChange}
        onDateRangeChange={handleDateRangeChange}
        onRefresh={refetch}
        showRefresh={isFirstPage}
        autoRefresh={autoRefresh}
        onAutoRefreshChange={setAutoRefresh}
      />
    </div>
  );
}

function AllLogsTraceSection({
  dateRange,
  onDateRangeChange,
}: {
  dateRange?: DateTimeRangeValue;
  onDateRangeChange: (range: DateTimeRangeValue | undefined) => void;
}) {
  const { t } = useTranslation();
  const { pageSize, setCursors, setPageSize, resetCursor, paginationArgs, cursorHistory } = usePaginationSearch({
    defaultPageSize: 20,
    pageSizeStorageKey: 'requests-all-traces-table-page-size',
    startCursorKey: 'allTracesStartCursor',
    endCursorKey: 'allTracesEndCursor',
    pageSizeKey: 'allTracesPageSize',
    directionKey: 'allTracesDirection',
    cursorHistoryKey: 'allTracesCursorHistory',
  });
  const [traceIdFilter, setTraceIdFilter] = useState('');
  const [autoRefresh, setAutoRefresh] = useState(false);
  const debouncedTraceIdFilter = useDebounce(traceIdFilter, 300);

  const whereClause = (() => {
    const where: { [key: string]: any } = {
      ...buildDateRangeWhereClause(dateRange),
    };

    if (debouncedTraceIdFilter.trim()) {
      where.traceIDContains = debouncedTraceIdFilter.trim();
    }

    return Object.keys(where).length > 0 ? where : undefined;
  })();

  const { data, isLoading, refetch } = useTraces({
    ...paginationArgs,
    where: whereClause,
    orderBy: {
      field: 'CREATED_AT',
      direction: 'DESC',
    },
  });

  const traces = data?.edges?.map((edge) => edge.node) || [];
  const pageInfo = data?.pageInfo;
  const isFirstPage = !paginationArgs.after && cursorHistory.length === 0;

  useInterval(
    () => {
      refetch();
    },
    autoRefresh && isFirstPage ? 30000 : null
  );

  return (
    <section className='space-y-3'>
      <div>
        <h3 className='text-base font-semibold'>{t('requests.allLogs.traces.title')}</h3>
        <p className='text-muted-foreground text-sm'>{t('requests.allLogs.traces.description')}</p>
      </div>
      <TracesTable
        data={traces}
        loading={isLoading}
        pageInfo={pageInfo}
        pageSize={pageSize}
        totalCount={data?.totalCount}
        dateRange={dateRange}
        showDateRangeFilter={false}
        traceIdFilter={traceIdFilter}
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
        onDateRangeChange={onDateRangeChange}
        onTraceIdFilterChange={(value) => {
          setTraceIdFilter(value);
          resetCursor();
        }}
        onRefresh={refetch}
        showRefresh={isFirstPage}
        autoRefresh={autoRefresh}
        onAutoRefreshChange={setAutoRefresh}
      />
    </section>
  );
}

function AllLogsContent() {
  const { t } = useTranslation();
  const [dateRange, setDateRange] = useState<DateTimeRangeValue | undefined>();
  const hasDateRange = !!dateRange?.from || !!dateRange?.to;

  return (
    <div className='flex flex-1 flex-col gap-6 overflow-auto'>
      <div className='shadow-soft flex flex-wrap items-center gap-3 rounded-2xl border border-[var(--table-border)] bg-background px-4 py-3'>
        <span className='text-sm font-medium'>{t('requests.allLogs.sharedDateRange')}</span>
        <DateRangePicker value={dateRange} onChange={setDateRange} />
        {hasDateRange && (
          <Button variant='ghost' size='sm' onClick={() => setDateRange(undefined)} className='h-8 px-2'>
            <X className='h-4 w-4' />
          </Button>
        )}
      </div>
      <AllLogsTraceSection dateRange={dateRange} onDateRangeChange={setDateRange} />
      <RuntimeLogsSection dateRange={dateRange} />
    </div>
  );
}

export default function RequestsManagement() {
  const { t } = useTranslation();
  const [viewMode, setViewMode] = useState<RequestLogsViewMode>('requests');
  const viewMeta = getRequestLogsViewMeta(viewMode);

  return (
    <RequestsProvider>
      <Header fixed>
        <div className='flex flex-1 items-center justify-between gap-4'>
          <div>
            <h2 className='text-xl font-bold tracking-tight'>{t(viewMeta.titleKey)}</h2>
            <p className='text-muted-foreground text-sm'>{t(viewMeta.descriptionKey)}</p>
          </div>
          <Tabs
            value={viewMode}
            onValueChange={(value) => {
              if (isRequestLogsViewMode(value)) {
                setViewMode(value);
              }
            }}
            className='gap-0'
          >
            <TabsList className='shadow-soft border-border bg-background grid w-full grid-cols-2 rounded-2xl border'>
              {REQUEST_LOGS_VIEW_MODES.map((mode) => {
                const modeMeta = getRequestLogsViewMeta(mode);
                return (
                  <TabsTrigger key={mode} value={mode} className='min-w-28'>
                    {t(modeMeta.modeLabelKey)}
                  </TabsTrigger>
                );
              })}
            </TabsList>
          </Tabs>
        </div>
      </Header>

      <Main fixed>
        {viewMode === 'requests' ? <RequestsContent /> : <AllLogsContent />}
      </Main>
    </RequestsProvider>
  );
}
