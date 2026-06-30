import { afterEach, describe, expect, it, vi } from "vitest";
import { createSession, verifySession } from "./session-core";

describe("signed application session", () => {
  afterEach(() => vi.unstubAllEnvs());

  it("round-trips a signed session", () => {
    vi.stubEnv("AUTH_SESSION_SECRET", "test-session-secret");
    const session = verifySession(createSession("demo-user"));
    expect(session?.subject).toBe("demo-user");
  });

  it("rejects a modified session", () => {
    vi.stubEnv("AUTH_SESSION_SECRET", "test-session-secret");
    const token = createSession("demo-user");
    expect(verifySession(`${token.slice(0, -1)}x`)).toBeNull();
  });
});
