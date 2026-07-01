import { describe, expect, it } from "vitest";
import { formatDuration, formatIssueType, formatPercent } from "./display";

describe("display formatting", () => {
  it("formats duration without negative output", () => {
    expect(formatDuration(65_000)).toBe("1 мин 5 с");
    expect(formatDuration(-1)).toBe("0 с");
  });

  it("clamps percentages", () => {
    expect(formatPercent(0.824)).toBe("82%");
    expect(formatPercent(2)).toBe("100%");
  });

  it("formats routine scenario categories", () => {
    expect(formatIssueType("Delivery destination change")).toBe(
      "Смена точки окончания доставки",
    );
    expect(formatIssueType("Recipient contact update")).toBe(
      "Обновление контакта получателя",
    );
    expect(formatIssueType("Delivery note update")).toBe(
      "Добавление комментария к доставке",
    );
  });
});
