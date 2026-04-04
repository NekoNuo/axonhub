import { graphqlRequest } from '@/gql/graphql';
import {
  manualModelProbeInputSchema,
  modelHealthHistorySchema,
  modelHealthSnapshotSchema,
  type ManualModelProbeInput,
  type ModelHealthHistory,
  type ModelHealthSnapshot,
} from './schema';

export const MODEL_HEALTH_SNAPSHOTS_QUERY = `
  query ModelHealthSnapshots($input: GetModelHealthSnapshotsInput!) {
    modelHealthSnapshots(input: $input) {
      displayModel
      channelID
      actualModelID
      isHealthy
      manualOverride
      probedAt
    }
  }
`;

export const MODEL_HEALTH_HISTORY_QUERY = `
  query ModelHealthHistory($input: GetModelHealthHistoryInput!) {
    modelHealthHistory(input: $input) {
      displayModel
      channelID
      actualModelID
      isHealthy
      manualOverride
      probedAt
    }
  }
`;

export const MANUAL_MODEL_PROBE_MUTATION = `
  mutation ManualModelProbe($input: ManualModelProbeInput!) {
    manualModelProbe(input: $input)
  }
`;

export function parseModelHealthSnapshots(data: unknown[]): ModelHealthSnapshot[] {
  return modelHealthSnapshotSchema.array().parse(data);
}

export function parseModelHealthHistory(data: unknown[]): ModelHealthHistory[] {
  return modelHealthHistorySchema.array().parse(data);
}

export function buildManualModelProbeVariables(input: ManualModelProbeInput) {
  return {
    input: manualModelProbeInputSchema.parse(input),
  };
}

export async function fetchModelHealthSnapshots(variables: { input: { displayModels?: string[] } }) {
  const data = await graphqlRequest<{ modelHealthSnapshots: unknown[] }>(MODEL_HEALTH_SNAPSHOTS_QUERY, variables);
  return parseModelHealthSnapshots(data.modelHealthSnapshots || []);
}

export async function fetchModelHealthHistory(variables: {
  input: { displayModel?: string; actualModelID?: string; channelIDs?: string[] };
}) {
  const data = await graphqlRequest<{ modelHealthHistory: unknown[] }>(MODEL_HEALTH_HISTORY_QUERY, variables);
  return parseModelHealthHistory(data.modelHealthHistory || []);
}

export async function manualModelProbe(input: ManualModelProbeInput) {
  const data = await graphqlRequest<{ manualModelProbe: boolean }>(
    MANUAL_MODEL_PROBE_MUTATION,
    buildManualModelProbeVariables(input)
  );
  return data.manualModelProbe;
}
