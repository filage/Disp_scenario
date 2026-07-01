import { describe, expect, it } from "vitest";
import { ACTION_OPTIONS } from "../events/types";
import { actionDescription } from "./action-description";

describe("actionDescription", () => {
  it("explains canonical actions in Russian", () => {
    expect(actionDescription("OPEN_ORDER")).toBe("Открыл заказ");
    expect(actionDescription("SELECT_DRIVER")).toBe("Выбрал курьера");
  });

  it("provides a readable fallback for an unknown action", () => {
    expect(actionDescription("CUSTOM_ACTION")).toBe("Другое действие сценария");
  });

  it("covers every action available in the editor", () => {
    for (const action of ACTION_OPTIONS) {
      expect(actionDescription(action)).not.toBe("Другое действие сценария");
    }
  });
});
