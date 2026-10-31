import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { NMessageProvider, NSwitch } from "naive-ui";
import { h } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/api/client";
import { listAuditLogs, type AuditPage } from "@/api/audit";
import AuditLogsView from "./AuditLogsView.vue";
import { auditActionKey } from "./auditActions";

vi.mock("@/api/audit", () => ({ listAuditLogs: vi.fn() }));

const listMock = vi.mocked(listAuditLogs);

enableAutoUnmount(afterEach);

class ResizeObserverStub {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}
vi.stubGlobal("ResizeObserver", ResizeObserverStub);

function page(): AuditPage {
  return {
    items: [
      {
        id: 2,
        at: "2026-10-31T01:00:00Z",
        user_id: 7,
        user_name: "alice",
        ip: "203.0.113.9",
        method: "DELETE",
        route: "/api/images/:id",
        target: "id=12",
        status: 200,
        request_id: "a1b2c3d4e5f6a1b2c3d4e5f6",
      },
      {
        id: 1,
        at: "2026-10-31T00:59:00Z",
        user_id: 0,
        user_name: "",
        ip: "198.51.100.4",
        method: "POST",
        route: "/api/ext/demo/hook",
        target: "",
        status: 401,
        request_id: "req-0001",
      },
    ],
    total: 2,
    page: 1,
    size: 50,
  };
}

function mountView(): VueWrapper {
  return mount(() => h(NMessageProvider, () => h(AuditLogsView)));
}

describe("AuditLogsView", () => {
  beforeEach(() => {
    listMock.mockReset();
  });

  it("列出动作、用户、结果和请求 ID", async () => {
    listMock.mockResolvedValue(page());
    const wrapper = mountView();
    await flushPromises();

    expect(listMock).toHaveBeenCalledWith({ page: 1, size: 50, userId: undefined, failed: false });
    const text = wrapper.text();
    expect(text).toContain("删除图片（进回收站）");
    expect(text).toContain("alice");
    expect(text).toContain("id=12");
    expect(text).toContain("成功 200");
    expect(text).toContain("未登录");
    expect(text).toContain("POST /api/ext/demo/hook");
    expect(text).toContain("失败 401");
    expect(text).toContain("a1b2c3d4e5f6a1b2c3d4e5f6");
  });

  it("点用户名按用户筛选，开关只看失败", async () => {
    listMock.mockResolvedValue(page());
    const wrapper = mountView();
    await flushPromises();

    const alice = wrapper.findAll("button").find((button) => button.text() === "alice");
    await alice!.trigger("click");
    await flushPromises();
    expect(listMock).toHaveBeenLastCalledWith({ page: 1, size: 50, userId: 7, failed: false });

    wrapper.findComponent(NSwitch).vm.$emit("update:value", true);
    await flushPromises();
    expect(listMock).toHaveBeenLastCalledWith({ page: 1, size: 50, userId: 7, failed: true });
  });

  it("加载失败时提示错误并可重试", async () => {
    listMock.mockRejectedValueOnce(new ApiError(50001, "internal error", 500));
    listMock.mockResolvedValueOnce(page());
    const wrapper = mountView();
    await flushPromises();
    expect(wrapper.find(".n-alert").exists()).toBe(true);

    const retry = wrapper.findAll("button").find((button) => button.text() === "重试");
    await retry!.trigger("click");
    await flushPromises();
    expect(wrapper.find(".n-alert").exists()).toBe(false);
    expect(wrapper.text()).toContain("alice");
  });
});

describe("auditActionKey", () => {
  it("v2 复用原生动作，未知路由返回 null", () => {
    expect(auditActionKey("POST", "/api/v2/upload")).toBe("upload");
    expect(auditActionKey("POST", "/api/v1/upload")).toBe("v1Upload");
    expect(auditActionKey("PUT", "/api/admin/settings")).toBe("adminSettings");
    expect(auditActionKey("POST", "/api/ext/demo/hook")).toBeNull();
  });
});
