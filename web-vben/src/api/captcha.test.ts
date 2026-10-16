import { afterEach, describe, expect, it, vi } from "vitest";
import { configureCaptcha, fetchCaptcha, getCaptchaSettings } from "./captcha";
import { login, register } from "./auth";

function mockResponse(data: unknown) {
  const fetcher = vi
    .fn()
    .mockImplementation(
      async () => new Response(JSON.stringify({ code: 0, data }), { status: 200 }),
    );
  vi.stubGlobal("fetch", fetcher);
  return fetcher;
}
afterEach(() => vi.unstubAllGlobals());

describe("CAPTCHA HTTP contract", () => {
  it("reads public configuration with a cancellable request", async () => {
    const fetcher = mockResponse({ enabled: false, provider: "", site_key: "", version: 1 });
    const controller = new AbortController();
    expect(await fetchCaptcha(controller.signal)).toEqual({
      enabled: false,
      provider: "",
      site_key: "",
      version: 1,
    });
    expect(fetcher.mock.calls[0]?.[0]).toBe("/api/auth/captcha");
    expect(fetcher.mock.calls[0]?.[1].signal).toBe(controller.signal);
  });
  it("reads admin state and configures in one request without inventing a secret", async () => {
    const fetcher = mockResponse({
      version: 3,
      enabled: false,
      active: null,
      draft: null,
      configuration_available: true,
    });
    await getCaptchaSettings();
    expect(fetcher.mock.calls[0]?.[0]).toBe("/api/admin/captcha");
    await configureCaptcha({
      expected_version: 3,
      enabled: true,
      site_key: "public-key",
      hostnames: ["img.test"],
      captcha_token: "one-use",
    });
    expect(fetcher.mock.calls[1]?.[0]).toBe("/api/admin/captcha");
    expect(fetcher.mock.calls[1]?.[1].method).toBe("PUT");
    expect(JSON.parse(fetcher.mock.calls[1]?.[1].body)).toEqual({
      expected_version: 3,
      enabled: true,
      site_key: "public-key",
      hostnames: ["img.test"],
      captcha_token: "one-use",
    });
  });
  it("preserves the exact legacy login body and adds the optional token only when supplied", async () => {
    const fetcher = mockResponse({});
    await login("a@example.test", "password");
    expect(JSON.parse(fetcher.mock.calls[0]?.[1].body)).toEqual({
      email: "a@example.test",
      password: "password",
    });
    const controller = new AbortController();
    await login("a@example.test", "password", "one-use", controller.signal);
    expect(fetcher.mock.calls[1]?.[1].signal).toBe(controller.signal);
    expect(JSON.parse(fetcher.mock.calls[1]?.[1].body)).toEqual({
      email: "a@example.test",
      password: "password",
      captcha_token: "one-use",
    });
  });
  it("allows registration to carry a one-use CAPTCHA token", async () => {
    const fetcher = mockResponse({});
    const controller = new AbortController();
    await register(
      {
        username: "alice",
        email: "a@example.test",
        password: "password-long",
        captcha_token: "one-use",
      },
      controller.signal,
    );
    expect(fetcher.mock.calls[0]?.[1].signal).toBe(controller.signal);
    expect(JSON.parse(fetcher.mock.calls[0]?.[1].body)).toEqual({
      username: "alice",
      email: "a@example.test",
      password: "password-long",
      captcha_token: "one-use",
    });
  });
  it("rejects malformed public configuration instead of treating it as disabled", async () => {
    mockResponse({});
    await expect(fetchCaptcha()).rejects.toThrow();
  });
});
