import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { NMessageProvider } from "naive-ui";
import { h } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/api/client";
import { fetchAdminOverview, type AdminOverview } from "@/api/overview";
import OverviewView from "./OverviewView.vue";

vi.mock("@/api/overview", () => ({ fetchAdminOverview: vi.fn() }));

const fetchMock = vi.mocked(fetchAdminOverview);

enableAutoUnmount(afterEach);

class ResizeObserverStub {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}
vi.stubGlobal("ResizeObserver", ResizeObserverStub);

const MiB = 1024 * 1024;

function fixture(): AdminOverview {
  const uploads = Array.from({ length: 30 }, (_, index) => ({
    day: new Date(Date.UTC(2026, 8, 20 + index)).toISOString().slice(0, 10),
    uploads: index === 29 ? 4 : index === 10 ? 2 : 0,
    bytes: index === 29 ? 3 * MiB : index === 10 ? MiB : 0,
  }));
  return {
    generated_at: "2026-10-19T08:00:00Z",
    totals: {
      users: 3,
      albums: 2,
      images: 12,
      bytes: 10 * MiB,
      trash_images: 1,
      trash_bytes: 512 * 1024,
      pending_images: 0,
      guest_images: 1,
      guest_bytes: 1024,
    },
    storages: [
      {
        id: 1,
        name: "local-main",
        driver: "local",
        enabled: true,
        images: 10,
        bytes: 8 * MiB,
        original_bytes: 5 * MiB,
        webp_bytes: 2 * MiB,
        thumb_bytes: 0.5 * MiB,
        variant_bytes: 0.5 * MiB,
        trash_images: 1,
        trash_bytes: 512 * 1024,
      },
      {
        id: 2,
        name: "b2-archive",
        driver: "s3",
        enabled: false,
        images: 2,
        bytes: 2 * MiB,
        original_bytes: 2 * MiB,
        webp_bytes: 0,
        thumb_bytes: 0,
        variant_bytes: 0,
        trash_images: 0,
        trash_bytes: 0,
      },
    ],
    policies: [
      { id: 5, name: "default-rule", storage_id: 1, enabled: true, images: 10, bytes: 8 * MiB },
    ],
    uploads,
    top_users: [{ id: 7, username: "alice", used_bytes: 6 * MiB, images: 9 }],
    variants: { eligible: 4, covered: 1 },
  };
}

function mountView(): VueWrapper {
  return mount(() => h(NMessageProvider, () => h(OverviewView)));
}

describe("OverviewView", () => {
  beforeEach(() => {
    fetchMock.mockReset();
  });

  it("展示总量、上传趋势、存储与规则用量和用户排行", async () => {
    fetchMock.mockResolvedValueOnce(fixture());
    const wrapper = mountView();
    await flushPromises();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const stat = (key: string) => wrapper.find(`[data-stat="${key}"]`).text();
    expect(stat("users")).toContain("3");
    expect(stat("bytes")).toContain("10.0 MB");
    expect(stat("trash")).toContain("512 KB");
    expect(stat("variants")).toContain("25%");

    const bars = wrapper.findAll(".overview-view__bar-slot");
    expect(bars).toHaveLength(30);
    expect(bars[29]!.attributes("aria-label")).toContain("4");
    expect(wrapper.find('[data-testid="overview-uploads"]').text()).toContain("6");

    const storages = wrapper.find('[data-testid="overview-storages"]').text();
    expect(storages).toContain("local-main");
    expect(storages).toContain("8.0 MB");
    expect(storages).toContain("80%");
    expect(storages).toContain("已停用");
    const stack = wrapper.find(".overview-view__stack");
    expect(stack.attributes("aria-label")).toContain("原图 5.0 MB");
    expect(stack.findAll(".overview-view__stack-part")).toHaveLength(4);

    const policies = wrapper.find('[data-testid="overview-policies"]').text();
    expect(policies).toContain("default-rule");
    expect(policies).toContain("local-main");

    const users = wrapper.find('[data-testid="overview-users"]').text();
    expect(users).toContain("alice");
    expect(users).toContain("6.0 MB");
  });

  it("加载失败时提示错误并可重试", async () => {
    fetchMock.mockRejectedValueOnce(new ApiError(50001, "internal error", 500));
    fetchMock.mockResolvedValueOnce(fixture());
    const wrapper = mountView();
    await flushPromises();

    const alert = wrapper.find('[data-testid="overview-error"]');
    expect(alert.exists()).toBe(true);
    await alert.find("button").trigger("click");
    await flushPromises();

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(wrapper.find('[data-testid="overview-error"]').exists()).toBe(false);
    expect(wrapper.find('[data-stat="images"]').text()).toContain("12");
  });
});
