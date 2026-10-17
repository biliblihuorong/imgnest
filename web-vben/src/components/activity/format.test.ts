import { describe, expect, it } from "vitest";
import { describeUserAgent, relativeTime } from "./format";

describe("activity format", () => {
  const now = Date.parse("2026-10-17T12:00:00Z");
  it("formats relative times", () => {
    expect(relativeTime("2026-10-17T11:59:40Z", "en-US", now)).toBe("now");
    expect(relativeTime("2026-10-17T11:55:00Z", "en-US", now)).toBe("5 minutes ago");
    expect(relativeTime("2026-10-15T12:00:00Z", "en-US", now)).toBe("2 days ago");
    expect(relativeTime("not a date", "en-US", now)).toBe("not a date");
  });
  it("summarizes user agents", () => {
    expect(
      describeUserAgent(
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36 Edg/140.0",
      ),
    ).toBe("Edge · Windows");
    expect(
      describeUserAgent(
        "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1",
      ),
    ).toBe("Safari · iOS");
    expect(describeUserAgent("curl/8.5.0")).toBe("curl/8.5.0");
  });
});
