import { describe, expect, it } from "vitest";
import { formatBytes } from "./format";

describe("formatBytes 字节数格式化", () => {
  it("0 字节输出 0 B", () => {
    expect(formatBytes(0)).toBe("0 B");
  });

  it("1024 以内的字节直接展示", () => {
    expect(formatBytes(1)).toBe("1 B");
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(1023)).toBe("1023 B");
  });

  it("按 1024 进制自适应 KB/MB/GB", () => {
    expect(formatBytes(1024)).toBe("1.0 KB");
    expect(formatBytes(1536)).toBe("1.5 KB");
    expect(formatBytes(1048576)).toBe("1.0 MB");
    expect(formatBytes(3221225472)).toBe("3.0 GB");
  });

  it("百位以上数值取整，超大数值封顶单位", () => {
    expect(formatBytes(150 * 1024)).toBe("150 KB");
    expect(formatBytes(1024 ** 5)).toBe("1.0 PB");
  });

  it("负数与 NaN/Infinity 输出占位符 -", () => {
    expect(formatBytes(-1)).toBe("-");
    expect(formatBytes(Number.NaN)).toBe("-");
    expect(formatBytes(Number.POSITIVE_INFINITY)).toBe("-");
  });
});
