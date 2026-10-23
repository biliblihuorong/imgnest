const MAX_CENTS = 1_000_000_000_000;
/** Parse whole cents, rejecting rounding, exponential notation and negative input. */
export function parseCents(input: string): number | null {
  const value = input.trim();
  if (!/^(?:0|[1-9]\d*)(?:\.\d{1,2})?$/.test(value)) return null;
  const [whole = "0", fraction = ""] = value.split(".");
  if (whole.length > 11) return null;
  const cents = Number(whole) * 100 + Number(fraction.padEnd(2, "0"));
  return Number.isSafeInteger(cents) && cents <= MAX_CENTS ? cents : null;
}
export function formatCents(cents: number): string {
  const value = Math.abs(cents);
  return `${cents < 0 ? "-" : ""}${Math.trunc(value / 100)}.${String(value % 100).padStart(2, "0")}`;
}
