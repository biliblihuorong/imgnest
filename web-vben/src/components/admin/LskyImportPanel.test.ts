import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "@/api/admin";
import type { LskyImportStatus } from "@/api/admin";
import LskyImportView from "./LskyImportPanel.vue";

vi.mock("@/api/admin", () => ({
  getLskyImport: vi.fn(),
  uploadLskyDump: vi.fn(),
  discardLskyDump: vi.fn(),
  startLskyImport: vi.fn(),
  cancelLskyImport: vi.fn(),
  listPolicies: vi.fn(),
  listStorages: vi.fn(),
}));

class ResizeObserverStub {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}
vi.stubGlobal("ResizeObserver", ResizeObserverStub);

enableAutoUnmount(afterEach);
const apiMock = vi.mocked(api);

function status(overrides: Partial<LskyImportStatus> = {}): LskyImportStatus {
  return {
    loaded: false,
    summary: null,
    running: false,
    phase: "idle",
    options: null,
    accounts: null,
    total: 0,
    processed: 0,
    imported: 0,
    existing: 0,
    skipped: 0,
    failed: 0,
    failures: null,
    skips: null,
    error: "",
    started_at: null,
    finished_at: null,
    ...overrides,
  };
}

const loaded = status({
  loaded: true,
  summary: {
    groups: 3,
    users: 7,
    tokens: 1,
    albums: 0,
    images: 296,
    loaded_at: "2026-10-16T00:00:00Z",
    strategies: [
      { id: 2, name: "local", url: "http://localhost:9080/uploads", images: 1 },
      { id: 9, name: "B2", url: "https://img.example.com/old-bucket", images: 295 },
    ],
  },
});

beforeEach(() => {
  vi.resetAllMocks();
  vi.useRealTimers();
  apiMock.listPolicies.mockResolvedValue([
    { id: 1, name: "Lsky Migration", storage_id: 1 } as api.PolicyView,
  ]);
  apiMock.listStorages.mockResolvedValue([
    {
      id: 1,
      name: "backblaze",
      driver: "s3",
      base_url: "https://img.example.com/old-bucket/",
      path_prefix: "",
      enabled: true,
    },
  ]);
});

describe("LskyImportView", () => {
  it("uploads the chosen file with the time zone", async () => {
    apiMock.getLskyImport.mockResolvedValue(status());
    apiMock.uploadLskyDump.mockResolvedValue(loaded);
    const wrapper = mount(LskyImportView);
    await flushPromises();
    const input = wrapper.get("[data-lsky-file]");
    const file = new File(["INSERT"], "LsyPro.sql");
    Object.defineProperty(input.element, "files", { value: [file] });
    await input.trigger("change");
    await flushPromises();
    expect(apiMock.uploadLskyDump).toHaveBeenCalledWith(file, "Asia/Shanghai");
    expect(wrapper.get("[data-lsky-summary]").text()).toContain("296");
  });

  it("adopts the strategy stored in the rule's bucket by default", async () => {
    apiMock.getLskyImport.mockResolvedValue(loaded);
    apiMock.startLskyImport.mockResolvedValue(
      status({ ...loaded, running: true, phase: "accounts", total: 295 }),
    );
    const wrapper = mount(LskyImportView);
    await flushPromises();
    // NPopconfirm renders its confirmation in a popover; call its handler directly.
    const confirm = wrapper.findComponent({ name: "Popconfirm" });
    confirm.vm.$emit("positive-click");
    await flushPromises();
    expect(apiMock.startLskyImport).toHaveBeenCalledWith({
      policy_id: 1,
      mode: "adopt",
      strategy_id: 9,
      source_url: "",
    });
    expect(wrapper.get("[data-lsky-phase]").text()).toContain("0 / 295");
  });

  it("polls while running and stops when done", async () => {
    vi.useFakeTimers();
    apiMock.getLskyImport
      .mockResolvedValueOnce(status({ ...loaded, running: true, phase: "images", total: 2 }))
      .mockResolvedValue(status({ ...loaded, phase: "done", total: 2, processed: 2, imported: 2 }));
    mount(LskyImportView);
    await flushPromises();
    expect(apiMock.getLskyImport).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(2000);
    expect(apiMock.getLskyImport).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(6000);
    expect(apiMock.getLskyImport).toHaveBeenCalledTimes(2);
  });

  it("does not restart polling when a status response arrives after unmount", async () => {
    vi.useFakeTimers();
    const running = status({ ...loaded, running: true, phase: "images", total: 2 });
    let resolveLate: (value: LskyImportStatus) => void = () => {};
    apiMock.getLskyImport
      .mockResolvedValueOnce(running)
      .mockImplementationOnce(() => new Promise((resolve) => (resolveLate = resolve)))
      .mockRejectedValue(new Error("unauthenticated"));
    const wrapper = mount(LskyImportView);
    await flushPromises();
    await vi.advanceTimersByTimeAsync(2000);
    expect(apiMock.getLskyImport).toHaveBeenCalledTimes(2);

    wrapper.unmount();
    resolveLate(running);
    await flushPromises();
    await vi.advanceTimersByTimeAsync(20000);
    expect(apiMock.getLskyImport).toHaveBeenCalledTimes(2);
    expect(vi.getTimerCount()).toBe(0);
  });
});
