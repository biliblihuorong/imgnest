import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { NMessageProvider } from "naive-ui";
import { defineComponent, h } from "vue";
import { createMemoryHistory, createRouter, type Router } from "vue-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as authApi from "@/api/auth";
import { ApiError, TOKEN_STORAGE_KEY } from "@/api/client";
import type { UserView } from "@/api/types";
import { useAuthStore } from "@/stores/auth";
import AccountSettingsView from "./AccountSettingsView.vue";

vi.mock("@/api/auth", () => ({
  login: vi.fn(),
  register: vi.fn(),
  logout: vi.fn(),
  me: vi.fn(),
  changePassword: vi.fn(),
  updateDisplayName: vi.fn(),
}));

const authApiMock = vi.mocked(authApi);

// NMessageProvider 的消息容器 teleport 到 document.body，统一做 body 级断言；
// 用例结束后卸载应用，避免 teleport 内容跨用例残留。
enableAutoUnmount(afterEach);

const userFixture: UserView = {
  id: 7,
  display_name: "",
  group_id: 1,
  username: "alice",
  email: "alice@example.com",
  role: "user",
  status: "enabled",
  used_bytes: 0,
  created_at: "2026-10-04T00:00:00Z",
  avatar_provider: "weavatar",
  avatar_url: "https://weavatar.com/avatar/abc?s=160&d=404",
  avatar_config_version: 0,
};

let pinia: ReturnType<typeof import("pinia").createPinia>;
let router: Router;

async function mountView(): Promise<VueWrapper> {
  const Harness = defineComponent({
    render() {
      return h(NMessageProvider, () => h(AccountSettingsView));
    },
  });
  const wrapper = mount(Harness, { global: { plugins: [pinia, router] } });
  await flushPromises();
  return wrapper;
}

beforeEach(async () => {
  localStorage.clear();
  localStorage.setItem(TOKEN_STORAGE_KEY, "7|token");
  vi.resetAllMocks();
  pinia = createPinia();
  setActivePinia(pinia);
  router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/account/settings", component: { render: () => null } },
      { path: "/login", component: { render: () => null } },
    ],
  });
  await router.push("/account/settings");
  await router.isReady();
  const auth = useAuthStore(pinia);
  auth.token = "7|token";
  auth.user = { ...userFixture };
});

describe("AccountSettingsView", () => {
  it("展示只读账户信息与头像说明，未修改时保存禁用", async () => {
    const wrapper = await mountView();
    expect(wrapper.text()).toContain("个人设置");
    expect(wrapper.text()).toContain("alice");
    expect(wrapper.text()).toContain("alice@example.com");
    expect(wrapper.text()).toContain("头像由站点统一配置的头像服务");
    const save = wrapper.findAll("button").find((button) => button.text() === "保存资料");
    expect(save?.attributes("disabled")).toBeDefined();
    // 修改密码卡在同一页签，使用独立表单。
    expect(wrapper.find("input[placeholder='当前密码']").exists()).toBe(true);
  });

  it("保存显示名称成功后真实更新认证 store", async () => {
    const updated = { ...userFixture, display_name: "昵称" };
    authApiMock.updateDisplayName.mockResolvedValue(updated);
    const wrapper = await mountView();

    await wrapper.find("input[placeholder='留空则显示用户名']").setValue("昵称");
    const save = wrapper.findAll("button").find((button) => button.text() === "保存资料");
    expect(save?.attributes("disabled")).toBeUndefined();
    await save?.trigger("click");
    await flushPromises();

    expect(authApiMock.updateDisplayName).toHaveBeenCalledWith("昵称");
    const auth = useAuthStore(pinia);
    expect(auth.user?.display_name).toBe("昵称");
    expect(document.body.textContent).toContain("资料已更新");
    // 保存后与 store 同步，按钮回到禁用态。
    expect(save?.attributes("disabled")).toBeDefined();
  });

  it("保存失败就地展示错误且不写回 store", async () => {
    authApiMock.updateDisplayName.mockRejectedValue(new ApiError(10001, "invalid request", 400));
    const wrapper = await mountView();

    await wrapper.find("input[placeholder='留空则显示用户名']").setValue("昵称");
    const save = wrapper.findAll("button").find((button) => button.text() === "保存资料");
    await save?.trigger("click");
    await flushPromises();

    const auth = useAuthStore(pinia);
    expect(auth.user?.display_name).toBe("");
    expect(wrapper.find(".account-settings__error").exists()).toBe(true);
  });
});
