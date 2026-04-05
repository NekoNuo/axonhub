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
      id
      displayModel
      channelID
      actualModelID
      isHealthy
      manualOverride
      probedAt
    }
  }
`;

export const DISCOVERED_MODEL_HEALTH_SNAPSHOTS_QUERY = `
  query DiscoveredModelHealthSnapshots($input: GetDiscoveredModelHealthSnapshotsInput!) {
    discoveredModelHealthSnapshots(input: $input) {
      id
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
      id
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

export const RESET_DISCOVERED_MODEL_HEALTH_MUTATION = `
  mutation ResetDiscoveredModelHealth {
    resetDiscoveredModelHealth
  }
`;

export function parseModelHealthSnapshots(data: unknown[]): ModelHealthSnapshot[] {
  return modelHealthSnapshotSchema.array().parse(data);
}

export function parseDiscoveredModelHealthSnapshots(data: unknown[]): ModelHealthSnapshot[] {
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

export async function fetchDiscoveredModelHealthSnapshots(variables: { input: { displayModels?: string[] } }) {
  const data = await graphqlRequest<{ discoveredModelHealthSnapshots: unknown[] }>(DISCOVERED_MODEL_HEALTH_SNAPSHOTS_QUERY, variables);
  return parseDiscoveredModelHealthSnapshots(data.discoveredModelHealthSnapshots || []);
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

export async function resetDiscoveredModelHealth() {
  const data = await graphqlRequest<{ resetDiscoveredModelHealth: boolean }>(RESET_DISCOVERED_MODEL_HEALTH_MUTATION);
  return data.resetDiscoveredModelHealth;
}
