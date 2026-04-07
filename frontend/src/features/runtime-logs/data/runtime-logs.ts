import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { graphqlRequest } from '@/gql/graphql';
import { useErrorHandler } from '@/hooks/use-error-handler';
import { useSelectedProjectId } from '@/stores/projectStore';
import { runtimeLogConnectionSchema, type RuntimeLogConnection } from './schema';

function buildRuntimeLogsQuery() {
  return `
    query GetRuntimeLogs(
      $first: Int
      $after: Cursor
      $last: Int
      $before: Cursor
      $orderBy: RuntimeLogOrder
      $where: RuntimeLogWhereInput
    ) {
      runtimeLogs(first: $first, after: $after, last: $last, before: $before, orderBy: $orderBy, where: $where) {
        edges {
          node {
            id
            createdAt
            updatedAt
            logger
            level
            message
            caller
            traceID
            requestID
            operationName
            channelID
            channelName
            modelID
            fieldsJSON
          }
          cursor
        }
        pageInfo {
          hasNextPage
          hasPreviousPage
          startCursor
          endCursor
        }
        totalCount
      }
    }
  `;
}

export function useRuntimeLogs(variables?: {
  first?: number;
  after?: string;
  last?: number;
  before?: string;
  orderBy?: { field: 'CREATED_AT' | 'UPDATED_AT'; direction: 'ASC' | 'DESC' };
  where?: Record<string, any>;
}) {
  const { handleError } = useErrorHandler();
  const { t } = useTranslation();
  const selectedProjectId = useSelectedProjectId();

  return useQuery({
    queryKey: ['runtime-logs', variables, selectedProjectId],
    queryFn: async () => {
      try {
        const headers = selectedProjectId ? { 'X-Project-ID': selectedProjectId } : undefined;
        const data = await graphqlRequest<{ runtimeLogs: RuntimeLogConnection }>(buildRuntimeLogsQuery(), variables, headers);
        return runtimeLogConnectionSchema.parse(data?.runtimeLogs);
      } catch (error) {
        handleError(error, t('common.errors.internalServerError'));
        throw error;
      }
    },
    enabled: true,
  });
}
