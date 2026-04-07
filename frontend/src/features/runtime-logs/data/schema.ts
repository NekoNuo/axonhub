import { z } from 'zod';
import { pageInfoSchema } from '@/gql/pagination';

export const runtimeLogLevelSchema = z.enum(['debug', 'info', 'warn', 'error']);

export const runtimeLogSchema = z.object({
  id: z.string(),
  createdAt: z.coerce.date(),
  updatedAt: z.coerce.date(),
  logger: z.string(),
  level: runtimeLogLevelSchema,
  message: z.string(),
  caller: z.string().nullable().optional(),
  traceID: z.string().nullable().optional(),
  requestID: z.string().nullable().optional(),
  operationName: z.string().nullable().optional(),
  channelID: z.number().nullable().optional(),
  channelName: z.string().nullable().optional(),
  modelID: z.string().nullable().optional(),
  fieldsJSON: z.record(z.string(), z.unknown()).default({}),
});

export type RuntimeLog = z.infer<typeof runtimeLogSchema>;

export const runtimeLogConnectionSchema = z.object({
  edges: z.array(
    z.object({
      node: runtimeLogSchema,
      cursor: z.string(),
    })
  ),
  pageInfo: pageInfoSchema,
  totalCount: z.number(),
});

export type RuntimeLogConnection = z.infer<typeof runtimeLogConnectionSchema>;
