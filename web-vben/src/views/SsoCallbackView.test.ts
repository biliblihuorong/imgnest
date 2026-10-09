import { flushPromises, mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";
import { TOKEN_STORAGE_KEY } from "@/api/client";
import * as authApi from "@/api/auth";
import type { UserView } from "@/api/types";
import SsoCallbackView from "./SsoCallbackView.vue";

vi.mock("@/api/auth", () => ({
  login: vi.fn(),
  logout: vi.fn(),
  me: vi.fn(),
  exchangeSsoTicket: vi.fn(),
}));
const authApiMock = vi.mocked(authApi);

const user: UserView = {
  id: 3,
  group_id: 1,
  username: "octocat",
  email: "octo@example.com",
  role: "user",
  status: "enabled",
  used_bytes: 0,
  created_at: "2026-10-09T00:00:00Z",
  display_name: "",
  avatar_provider: "weavatar",
  avatar_url: null,
  avatar_config_version: 0,
};

async function mountCallback(hash: string) {
  window.history.replaceState(null, "", "/auth/sso" + hash);
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/auth/sso", component: SsoCallbackView },
      { path: "/upload", component: { template: "<div>upload</div>" } },
    ],
  });
  await router.push("/auth/sso");
  await router.isReady();
  const wrapper = mount(SsoCallbackView, { global: { plugins: [createPinia(), router] } });
  await flushPromises();
  return { wrapper, router };
}

describe("SsoCallbackView", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetAllMocks();
  });

  it("用片段里的票据换取会话并跳到落地页", async () => {
    authApiMock.exchangeSsoTicket.mockResolvedValue({
      token: "9|sso-token",
      user,
      expires_at: "2026-10-10T00:00:00Z",
    });

    const { router } = await mountCallback("#ticket=abc123");

    expect(authApiMock.exchangeSsoTicket).toHaveBeenCalledWith("abc123", expect.any(AbortSignal));
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBe("9|sso-token");
    expect(router.currentRoute.value.path).toBe("/upload");
    // 票据从地址栏抹掉，不留在历史记录里。
    expect(window.location.hash).toBe("");
  });

  it("没有票据时直接提示失败", async () => {
    const { wrapper } = await mountCallback("");

    expect(authApiMock.exchangeSsoTicket).not.toHaveBeenCalled();
    expect(wrapper.get("[role='alert']").text()).toContain("单点登录失败");
  });

  it("票据被拒绝时提示失败并提供返回登录", async () => {
    authApiMock.exchangeSsoTicket.mockRejectedValue(new Error("401"));

    const { wrapper } = await mountCallback("#ticket=used");

    expect(wrapper.get("[role='alert']").text()).toContain("单点登录失败");
    expect(wrapper.find("a[href='/login']").exists()).toBe(true);
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBeNull();
  });
});
