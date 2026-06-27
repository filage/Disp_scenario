import { describe, expect, it } from "vitest";
import { createScenarioBoundaryLookup } from "./scenario-boundaries";
import type { ActionEvent, ScenarioInstance } from "./types";

const events: ActionEvent[] = [
  event("event-1", 1_000),
  event("event-2", 2_000),
  event("event-3", 3_000),
];

describe("createScenarioBoundaryLookup", () => {
  it("uses scenario event IDs for exact start and end markers", () => {
    const boundaries = createScenarioBoundaryLookup(events, [
      scenario(["event-1", "event-2"]),
    ]);

    expect(boundaries.startsByEventId.get("event-1")?.[0]).toMatchObject({
      kind: "start",
      title: "Начало сценария",
    });
    expect(boundaries.endsByEventId.get("event-2")?.[0]).toMatchObject({
      kind: "end",
      title: "Конец сценария",
    });
  });

  it("falls back to the closest event when event IDs are unavailable", () => {
    const boundaries = createScenarioBoundaryLookup(events, [
      scenario([], 1_100, 2_900),
    ]);

    expect(boundaries.startsByEventId.has("event-1")).toBe(true);
    expect(boundaries.endsByEventId.has("event-3")).toBe(true);
  });
});

function event(id: string, timestampMs: number): ActionEvent {
  return {
    id,
    recordingId: "recording-1",
    timestampMs,
    canonicalAction: "CHECK",
    eventType: "click",
    screen: "Orders",
    target: "Order",
    confidence: 0.9,
    version: 1,
  };
}

function scenario(
  eventIds: string[],
  startedAtMs = 1_000,
  endedAtMs = 2_000,
): ScenarioInstance {
  return {
    id: "scenario-1",
    recordingId: "recording-1",
    issueType: "Late pickup",
    startedAtMs,
    endedAtMs,
    durationMs: endedAtMs - startedAtMs,
    eventIds,
    outcome: "resolved",
    status: "completed",
    confidence: 0.9,
  };
}
