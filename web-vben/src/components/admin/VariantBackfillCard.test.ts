import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "@/api/admin";
import type { VariantBackfill } from "@/api/admin";
import VariantBackfillCard from "./VariantBackfillCard.vue";

vi.mock("@/api/admin", () => ({
  getVariantBackfill: vi.fn(),
  controlVariantBackfill: vi.fn(),
}));

enableAutoUnmount(afterEach);
const apiMock = vi.mocked(api);

function progress(overrides: Partial<VariantBackfill> = {}): VariantBackfill {
  return {
    status: "idle",
    cursor: 0,
    processed: 0,
    generated: 0,
    skipped: 0,
    failed: 0,
    updated_at: "2026-10-16T00:00:00Z",
    active: false,
    ...overrides,
  };
}

beforeEach(() => {
  vi.resetAllMocks();
  vi.useRealTimers();
});

describe("VariantBackfillCard", () => {
  it("starts an idle backfill and switches to pause while running", async () => {
    apiMock.getVariantBackfill.mockResolvedValue(progress());
    apiMock.controlVariantBackfill.mockResolvedValue(progress({ status: "running", active: true }));
    const wrapper = mount(VariantBackfillCard);
    await flushPromises();

    await wrapper.get('button[data-action="start"]').trigger("click");
    await flushPromises();

    expect(apiMock.controlVariantBackfill).toHaveBeenCalledWith("start", false);
    expect(wrapper.find('button[data-action="pause"]').exists()).toBe(true);
    expect(wrapper.find('button[data-action="start"]').exists()).toBe(false);
  });

  it("pauses a running backfill and offers to resume", async () => {
    apiMock.getVariantBackfill.mockResolvedValue(
      progress({ status: "running", processed: 40, generated: 12, active: true }),
    );
    apiMock.controlVariantBackfill.mockResolvedValue(progress({ status: "paused", processed: 40 }));
    const wrapper = mount(VariantBackfillCard);
    await flushPromises();
    expect(wrapper.get("[data-variant-status]").text()).toContain("40");

    await wrapper.get('button[data-action="pause"]').trigger("click");
    await flushPromises();

    expect(apiMock.controlVariantBackfill).toHaveBeenCalledWith("pause", false);
    expect(wrapper.find('button[data-action="start"]').exists()).toBe(true);
  });

  it("polls only while running", async () => {
    vi.useFakeTimers();
    apiMock.getVariantBackfill
      .mockResolvedValueOnce(progress({ status: "running", active: true }))
      .mockResolvedValue(progress({ status: "done", processed: 3 }));
    mount(VariantBackfillCard);
    await flushPromises();
    expect(apiMock.getVariantBackfill).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(3000);
    expect(apiMock.getVariantBackfill).toHaveBeenCalledTimes(2);

    await vi.advanceTimersByTimeAsync(9000);
    expect(apiMock.getVariantBackfill).toHaveBeenCalledTimes(2);
  });

  it("shows an error when progress cannot be loaded", async () => {
    apiMock.getVariantBackfill.mockRejectedValue(new Error("offline"));
    const wrapper = mount(VariantBackfillCard);
    await flushPromises();
    expect(wrapper.find(".variant-backfill__notice").exists()).toBe(true);
  });
});
