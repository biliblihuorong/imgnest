import { beforeEach, describe, expect, it, vi } from "vitest";
import { request } from "./client";
import { fetchDashboardMetric } from "./dashboard";
vi.mock("./client", () => ({ request: vi.fn() }));
const requestMock = vi.mocked(request);
beforeEach(() => requestMock.mockReset());
describe("dashboard native metrics", () => {
  it.each([
    ["images", "/api/images?page=1&size=1"],
    ["trash", "/api/trash?page=1&size=1"],
    ["albums", "/api/albums?page=1&size=1"],
    ["globalImages", "/api/admin/images?page=1&size=1"],
    ["accounts", "/api/admin/users?page=1&size=1"],
  ] as const)("reads %s from total rather than a page length", async (metric, path) => {
    requestMock.mockResolvedValue({ items: [], total: 1357, page: 1, size: 1 });
    expect(await fetchDashboardMetric(metric, { id: 7, role: "admin" })).toBe(1357);
    expect(requestMock).toHaveBeenCalledExactlyOnceWith(path);
  });
  it("uses native account charged bytes", async () => {
    requestMock.mockResolvedValue({ id: 7, used_bytes: 2048 });
    expect(await fetchDashboardMetric("storage", { id: 7, role: "user" })).toBe(2048);
    expect(requestMock).toHaveBeenCalledExactlyOnceWith("/api/auth/me");
  });
  it("rejects a mismatched account response", async () => {
    requestMock.mockResolvedValue({ id: 8, used_bytes: 2048 });
    await expect(fetchDashboardMetric("storage", { id: 7, role: "user" })).rejects.toThrow();
  });
  it.each(["globalImages", "accounts"] as const)(
    "never fetches %s for ordinary users",
    async (metric) => {
      await expect(fetchDashboardMetric(metric, { id: 7, role: "user" })).rejects.toThrow();
      expect(requestMock).not.toHaveBeenCalled();
    },
  );
  it.each([undefined, null, -1, NaN, Infinity, "5"])(
    "rejects invalid totals instead of inventing zero (%s)",
    async (total) => {
      requestMock.mockResolvedValue({ items: [], total });
      await expect(fetchDashboardMetric("images", { id: 7, role: "user" })).rejects.toThrow();
    },
  );
});
