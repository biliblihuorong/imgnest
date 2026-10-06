/** ImageNest query v1.1: pure, bounded scanner/parser; all offsets are original UTF-16. */
export const FORMATS = [
  "jpg",
  "png",
  "gif",
  "webp",
  "avif",
  "bmp",
  "tiff",
  "svg",
  "heic",
  "heif",
  "unknown",
] as const;
export const SORTS = [
  "newest",
  "oldest",
  "size-desc",
  "size-asc",
  "name-asc",
  "name-desc",
] as const;
export const FIELDS = [
  "format",
  "album",
  "camera",
  "minsize",
  "maxsize",
  "after",
  "before",
  "visibility",
  "sort",
] as const;
export type Field = (typeof FIELDS)[number];
export type Span = { start: number; end: number };
export type Diagnostic = {
  code: string;
  messageKey: string;
  span: Span;
  args: Record<string, unknown>;
};
export type AlbumTerm = { kind: "name" | "id"; value: string } | { kind: "unfiled" };
export interface QueryAstV1 {
  version: 1;
  terms: string[];
  filters: {
    formats: (typeof FORMATS)[number][];
    albums: AlbumTerm[];
    camera: string | null;
    minBytes: string | null;
    maxBytes: string | null;
    after: string | null;
    before: string | null;
    visibility: "all" | "public" | "private";
  };
  sort: (typeof SORTS)[number];
}
export type Token = { field: Field | "term"; span: Span; valueSpan: Span; raw: string };
export type ParseResult =
  { ok: true; ast: QueryAstV1; tokens: Token[] } | { ok: false; diagnostics: Diagnostic[] };
const ALIASES: Record<string, Field> = { extension: "format", is: "visibility", order: "sort" };
const ORDERS: Record<string, QueryAstV1["sort"]> = {
  earliest: "oldest",
  utmost: "size-desc",
  least: "size-asc",
  created_at: "newest",
  "created_at-asc": "oldest",
};
const whitespace = (c: string) => /^[\t\n\v\f\r \u00a0\u3000]$/.test(c);
export const foldText = (value: string): string =>
  value.normalize("NFC").replace(/[A-Z]/g, (c) => c.toLowerCase());
export const quoteText = (value: string): string =>
  `"${value.replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`;
export function diagnostic(
  code: string,
  start: number,
  end: number,
  args: Record<string, unknown> = {},
): Diagnostic {
  const camel = code.toLowerCase().replace(/_([a-z])/g, (_, c: string) => c.toUpperCase());
  return { code, messageKey: `search.error.${camel}`, span: { start, end }, args };
}
function emptyAst(): QueryAstV1 {
  return {
    version: 1,
    terms: [],
    filters: {
      formats: [],
      albums: [],
      camera: null,
      minBytes: null,
      maxBytes: null,
      after: null,
      before: null,
      visibility: "all",
    },
    sort: "newest",
  };
}
export function validDate(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const y = Number(value.slice(0, 4)),
    m = Number(value.slice(5, 7)),
    d = Number(value.slice(8, 10));
  if (y < 1970 || y > 9999 || m < 1 || m > 12 || d < 1) return false;
  const days = [
    31,
    y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0) ? 29 : 28,
    31,
    30,
    31,
    30,
    31,
    31,
    30,
    31,
    30,
    31,
  ];
  return d <= days[m - 1]!;
}
function sizeBytes(value: string): string | null {
  const match = /^(\d+)(?:\.(\d{1,6}))?MB$/i.exec(value);
  if (!match) return null;
  const bytes = BigInt(match[1]!) * 1000000n + BigInt((match[2] ?? "").padEnd(6, "0"));
  return bytes <= 1024000000000n ? String(bytes) : null;
}
export function bytesMB(bytes: string): string {
  const n = BigInt(bytes),
    whole = n / 1000000n;
  const fraction = String(n % 1000000n)
    .padStart(6, "0")
    .replace(/0+$/, "");
  return `${whole}${fraction ? `.${fraction}` : ""}MB`;
}
export function parseQueryV1(raw: string): ParseResult {
  // Throw only private diagnostic values, never expose an executable partial AST.
  const fail = (
    code: string,
    start: number,
    end: number,
    args: Record<string, unknown> = {},
  ): never => {
    throw diagnostic(code, start, end, args);
  };
  try {
    if (new TextEncoder().encode(raw).length > 4096) fail("QUERY_TOO_COMPLEX", 0, raw.length);
    const spans: Span[] = [];
    let i = 0;
    while (i < raw.length) {
      if (whitespace(raw[i]!)) {
        i++;
        continue;
      }
      const start = i;
      let quoted = false,
        escaped = false;
      while (i < raw.length && (quoted || !whitespace(raw[i]!))) {
        if (/^[\x00-\x1f\x7f-\x9f]$/.test(raw[i]!) && (quoted || !whitespace(raw[i]!)))
          fail("CONTROL_CHARACTER", i, i + 1);
        if (escaped) {
          escaped = false;
          i++;
          continue;
        }
        if (raw[i] === "\\" && quoted) {
          escaped = true;
          i++;
          continue;
        }
        if (raw[i] === '"') quoted = !quoted;
        i++;
      }
      spans.push({ start, end: i });
    }
    if (spans.length > 32) fail("QUERY_TOO_COMPLEX", spans[32]!.start, spans[32]!.end);
    const ast = emptyAst(),
      tokens: Token[] = [],
      seen = new Map<string, Span>();
    let termCount = 0,
      albumCount = 0;
    function readValue(
      start: number,
      end: number,
      list: boolean,
      plain: boolean,
      listStart: number,
    ): { value: string; quoted: boolean; span: Span; next: number } {
      if (start >= end) fail("MISSING_VALUE", start, start);
      let cursor = start,
        value = "";
      const quoted = raw[cursor] === '"';
      const valueStart = quoted ? ++cursor : cursor;
      while (cursor < end) {
        const c = raw[cursor]!;
        if (/^[\x00-\x1f\x7f-\x9f]$/.test(c)) fail("CONTROL_CHARACTER", cursor, cursor + 1);
        if (quoted && c === '"') break;
        if (!quoted && list && c === ",") break;
        if (!quoted && c === '"') fail("UNEXPECTED_CHARACTER", cursor, cursor + 1);
        if (!quoted && c === "\\") fail("INVALID_ESCAPE", cursor, cursor + 1);
        if (!quoted && !plain && !list && c === ",")
          fail("UNEXPECTED_CHARACTER", cursor, cursor + 1);
        if (quoted && c === "\\") {
          if (cursor + 1 >= end) fail("UNCLOSED_QUOTE", start, end);
          const next = raw[cursor + 1];
          if (next !== '"' && next !== "\\")
            fail(
              "INVALID_ESCAPE",
              cursor,
              Math.min(end, cursor + 1 + ((raw.codePointAt(cursor + 1) ?? 0) > 0xffff ? 2 : 1)),
            );
          value += next;
          cursor += 2;
        } else {
          value += c;
          cursor++;
        }
      }
      const valueEnd = cursor;
      if (quoted) {
        if (cursor >= end) fail("UNCLOSED_QUOTE", start, end);
        cursor++;
        if (cursor < end && (!list || raw[cursor] !== ","))
          fail(
            "UNEXPECTED_CHARACTER",
            cursor,
            cursor + ((raw.codePointAt(cursor) ?? 0) > 0xffff ? 2 : 1),
          );
      }
      if (value === "")
        fail(
          list && (start > listStart || cursor < end) ? "EMPTY_LIST_ITEM" : "MISSING_VALUE",
          valueStart,
          valueStart,
        );
      return {
        value,
        quoted,
        span: { start: valueStart, end: valueEnd },
        next: cursor,
      };
    }
    for (const span of spans) {
      const tokenRaw = raw.slice(span.start, span.end);
      let originalField = "",
        field: Field | "term" = "term",
        start = span.start;
      if (raw[start] !== '"') {
        const colon = tokenRaw.indexOf(":");
        if (colon !== -1) {
          originalField = foldText(tokenRaw.slice(0, colon));
          const candidate = ALIASES[originalField] ?? originalField;
          if (!/^[a-z][a-z0-9_-]*$/.test(originalField))
            fail("UNEXPECTED_CHARACTER", start, start + colon);
          if (!FIELDS.includes(candidate as Field)) {
            const suggestions: Record<string, string> = {
              ext: "format",
              type: "format",
              size: "minsize",
              name: '"filename"',
            };
            fail("UNKNOWN_FIELD", start, start + colon, {
              field: originalField,
              ...(suggestions[originalField] ? { suggestion: suggestions[originalField] } : {}),
            });
          }
          field = candidate as Field;
          start += colon + 1;
        } else {
          const fullwidth = /^[A-Za-z][A-Za-z0-9_-]*：/.exec(tokenRaw);
          if (fullwidth)
            fail(
              "UNEXPECTED_CHARACTER",
              span.start + fullwidth[0].length - 1,
              span.start + fullwidth[0].length,
              { suggestion: ":" },
            );
        }
      }
      if (field !== "term" && field !== "format" && field !== "album") {
        if (seen.has(field)) fail("DUPLICATE_FIELD", span.start, span.end, { field });
        seen.set(field, span);
      }
      const list = field === "format" || field === "album";
      const token: Token = { field, span, valueSpan: { start, end: span.end }, raw: tokenRaw };
      tokens.push(token);
      let position = start;
      const values: ReturnType<typeof readValue>[] = [];
      do {
        if (list && (raw[position] === "," || (position === span.end && position > start)))
          fail("EMPTY_LIST_ITEM", position, position);
        const item = readValue(position, span.end, list, field === "term", start);
        values.push(item);
        if (item.next === span.end) break;
        position = item.next + 1;
        if (position === span.end) fail("EMPTY_LIST_ITEM", position, position);
      } while (position <= span.end);
      for (const item of values) {
        if ([...item.value].length > 200) fail("QUERY_TOO_COMPLEX", item.span.start, item.span.end);
        const value = foldText(item.value);
        if (field === "term") {
          if (++termCount > 16) fail("QUERY_TOO_COMPLEX", span.start, span.end);
          if (!ast.terms.includes(value)) ast.terms.push(value);
          if (ast.terms.length > 8) fail("QUERY_TOO_COMPLEX", span.start, span.end);
        } else if (field === "format") {
          const format = value === "jpeg" ? "jpg" : value === "tif" ? "tiff" : value;
          if (!FORMATS.includes(format as (typeof FORMATS)[number]))
            fail("INVALID_ENUM", item.span.start, item.span.end, { field });
          if (!ast.filters.formats.includes(format as (typeof FORMATS)[number]))
            ast.filters.formats.push(format as (typeof FORMATS)[number]);
        } else if (field === "album") {
          if (++albumCount > 10) fail("QUERY_TOO_COMPLEX", item.span.start, item.span.end);
          let album: AlbumTerm;
          if (!item.quoted && value === "unfiled") album = { kind: "unfiled" };
          else if (!item.quoted && /^#[0-9]+$/.test(item.value)) {
            if (!/^#[1-9][0-9]*$/.test(item.value))
              fail("INVALID_ENUM", item.span.start, item.span.end, { field });
            album = { kind: "id", value: item.value.slice(1) };
          } else album = { kind: "name", value: item.value.normalize("NFC") };
          if (!ast.filters.albums.some((x) => JSON.stringify(x) === JSON.stringify(album)))
            ast.filters.albums.push(album);
          // Names may resolve to the same IDs; final max-five check belongs to the authorized resolver.
          const known = ast.filters.albums.filter((x) => x.kind !== "name");
          if (known.length > 5) fail("QUERY_TOO_COMPLEX", item.span.start, item.span.end);
        } else if (field === "camera") ast.filters.camera = value;
        else if (field === "minsize" || field === "maxsize") {
          const bytes = sizeBytes(item.value);
          if (bytes === null) fail("INVALID_SIZE", item.span.start, item.span.end, { field });
          ast.filters[field === "minsize" ? "minBytes" : "maxBytes"] = bytes;
        } else if (field === "after" || field === "before") {
          if (!validDate(item.value))
            fail("INVALID_DATE", item.span.start, item.span.end, { field });
          ast.filters[field] = item.value;
        } else if (field === "visibility") {
          if (
            !(
              originalField === "is" ? ["public", "private"] : ["all", "public", "private"]
            ).includes(value)
          )
            fail("INVALID_ENUM", item.span.start, item.span.end, { field: originalField });
          ast.filters.visibility = value as QueryAstV1["filters"]["visibility"];
        } else if (field === "sort") {
          const sort = originalField === "order" ? ORDERS[value] : value;
          if (!sort || !SORTS.includes(sort as QueryAstV1["sort"]))
            fail("INVALID_ENUM", item.span.start, item.span.end, { field: originalField });
          ast.sort = sort as QueryAstV1["sort"];
        }
      }
    }
    const conflict = (a: string, b: string) => {
      const x = seen.get(a)!,
        y = seen.get(b)!;
      fail("RANGE_CONFLICT", Math.min(x.start, y.start), Math.max(x.end, y.end));
    };
    if (
      ast.filters.minBytes !== null &&
      ast.filters.maxBytes !== null &&
      BigInt(ast.filters.minBytes) > BigInt(ast.filters.maxBytes)
    )
      conflict("minsize", "maxsize");
    if (ast.filters.after && ast.filters.before && ast.filters.after >= ast.filters.before)
      conflict("after", "before");
    ast.filters.formats.sort((a, b) => FORMATS.indexOf(a) - FORMATS.indexOf(b));
    ast.filters.albums.sort((a, b) => {
      if (a.kind === b.kind)
        return a.kind === "id" && b.kind === "id"
          ? BigInt(a.value) < BigInt(b.value)
            ? -1
            : BigInt(a.value) > BigInt(b.value)
              ? 1
              : 0
          : 0;
      return ["unfiled", "id", "name"].indexOf(a.kind) - ["unfiled", "id", "name"].indexOf(b.kind);
    });
    if (new TextEncoder().encode(serializeQueryV1(ast)).length > 4096)
      fail("QUERY_TOO_COMPLEX", 0, raw.length);
    return { ok: true, ast, tokens };
  } catch (error) {
    if (error && typeof error === "object" && "code" in error && "span" in error)
      return { ok: false, diagnostics: [error as Diagnostic] };
    throw error;
  }
}
export function serializeQueryV1(ast: QueryAstV1): string {
  const parts = ast.terms.map(quoteText),
    f = ast.filters;
  if (f.formats.length)
    parts.push(`format:${FORMATS.filter((x) => f.formats.includes(x)).join(",")}`);
  if (f.albums.length) {
    const ids = f.albums
      .filter((x): x is { kind: "id"; value: string } => x.kind === "id")
      .sort((a, b) =>
        BigInt(a.value) < BigInt(b.value) ? -1 : BigInt(a.value) > BigInt(b.value) ? 1 : 0,
      );
    const names = f.albums.filter((x): x is { kind: "name"; value: string } => x.kind === "name");
    parts.push(
      `album:${[...(f.albums.some((x) => x.kind === "unfiled") ? ["unfiled"] : []), ...ids.map((x) => `#${x.value}`), ...names.map((x) => quoteText(x.value))].join(",")}`,
    );
  }
  if (f.camera !== null) parts.push(`camera:${quoteText(f.camera)}`);
  if (f.minBytes !== null) parts.push(`minsize:${bytesMB(f.minBytes)}`);
  if (f.maxBytes !== null) parts.push(`maxsize:${bytesMB(f.maxBytes)}`);
  if (f.after) parts.push(`after:${f.after}`);
  if (f.before) parts.push(`before:${f.before}`);
  if (f.visibility !== "all") parts.push(`visibility:${f.visibility}`);
  if (ast.sort !== "newest") parts.push(`sort:${ast.sort}`);
  return parts.join(" ");
}
/** Edit source spans, preserving all unrelated user text and the draft as the only editable source. */
export function removeField(raw: string, field: Field): { raw: string; caret: number } {
  const parsed = parseQueryV1(raw);
  if (!parsed.ok) return { raw, caret: raw.length };
  const spans = parsed.tokens.filter((x) => x.field === field).map((x) => x.span);
  let next = raw;
  for (const span of spans.toReversed()) next = next.slice(0, span.start) + next.slice(span.end);
  return { raw: next.trim(), caret: Math.min(spans[0]?.start ?? next.length, next.trim().length) };
}
export function insertCondition(
  raw: string,
  addition: string,
  selection?: Span,
): { raw: string; caret: number } {
  if (selection && selection.end > selection.start) {
    const next = raw.slice(0, selection.start) + addition + raw.slice(selection.end);
    return { raw: next, caret: selection.start + addition.length };
  }
  const parsed = parseQueryV1(raw),
    added = parseQueryV1(addition);
  let next = raw;
  if (parsed.ok && added.ok) {
    const scalars = new Set<Token["field"]>(
      added.tokens
        .map((x) => x.field)
        .filter((x) => x !== "term" && x !== "format" && x !== "album"),
    );
    for (const span of parsed.tokens
      .filter((x) => scalars.has(x.field))
      .map((x) => x.span)
      .toReversed())
      next = next.slice(0, span.start) + next.slice(span.end);
  }
  next = `${next.trim()}${next.trim() ? " " : ""}${addition}`;
  return { raw: next, caret: next.length };
}
