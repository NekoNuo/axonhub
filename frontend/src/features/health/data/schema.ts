import { z } from 'zod';

export const modelHealthSnapshotSchema = z.object({
  id: z.string().optional(),
  displayModel: z.string(),
  channelID: z.string(),
  actualModelID: z.string(),
  isHealthy: z.boolean(),
  manualOverride: z.boolean(),
  probedAt: z.number(),
});
export type ModelHealthSnapshot = z.infer<typeof modelHealthSnapshotSchema>;

export const modelHealthHistorySchema = z.object({
  id: z.string().optional(),
  displayModel: z.string(),
  channelID: z.string(),
  actualModelID: z.string(),
  isHealthy: z.boolean(),
  manualOverride: z.boolean(),
  probedAt: z.number(),
});
export type ModelHealthHistory = z.infer<typeof modelHealthHistorySchema>;

export const manualModelProbeInputSchema = z.object({
  displayModel: z.string(),
  actualModelID: z.string(),
  channelID: z.string(),
});
export type ManualModelProbeInput = z.infer<typeof manualModelProbeInputSchema>;

export const modelSettingsSchema = z.object({
  enableModelProbe: z.boolean(),
});
export type ModelProbeSettings = z.infer<typeof modelSettingsSchema>;

export const modelProbeConfigSchema = z.object({
  displayModel: z.string(),
  channelID: z.string(),
  actualModelID: z.string(),
  probeEnabled: z.boolean(),
  consecutiveFailures: z.number(),
  autoDisabledAt: z.number().nullable().optional(),
});
export type ModelProbeConfig = z.infer<typeof modelProbeConfigSchema>;

export const setModelProbeEnabledInputSchema = z.object({
  displayModel: z.string(),
  channelID: z.string(),
  actualModelID: z.string(),
  enabled: z.boolean(),
});
export type SetModelProbeEnabledInput = z.infer<typeof setModelProbeEnabledInputSchema>;

export const batchSetChannelProbeEnabledInputSchema = z.object({
  displayModel: z.string(),
  channelID: z.string(),
  enabled: z.boolean(),
});
export type BatchSetChannelProbeEnabledInput = z.infer<typeof batchSetChannelProbeEnabledInputSchema>;
