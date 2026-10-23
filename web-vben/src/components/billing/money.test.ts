import { describe, expect, it } from "vitest";
import { formatCents, parseCents } from "./money";
describe("integer-cent input", () => {
  it("parses exact decimal strings without rounding", () => {
    expect(parseCents("0.01")).toBe(1);
    expect(parseCents("1.10")).toBe(110);
    expect(parseCents("10000000000.00")).toBe(1_000_000_000_000);
    expect(formatCents(101)).toBe("1.01");
    expect(formatCents(-1200)).toBe("-12.00");
  });
  it("rejects exponential, fractional-cent, negative and excessive inputs", () => {
    for (const input of ["1e3", "NaN", "Infinity", "1.001", "-1", "10000000000.01", "", "00.5"])
      expect(parseCents(input), input).toBeNull();
  });
});
