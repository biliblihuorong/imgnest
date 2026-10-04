import { describe, expect, it } from "vitest";
import { formatDateTime, formatRetention } from "./time";

describe("formatDateTime", () => {
  it("空值返回 -", () => {
    expect(formatDateTime(null)).toBe("-");
    expect(formatDateTime("")).toBe("-");
  });

  it("非法时间返回 -", () => {
    expect(formatDateTime("not-a-date")).toBe("-");
  });

  it("按本地时区格式化为 YYYY-MM-DD HH:mm", () => {
    const date = new Date("2026-10-04T08:30:00Z");
    const pad = (value: number): string => String(value).padStart(2, "0");
    const expected =
      `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ` +
      `${pad(date.getHours())}:${pad(date.getMinutes())}`;

    expect(formatDateTime("2026-10-04T08:30:00Z")).toBe(expected);
  });
});

describe("formatRetention", () => {
  const now = new Date("2026-10-04T12:00:00Z");

  it("空值或非法时间返回 -", () => {
    expect(formatRetention(null, now)).toBe("-");
    expect(formatRetention("bad", now)).toBe("-");
  });

  it("已到期返回 即将清理", () => {
    expect(formatRetention("2026-10-04T11:59:59Z", now)).toBe("即将清理");
  });

  it("不足 1 小时返回 剩余不足 1 小时", () => {
    expect(formatRetention("2026-10-04T12:30:00Z", now)).toBe("剩余不足 1 小时");
  });

  it("同一天内按小时展示", () => {
    expect(formatRetention("2026-10-04T20:00:00Z", now)).toBe("剩余 8 小时");
  });

  it("超过 1 天按天展示", () => {
    expect(formatRetention("2026-10-06T12:00:00Z", now)).toBe("剩余 2 天");
    expect(formatRetention("2026-10-07T11:59:00Z", now)).toBe("剩余 2 天");
  });
});
