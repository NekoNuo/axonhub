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
