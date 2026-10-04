import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { NInputNumber, NMessageProvider, NSelect, NSwitch } from "naive-ui";
import { h } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as adminApi from "@/api/admin";
import type { AdminSettings, GroupView } from "@/api/admin";
import SettingsView from "./SettingsView.vue";

vi.mock("@/api/admin", () => ({
  getSettings: vi.fn(),
  putSettings: vi.fn(),
  listGroups: vi.fn(),
}));

const adminApiMock = vi.mocked(adminApi);

// jsdom 未实现 ResizeObserver，naive-ui 表单/弹层依赖它
class ResizeObserverStub {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}
vi.stubGlobal("ResizeObserver", ResizeObserverStub);

enableAutoUnmount(afterEach);

const settingsFixture: AdminSettings = {
  site_name: "ImgNest",
  registration_enabled: true,
  guest_upload_enabled: false,
  gallery_enabled: true,
  api_enabled: true,
  trash_days: 7,
  guest_group_id: 3,
  default_group_id: 1,
};

function makeGroup(overrides: Partial<GroupView> = {}): GroupView {
  return {
    id: 1,
    name: "默认组",
    is_default: true,
    is_guest: false,
    capacity_bytes: 1024 ** 3,
    max_file_bytes: 10 * 1024 ** 2,
    allowed_exts: ["jpg", "png"],
    upload_per_min: 30,
    default_policy_id: 1,
    policy_ids: [1],
    user_count: 2,
    ...overrides,
  };
}

async function mountView(): Promise<VueWrapper> {
  const wrapper = mount(() => h(NMessageProvider, () => h(SettingsView)));
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  vi.resetAllMocks();
  adminApiMock.getSettings.mockResolvedValue({ ...settingsFixture });
  adminApiMock.listGroups.mockResolvedValue([
    makeGroup(),
    makeGroup({ id: 3, name: "游客组", is_default: false, is_guest: true }),
  ]);
});

describe("SettingsView", () => {
  it("加载后八字段填充表单，组选项含「未设置（0）」", async () => {
    const wrapper = await mountView();

    expect(adminApiMock.getSettings).toHaveBeenCalledTimes(1);
    const nameEl = wrapper.find("input[placeholder='站点名称']").element as HTMLInputElement;
    expect(nameEl.value).toBe("ImgNest");

    // 开关顺序：开放注册 / 游客上传 / 公共画廊 / 蓝空 v1 API
    const switchValues = wrapper.findAllComponents(NSwitch).map((item) => item.props("value"));
    expect(switchValues).toEqual([true, false, true, true]);

    expect(wrapper.findComponent(NInputNumber).props("value")).toBe(7);

    const selects = wrapper.findAllComponents(NSelect);
    expect(selects.map((item) => item.props("value"))).toEqual([3, 1]);
    for (const select of selects) {
      expect(select.props("options")).toEqual(
        expect.arrayContaining([expect.objectContaining({ label: "未设置（0）", value: 0 })]),
      );
    }
  });

  it("保存时调用 putSettings 传全量八字段，成功后回读刷新", async () => {
    // 初次加载返回旧值，保存成功后的回读返回已更新的值（模拟服务端已持久化）
    adminApiMock.getSettings
      .mockResolvedValueOnce({ ...settingsFixture })
      .mockResolvedValueOnce({ ...settingsFixture, site_name: "我的图床" });
    adminApiMock.putSettings.mockResolvedValue({
      ...settingsFixture,
      site_name: "我的图床",
    });
    const wrapper = await mountView();

    await wrapper.find("input[placeholder='站点名称']").setValue("我的图床");
    const saveButton = wrapper.findAll("button").find((button) => button.text() === "保存设置");
    expect(saveButton).toBeTruthy();
    await saveButton?.trigger("click");
    await flushPromises();

    expect(adminApiMock.putSettings).toHaveBeenCalledTimes(1);
    expect(adminApiMock.putSettings).toHaveBeenCalledWith({
      site_name: "我的图床",
      registration_enabled: true,
      guest_upload_enabled: false,
      gallery_enabled: true,
      api_enabled: true,
      trash_days: 7,
      guest_group_id: 3,
      default_group_id: 1,
    });
    expect(adminApiMock.getSettings).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).toContain("站点设置已保存");
    // 回读后表单展示服务端返回值
    const nameEl = wrapper.find("input[placeholder='站点名称']").element as HTMLInputElement;
    expect(nameEl.value).toBe("我的图床");
  });

  it("trash_days 小于 0 时拦截保存并就地提示", async () => {
    const wrapper = await mountView();

    wrapper.findComponent(NInputNumber).vm.$emit("update:value", -1);
    await flushPromises();

    const saveButton = wrapper.findAll("button").find((button) => button.text() === "保存设置");
    await saveButton?.trigger("click");
    await flushPromises();

    expect(adminApiMock.putSettings).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("回收站保留天数不能小于 0");
  });

  it("加载失败显示告警，重试成功后填充", async () => {
    adminApiMock.getSettings.mockRejectedValueOnce(new Error("无权限"));
    const wrapper = await mountView();

    expect(wrapper.text()).toContain("站点设置加载失败：无权限");

    const retry = wrapper.findAll("button").find((button) => button.text() === "重试");
    expect(retry).toBeTruthy();
    await retry?.trigger("click");
    await flushPromises();

    const nameEl = wrapper.find("input[placeholder='站点名称']").element as HTMLInputElement;
    expect(nameEl.value).toBe("ImgNest");
  });

  it("保存失败时提示错误且不回读", async () => {
    adminApiMock.putSettings.mockRejectedValue(new Error("网络错误"));
    const wrapper = await mountView();

    const saveButton = wrapper.findAll("button").find((button) => button.text() === "保存设置");
    await saveButton?.trigger("click");
    await flushPromises();

    expect(document.body.textContent).toContain("保存失败：网络错误");
    expect(adminApiMock.getSettings).toHaveBeenCalledTimes(1);
  });
});
