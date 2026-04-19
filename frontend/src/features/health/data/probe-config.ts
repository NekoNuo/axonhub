import { graphqlRequest } from '@/gql/graphql';
import {
  batchSetChannelProbeEnabledInputSchema,
  modelProbeConfigSchema,
  setModelProbeEnabledInputSchema,
  type BatchSetChannelProbeEnabledInput,
  type ModelProbeConfig,
  type SetModelProbeEnabledInput,
} from './schema';

const MODEL_PROBE_CONFIGS_QUERY = `
  query ModelProbeConfigs($input: GetModelProbeConfigsInput!) {
    modelProbeConfigs(input: $input) {
      displayModel
      channelID
      actualModelID
      probeEnabled
      consecutiveFailures
      autoDisabledAt
    }
  }
`;

const SET_MODEL_PROBE_ENABLED_MUTATION = `
  mutation SetModelProbeEnabled($input: SetModelProbeEnabledInput!) {
    setModelProbeEnabled(input: $input) {
      displayModel
      channelID
      actualModelID
      probeEnabled
      consecutiveFailures
      autoDisabledAt
    }
  }
`;

const BATCH_SET_CHANNEL_PROBE_ENABLED_MUTATION = `
  mutation BatchSetChannelProbeEnabled($input: BatchSetChannelProbeEnabledInput!) {
    batchSetChannelProbeEnabled(input: $input) {
      displayModel
      channelID
      actualModelID
      probeEnabled
      consecutiveFailures
      autoDisabledAt
    }
  }
`;

function parseModelProbeConfigs(data: unknown[]): ModelProbeConfig[] {
  return modelProbeConfigSchema.array().parse(data);
}

export async function fetchModelProbeConfigs(variables: { input: { displayModels?: string[] } }) {
  const data = await graphqlRequest<{ modelProbeConfigs: unknown[] }>(MODEL_PROBE_CONFIGS_QUERY, variables);
  return parseModelProbeConfigs(data.modelProbeConfigs || []);
}

export async function setModelProbeEnabled(input: SetModelProbeEnabledInput) {
  const data = await graphqlRequest<{ setModelProbeEnabled: unknown }>(SET_MODEL_PROBE_ENABLED_MUTATION, {
    input: setModelProbeEnabledInputSchema.parse(input),
  });
  return modelProbeConfigSchema.parse(data.setModelProbeEnabled);
}

export async function batchSetChannelProbeEnabled(input: BatchSetChannelProbeEnabledInput) {
  const data = await graphqlRequest<{ batchSetChannelProbeEnabled: unknown[] }>(
    BATCH_SET_CHANNEL_PROBE_ENABLED_MUTATION,
    { input: batchSetChannelProbeEnabledInputSchema.parse(input) }
  );
  return parseModelProbeConfigs(data.batchSetChannelProbeEnabled || []);
}
