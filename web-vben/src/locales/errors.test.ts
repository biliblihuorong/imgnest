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
});
