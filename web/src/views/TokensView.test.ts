import {
  enableAutoUnmount,
  flushPromises,
  mount,
  type VueWrapper,
} from "@vue/test-utils";
import { createPinia } from "pinia";
import { NMessageProvider } from "naive-ui";
import { defineComponent, h } from "vue";
import { createMemoryHistory, createRouter, type Router } from "vue-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as tokensApi from "@/api/tokens";
import type { IssuedToken, TokenView } from "@/api/tokens";
import TokensView from "./TokensView.vue";

vi.mock("@/api/tokens", () => ({
  listTokens: vi.fn(),
  createToken: vi.fn(),
  revokeToken: vi.fn(),
}));

vi.mock("@/api/auth", () => ({
  login: vi.fn(),
  register: vi.fn(),
  logout: vi.fn(),
  me: vi.fn(),
  changePassword: vi.fn(),
}));

const tokensApiMock = vi.mocked(tokensApi);

const webToken: TokenView = {
  id: 1,
  name: "浏览器会话",
  kind: "web",
  abilities: ["*"],
  expires_at: null,
  last_used_at: null,
  created_at: "2026-10-04T12:00:00Z",
};

const apiToken: TokenView = {
  id: 2,
  name: "blog-client",
  kind: "api",
  abilities: ["*"],
  expires_at: "2027-10-04T12:00:00Z",
  last_used_at: "2026-10-03T08:30:00Z",
  created_at: "2026-09-01T09:00:00Z",
};

const issuedFixture: IssuedToken = {
  token: "9|abcdefghijklmnopqrstuvwxyz0123456789ABCD",
  info: {
    id: 9,
    name: "blog-client",
    kind: "api",
    abilities: ["*"],
    expires_at: null,
    last_used_at: null,
    created_at: "2026-10-04T12:00:00Z",
  },
};

// jsdom 未实现 ResizeObserver，naive-ui 布局/弹层依赖它
class ResizeObserverStub {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}
vi.stubGlobal("ResizeObserver", ResizeObserverStub);

function bodyButton(text: string): HTMLButtonElement | null {
  return (
    Array.from(document.body.querySelectorAll("button")).find((button) =>
      button.textContent?.includes(text),
    ) ?? null
  );
}

// NModal 的离场过渡在 jsdom 中不会结束，隐藏内容会残留在 document.body，
// 干扰后续测试的 body 级查询；每个用例结束后卸载全部应用（含 teleport 内容）。
enableAutoUnmount(afterEach);

async function mountView(): Promise<{ wrapper: VueWrapper; router: Router }> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/tokens", component: TokensView },
      { path: "/login", component: { render: () => null } },
    ],
  });
  await router.push("/tokens");
  await router.isReady();

  const Harness = defineComponent({
    render() {
      return h(NMessageProvider, () => h(TokensView));
    },
  });
  const wrapper = mount(Harness, { global: { plugins: [createPinia(), router] } });
  await flushPromises();
  return { wrapper, router };
}

describe("TokensView", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetAllMocks();
    Reflect.deleteProperty(navigator, "clipboard");
  });

  it("渲染列表各字段：无过期显示「永不过期」、从未使用显示「—」", async () => {
    tokensApiMock.listTokens.mockResolvedValue([webToken, apiToken]);
    const { wrapper } = await mountView();

    const text = wrapper.text();
    expect(text).toContain("浏览器会话");
    expect(text).toContain("blog-client");
    expect(text).toContain("web");
    expect(text).toContain("api");
    expect(text).toContain("*");
    expect(text).toContain("永不过期");
    expect(text).toContain("—");
    expect(text).toMatch(/\d{4}-\d{2}-\d{2} \d{2}:\d{2}/);
  });

  it("空列表显示空状态", async () => {
    tokensApiMock.listTokens.mockResolvedValue([]);
    const { wrapper } = await mountView();

    expect(wrapper.text()).toContain("暂无 Token");
  });

  it("创建成功后明文 Token 只在弹窗出现一次，确认关闭后消失并刷新列表", async () => {
    tokensApiMock.listTokens
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([issuedFixture.info]);
    tokensApiMock.createToken.mockResolvedValue(issuedFixture);
    const { wrapper } = await mountView();

    await wrapper.find("input[placeholder='例如：blog-client']").setValue("blog-client");
    await wrapper.find("form.tokens-view__form").trigger("submit");
    await flushPromises();

    expect(tokensApiMock.createToken).toHaveBeenCalledWith({ name: "blog-client" });
    // 明文只出现在弹窗（teleport 到 body）里一次，页面本体不含明文
    const bodyText = document.body.textContent ?? "";
    expect(bodyText.split(issuedFixture.token).length - 1).toBe(1);
    expect(wrapper.text()).not.toContain(issuedFixture.token);
    expect(document.body.textContent).toContain("关闭后将无法再次查看，请立即保存");
    // 弹窗未关闭前不刷新列表
    expect(tokensApiMock.listTokens).toHaveBeenCalledTimes(1);

    const closeButton = bodyButton("我已保存，关闭");
    expect(closeButton).toBeTruthy();
    closeButton?.click();
    await flushPromises();

    expect(document.body.textContent).not.toContain(issuedFixture.token);
    expect(tokensApiMock.listTokens).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).toContain(issuedFixture.info.name);
  });

  it("复制按钮把明文 Token 写入剪贴板", async () => {
    tokensApiMock.listTokens.mockResolvedValue([]);
    tokensApiMock.createToken.mockResolvedValue(issuedFixture);
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });
    const { wrapper } = await mountView();

    await wrapper.find("input[placeholder='例如：blog-client']").setValue("blog-client");
    await wrapper.find("form.tokens-view__form").trigger("submit");
    await flushPromises();

    const copyButton = bodyButton("复制 Token");
    expect(copyButton).toBeTruthy();
    copyButton?.click();
    await flushPromises();

    expect(writeText).toHaveBeenCalledWith(issuedFixture.token);
  });

  it("名称为空时不发起创建请求并就地提示", async () => {
    tokensApiMock.listTokens.mockResolvedValue([]);
    const { wrapper } = await mountView();

    await wrapper.find("form.tokens-view__form").trigger("submit");
    await flushPromises();

    expect(tokensApiMock.createToken).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("请输入 Token 名称");
  });

  it("吊销需要确认，确认后调用 DELETE 并刷新列表", async () => {
    tokensApiMock.listTokens.mockResolvedValue([apiToken]);
    tokensApiMock.revokeToken.mockResolvedValue(null);
    const { wrapper } = await mountView();

    const revokeButton = wrapper.findAll("button").find((button) => button.text() === "吊销");
    expect(revokeButton).toBeTruthy();
    await revokeButton?.trigger("click");
    await flushPromises();

    // 仅弹出确认框，尚未调用 DELETE
    expect(tokensApiMock.revokeToken).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("吊销后该 Token 立即失效");

    const confirmButton = wrapper
      .findAll("button")
      .find((button) => button.text() === "确认吊销");
    expect(confirmButton).toBeTruthy();
    await confirmButton?.trigger("click");
    await flushPromises();

    expect(tokensApiMock.revokeToken).toHaveBeenCalledWith(apiToken.id);
    expect(tokensApiMock.listTokens).toHaveBeenCalledTimes(2);
  });
});
