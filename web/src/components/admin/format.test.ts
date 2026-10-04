import { describe, expect, it } from "vitest";
import {
  apiErrorMessage,
  formatBytesOrUnlimited,
  formatDateTime,
  formatUploadPerMin,
} from "./format";

describe("admin/format", () => {
  it("formatBytesOrUnlimited：0 显示「不限」，正值走 formatBytes", () => {
    expect(formatBytesOrUnlimited(0)).toBe("不限");
    expect(formatBytesOrUnlimited(1024 ** 3)).toBe("1.0 GB");
    expect(formatBytesOrUnlimited(100 * 1024 * 1024)).toBe("100 MB");
  });

  it("formatUploadPerMin：0 显示「不限」，正值带单位", () => {
    expect(formatUploadPerMin(0)).toBe("不限");
    expect(formatUploadPerMin(30)).toBe("30 次/分");
  });

  it("formatDateTime：RFC3339 → 本地 YYYY-MM-DD HH:mm，非法原样返回，空串为 -", () => {
    expect(formatDateTime("2026-10-04T12:34:56Z")).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/);
    expect(formatDateTime("not-a-date")).toBe("not-a-date");
    expect(formatDateTime("")).toBe("-");
  });

  it("apiErrorMessage：Error 取 message，其余兜底", () => {
    expect(apiErrorMessage(new Error("网络错误"))).toBe("网络错误");
    expect(apiErrorMessage("boom")).toBe("未知错误");
  });
});
