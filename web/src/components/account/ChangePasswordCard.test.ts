import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { createPinia, setActivePinia, type Pinia } from "pinia";
import { NMessageProvider } from "naive-ui";
import { defineComponent, h } from "vue";
import { createMemoryHistory, createRouter, type Router } from "vue-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as authApi from "@/api/auth";
import { ApiError, TOKEN_STORAGE_KEY } from "@/api/client";
import { useAuthStore } from "@/stores/auth";
import ChangePasswordCard from "./ChangePasswordCard.vue";

vi.mock("@/api/auth", () => ({
  login: vi.fn(),
  register: vi.fn(),
  logout: vi.fn(),
  me: vi.fn(),
  changePassword: vi.fn(),
}));

const authApiMock = vi.mocked(authApi);

let pinia: Pinia;
let router: Router;

interface SubmitValues {
  current: string;
  next: string;
  confirm: string;
}

// NMessageProvider 的消息容器 teleport 到 document.body，统一做 body 级断言；
// 用例结束后卸载应用，避免 teleport 内容跨用例残留。
enableAutoUnmount(afterEach);

async function mountCard(): Promise<VueWrapper> {
  const Harness = defineComponent({
    render() {
      return h(NMessageProvider, () => h(ChangePasswordCard));
    },
  });
  const wrapper = mount(Harness, { global: { plugins: [pinia, router] } });
  await flushPromises();
  return wrapper;
}

async function fillAndSubmit(wrapper: VueWrapper, values: SubmitValues): Promise<void> {
  await wrapper.find("input[placeholder='当前密码']").setValue(values.current);
  await wrapper.find("input[placeholder='新密码（至少 12 位）']").setValue(values.next);
  await wrapper.find("input[placeholder='再次输入新密码']").setValue(values.confirm);
  await wrapper.find("form").trigger("submit");
  await flushPromises();
}

describe("ChangePasswordCard", () => {
  beforeEach(async () => {
    localStorage.clear();
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|token");
    vi.resetAllMocks();
    pinia = createPinia();
    setActivePinia(pinia);
    router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/tokens", component: { render: () => null } },
        { path: "/login", component: { render: () => null } },
      ],
    });
    await router.push("/tokens");
    await router.isReady();
  });

  it("修改成功后清空登录态并跳转 /login", async () => {
    authApiMock.changePassword.mockResolvedValue(null);
    const wrapper = await mountCard();
    const auth = useAuthStore(pinia);
    const clearSpy = vi.spyOn(auth, "clear");

    await fillAndSubmit(wrapper, {
      current: "old-password",
      next: "new-password-12",
      confirm: "new-password-12",
    });

    expect(authApiMock.changePassword).toHaveBeenCalledWith("old-password", "new-password-12");
    expect(document.body.textContent).toContain("密码已修改，请重新登录");
    expect(clearSpy).toHaveBeenCalledTimes(1);
    expect(auth.token).toBeNull();
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBeNull();
    expect(router.currentRoute.value.path).toBe("/login");
  });

  it("旧密码错误（20002）时就地展示错误且不清空登录态", async () => {
    authApiMock.changePassword.mockRejectedValue(new ApiError(20002, "当前密码不正确", 401));
    const wrapper = await mountCard();
    const auth = useAuthStore(pinia);
    const clearSpy = vi.spyOn(auth, "clear");

    await fillAndSubmit(wrapper, {
      current: "wrong-old-password",
      next: "new-password-12",
      confirm: "new-password-12",
    });

    expect(authApiMock.changePassword).toHaveBeenCalledWith(
      "wrong-old-password",
      "new-password-12",
    );
    expect(wrapper.text()).toContain("当前密码不正确");
    expect(clearSpy).not.toHaveBeenCalled();
    expect(auth.token).toBe("7|token");
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBe("7|token");
    expect(router.currentRoute.value.path).toBe("/tokens");
  });

  it("两次输入的新密码不一致时不发起请求", async () => {
    const wrapper = await mountCard();

    await fillAndSubmit(wrapper, {
      current: "old-password",
      next: "new-password-12",
      confirm: "different-password",
    });

    expect(authApiMock.changePassword).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("两次输入的密码不一致");
  });

  it("新密码不足 12 位时不发起请求", async () => {
    const wrapper = await mountCard();

    await fillAndSubmit(wrapper, {
      current: "old-password",
      next: "short-pass",
      confirm: "short-pass",
    });

    expect(authApiMock.changePassword).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("新密码至少 12 个字符");
  });

  it("新密码超过 72 字节时不发起请求", async () => {
    const wrapper = await mountCard();

    await fillAndSubmit(wrapper, {
      current: "old-password",
      next: "a".repeat(73),
      confirm: "a".repeat(73),
    });

    expect(authApiMock.changePassword).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("新密码最长 72 字节");
  });
});
