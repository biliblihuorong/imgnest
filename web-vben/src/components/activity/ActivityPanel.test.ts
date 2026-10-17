import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ActivityView } from "@/api/activity";
import ActivityPanel from "./ActivityPanel.vue";

enableAutoUnmount(afterEach);

function activity(): ActivityView {
  const days = Array.from({ length: 30 }, (_, index) => ({
    day: `2026-10-${String(index + 1).padStart(2, "0")}`,
    requests: index === 29 ? 40 : index === 28 ? 10 : 0,
  }));
  return {
    last_seen_at: "2026-10-17T11:00:00Z",
    last_login: {
      at: "2026-10-17T10:00:00Z",
      ip: "198.51.100.7",
      user_agent: "Mozilla/5.0 (Macintosh) Firefox/140.0",
    },
    logins: [
      {
        at: "2026-10-17T10:00:00Z",
        ip: "198.51.100.7",
        user_agent: "Mozilla/5.0 (Macintosh) Firefox/140.0",
      },
    ],
    active_sessions: 2,
    tokens: [
      {
        id: 3,
        name: "picgo",
        kind: "api",
        abilities: ["*"],
        expires_at: null,
        last_used_at: "2026-10-17T11:30:00Z",
        last_used_ip: "203.0.113.9",
        use_count: 1234,
        created_at: "2026-10-01T00:00:00Z",
      },
    ],
    api_usage: days,
    api_requests: 50,
  };
}

describe("ActivityPanel", () => {
  it("shows the summary, the 30-day bars and the API key usage", async () => {
    const wrapper = mount(ActivityPanel, { props: { load: () => Promise.resolve(activity()) } });
    await flushPromises();
    expect(wrapper.get('[data-stat="sessions"] .activity-panel__value').text()).toBe("2");
    expect(wrapper.get('[data-stat="apiRequests"] .activity-panel__value').text()).toBe("50");
    expect(wrapper.get('[data-stat="lastLogin"]').text()).toContain("198.51.100.7");
    const bars = wrapper.findAll(".activity-panel__bar");
    expect(bars).toHaveLength(30);
    expect(bars[29]!.attributes("style")).toContain("height: 100%");
    expect(bars[28]!.attributes("style")).toContain("height: 25%");
    expect(bars[0]!.attributes("style")).toContain("height: 0%");
    expect(wrapper.text()).toContain("picgo");
    expect(wrapper.text()).toContain("1,234");
    expect(wrapper.text()).toContain("203.0.113.9");
    expect(wrapper.text()).toContain("Firefox · macOS");
  });

  it("reports a failed load and retries", async () => {
    const load = vi.fn().mockRejectedValueOnce(new Error("boom")).mockResolvedValue(activity());
    const wrapper = mount(ActivityPanel, { props: { load } });
    await flushPromises();
    expect(wrapper.find(".activity-panel__error").exists()).toBe(true);
    await wrapper.get(".activity-panel__error button").trigger("click");
    await flushPromises();
    expect(load).toHaveBeenCalledTimes(2);
    expect(wrapper.find(".activity-panel__error").exists()).toBe(false);
  });

  it("reloads when the account changes", async () => {
    const load = vi.fn().mockResolvedValue(activity());
    const wrapper = mount(ActivityPanel, { props: { load, reloadKey: 1 } });
    await flushPromises();
    await wrapper.setProps({ reloadKey: 2 });
    await flushPromises();
    expect(load).toHaveBeenCalledTimes(2);
  });
});
