import { describe, expect, it } from "vitest";
import { parseQueryV1, serializeQueryV1 } from "./query";

describe("query v1 strict grammar", () => {
  it.each([
    ["暑假 format:JPEG,png format:jpg sort:NEWEST", '"暑假" format:jpg,png'],
    ["extension:JPEG format:png order:earliest", "format:jpg,png sort:oldest"],
    [
      'is:PRIVATE camera:"Canon EOS" minsize:0.000001MB',
      'camera:"canon eos" minsize:0.000001MB visibility:private',
    ],
    ['album:"A,B",#9007199254740993,unfiled', 'album:unfiled,#9007199254740993,"A,B"'],
    ["CAFÉ cafe\u0301 Ａ A a", '"cafÉ" "café" "Ａ" "a"'],
  ])("normalizes %s", (raw, canonical) => {
    const result = parseQueryV1(raw);
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(serializeQueryV1(result.ast)).toBe(canonical);
    const roundtrip = parseQueryV1(serializeQueryV1(result.ast));
    expect(roundtrip.ok && serializeQueryV1(roundtrip.ast)).toBe(canonical);
  });
  it.each([
    ["format:", "MISSING_VALUE"],
    ["format:jpg, png", "EMPTY_LIST_ITEM"],
    ['album:"a\\q"', "INVALID_ESCAPE"],
    ['camera:""', "MISSING_VALUE"],
    ['ab"cd"', "UNEXPECTED_CHARACTER"],
    ["after:2026-02-29", "INVALID_DATE"],
    ["after:1969-12-31", "INVALID_DATE"],
    ["minsize:1MiB", "INVALID_SIZE"],
    ["minsize:0.0000001MB", "INVALID_SIZE"],
    ["is:all", "INVALID_ENUM"],
    ["order:newest", "INVALID_ENUM"],
    ["name:张三", "UNKNOWN_FIELD"],
    ["is:private visibility:private", "DUPLICATE_FIELD"],
    ["after:2026-10-02 before:2026-10-02", "RANGE_CONFLICT"],
    ["format：jpg", "UNEXPECTED_CHARACTER"],
    ["a\u0000", "CONTROL_CHARACTER"],
    [
      Array(9)
        .fill(0)
        .map((_, i) => `word${i}`)
        .join(" "),
      "QUERY_TOO_COMPLEX",
    ],
    ["album:#01", "INVALID_ENUM"],
  ])("rejects %s", (raw, code) => {
    const result = parseQueryV1(raw);
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.diagnostics[0]?.code).toBe(code);
  });
  it("reports original UTF16 offsets", () => {
    const result = parseQueryV1("😀 foo:bar");
    expect(result).toMatchObject({
      ok: false,
      diagnostics: [{ code: "UNKNOWN_FIELD", span: { start: 3, end: 6 } }],
    });
  });
});

import fixtures from "../../../../internal/searchquery/testdata/query-v1.json";
describe("shared Go/TypeScript fixture contract", () => {
  for (const c of fixtures.cases)
    it(c.id, () => {
      const result = parseQueryV1(c.raw);
      if ("diagnostics" in c && c.diagnostics) {
        expect(result).toMatchObject({ ok: false, diagnostics: c.diagnostics });
      } else {
        expect(result.ok).toBe(true);
        if (!result.ok) return;
        expect(result.ast).toEqual(c.ast);
        expect(serializeQueryV1(result.ast)).toBe(c.canonical);
      }
    });
});

it("leaves other hash-prefixed album values as exact names", () => {
  const parsed = parseQueryV1("album:#trip");
  expect(parsed).toMatchObject({
    ok: true,
    ast: { filters: { albums: [{ kind: "name", value: "#trip" }] } },
  });
});
it.each([
  ["0x:foo", "UNEXPECTED_CHARACTER", { start: 0, end: 2 }],
  ["foo1:bar", "UNKNOWN_FIELD", { start: 0, end: 4 }],
  ["foo1：bar", "UNEXPECTED_CHARACTER", { start: 4, end: 5 }],
  ["fo\u0000o:bar", "CONTROL_CHARACTER", { start: 2, end: 3 }],
])("validates field scanner for %s", (raw, code, span) => {
  expect(parseQueryV1(raw)).toMatchObject({ ok: false, diagnostics: [{ code, span }] });
});
