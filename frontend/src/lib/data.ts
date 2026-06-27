import "server-only";
import { cookies } from "next/headers";

const serverApiUrl =
  process.env.API_URL ??
  process.env.NEXT_PUBLIC_API_URL ??
  "http://localhost:8787";

export interface Recording {
  id: string;
  originalName: string;
  mimeType: string;
  sizeBytes: number;
  durationSec?: number | null;
  status: string;
  createdAt: string;
}

export interface AnalysisRun {
  id: string;
  recordingId: string;
  status: string;
  provider: string;
  model?: string | null;
  promptVersion: string;
  normalizationVersion: string;
  groupingVersion: string;
  rawText?: string | null;
  error?: string | null;
  inputTokens?: number | null;
  outputTokens?: number | null;
  thinkingTokens?: number | null;
  totalTokens?: number | null;
  estimatedCostUsd?: number | null;
  pricingVersion?: string | null;
  createdAt: string;
  startedAt?: string | null;
  completedAt?: string | null;
}

export async function apiData<T>(path: string): Promise<T> {
  const token = (await cookies()).get("id_token")?.value;
  const response = await fetch(`${serverApiUrl}${path}`, {
    cache: "no-store",
    headers: {
      Accept: "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
  });
  if (!response.ok) {
    throw new Error(`API ${response.status}: ${path}`);
  }
  return response.json() as Promise<T>;
}

export async function listRecordings(): Promise<Recording[]> {
  const response = await apiData<{ items: Recording[] }>("/v1/recordings");
  return response.items;
}

export async function listRuns(recordingId?: string): Promise<AnalysisRun[]> {
  const query = recordingId
    ? `?recordingId=${encodeURIComponent(recordingId)}`
    : "";
  const response = await apiData<{ items: AnalysisRun[] }>(
    `/v1/analysis-runs${query}`,
  );
  return response.items;
}

export function publicApiUrl(path: string): string {
  const base = process.env.NEXT_PUBLIC_API_URL ?? "/api/backend";
  return `${base}${path}`;
}
