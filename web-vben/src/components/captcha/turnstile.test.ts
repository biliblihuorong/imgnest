import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const scriptURL = "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
beforeEach(() => {
  vi.resetModules();
  document.head.querySelectorAll(`script[src="${scriptURL}"]`).forEach((node) => node.remove());
  delete window.turnstile;
});
afterEach(() => {
  vi.useRealTimers();
  delete window.turnstile;
});

describe("fixed Turnstile script loader", () => {
  it("shares one script and loading promise across widgets", async () => {
    const { loadTurnstile } = await import("./turnstile");
    const first = loadTurnstile();
    const second = loadTurnstile();
    expect(first).toBe(second);
    expect(document.querySelectorAll(`script[src="${scriptURL}"]`)).toHaveLength(1);
    const api = { render: vi.fn(), reset: vi.fn(), remove: vi.fn() };
    window.turnstile = api;
    document.querySelector(`script[src="${scriptURL}"]`)!.dispatchEvent(new Event("load"));
    expect(await first).toBe(api);
  });
  it("rejects CSP/load failures and permits an explicit retry with one script", async () => {
    const { loadTurnstile } = await import("./turnstile");
    const first = loadTurnstile();
    const rejected = expect(first).rejects.toThrow("captcha-script-unavailable");
    document.querySelector(`script[src="${scriptURL}"]`)!.dispatchEvent(new Event("error"));
    await rejected;
    const second = loadTurnstile();
    const retried = expect(second).rejects.toThrow();
    expect(document.querySelectorAll(`script[src="${scriptURL}"]`)).toHaveLength(1);
    document.querySelector(`script[src="${scriptURL}"]`)!.dispatchEvent(new Event("error"));
    await retried;
  });
  it("times out a blocked script without falling back to password-only", async () => {
    vi.useFakeTimers();
    const { loadTurnstile } = await import("./turnstile");
    const result = expect(loadTurnstile()).rejects.toThrow("captcha-script-unavailable");
    await vi.advanceTimersByTimeAsync(15000);
    await result;
    expect(document.querySelectorAll(`script[src="${scriptURL}"]`)).toHaveLength(0);
  });
});
