import { describe, expect, it } from "vitest";
import { i18n } from "@vben/locales";
import { ApiError } from "@/api/client";
import { formatApiError } from "./errors";

describe("localized API errors", () => {
  it("disambiguates invalid input and missing resource using HTTP status", () => {
    expect(formatApiError(new ApiError(10001, "not found", 404), "common.errors.unknown")).toBe(
      "记录不存在或已被删除",
    );
    expect(formatApiError(new ApiError(10001, "invalid input", 400), "common.errors.unknown")).toBe(
      "输入有误，请检查后重试",
    );
  });
  it("does not leak server messages or secrets into a fallback", () => {
    i18n.global.locale.value = "en-US";
    expect(
      formatApiError(new ApiError(99999, "secret backend detail", 500), "common.errors.unknown"),
    ).toBe("The service is temporarily unavailable.");
  });
  it("uses action-specific current password text", () => {
    expect(
      formatApiError(new ApiError(20002, "invalid credentials", 401), "account.errors.password"),
    ).toBe("当前密码错误");
  });
  it("shows the server's reason hint instead of a generic invalid-input text", () => {
    i18n.global.locale.value = "zh-CN";
    expect(
      formatApiError(new ApiError(10001, "invalid request", 400, { reason: "password_length" })),
    ).toBe("密码长度需要在 12 到 72 个字符之间");
    expect(
      formatApiError(
        new ApiError(50002, "storage operation failed", 502, { reason: "storage_denied" }),
      ),
    ).toBe("存储服务暂时不可用：存储拒绝了访问，请检查 AccessKey / SecretKey 和权限");
  });
  it("ignores unknown or malformed reasons", () => {
    expect(formatApiError(new ApiError(10001, "x", 400, { reason: "brand_new" }))).toBe(
      "输入有误，请检查后重试",
    );
    expect(formatApiError(new ApiError(10001, "x", 400, { reason: "<b>x</b>" }))).toBe(
      "输入有误，请检查后重试",
    );
  });
  it("explains a gateway error page instead of blaming the network", () => {
    expect(formatApiError(new ApiError(-1, "网络错误", 502))).toContain("HTTP 502");
    expect(formatApiError(new ApiError(-1, "网络错误", 0))).toBe("网络连接失败，请重试");
  });
});
