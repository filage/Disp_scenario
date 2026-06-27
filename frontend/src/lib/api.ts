import "server-only";
import { apiData, type Recording } from "@/lib/data";

type Health = {
  status: string;
  version: string;
  dependencies: Record<string, string>;
};

export async function getSystemSnapshot() {
  const [health, recordingResponse] = await Promise.all([
    apiData<Health>("/health"),
    apiData<{ items: Recording[] }>("/v1/recordings"),
  ]);

  return {
    health,
    recordings: recordingResponse.items,
    unavailable: health.status !== "ok",
  };
}
