import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { createPinia } from "pinia";
import { NDatePicker, NMessageProvider } from "naive-ui";
import { defineComponent, h, nextTick } from "vue";
import { createMemoryHistory, createRouter, RouterView, type Router } from "vue-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as tokensApi from "@/api/tokens";
import type { IssuedToken, TokenView } from "@/api/tokens";
import TokensView from "./TokensView.vue";
import { i18n } from "@vben/locales";
import { ApiError } from "@/api/client";

vi.mock("@/api/tokens", () => ({
  listTokens: vi.fn(),
  createToken: vi.fn(),
  revokeToken: vi.fn(),
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
      return h(NMessageProvider, () => h(RouterView));
    },
  });
  const wrapper = mount(Harness, { global: { plugins: [createPinia(), router] } });
  await flushPromises();
  return { wrapper, router };
}

describe("TokensView", () => {
  it("页面提供唯一主标题，刷新 Token 列表时保留未提交的名称", async () => {
    tokensApiMock.listTokens.mockResolvedValue([webToken]);
    const { wrapper } = await mountView();
    expect(wrapper.findAll("h1")).toHaveLength(1);
    expect(wrapper.find("h1").text()).toBe("Token 与安全");
    await wrapper.find("input[placeholder='例如：blog-client']").setValue("unfinished-client");

    const refresh = wrapper.findAll("button").find((button) => button.text() === "刷新列表");
    expect(refresh).toBeDefined();
    await refresh!.trigger("click");
    await flushPromises();

    expect(tokensApiMock.listTokens).toHaveBeenCalledTimes(2);
    expect(
      (wrapper.find("input[placeholder='例如：blog-client']").element as HTMLInputElement).value,
    ).toBe("unfinished-client");
    expect(tokensApiMock.createToken).not.toHaveBeenCalled();
  });

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
    expect(text).toContain("2026年10月4日");
  });

  it("空列表显示空状态", async () => {
    tokensApiMock.listTokens.mockResolvedValue([]);
    const { wrapper } = await mountView();

    expect(wrapper.text()).toContain("暂无 Token");
  });

  it("创建成功后明文 Token 只在弹窗出现一次，确认关闭后消失并刷新列表", async () => {
    tokensApiMock.listTokens.mockResolvedValueOnce([]).mockResolvedValueOnce([issuedFixture.info]);
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

  it("refreshes after creation when closing the secret overlaps an older token-list request", async () => {
    let finishOlderList!: (value: TokenView[]) => void;
    tokensApiMock.listTokens
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            finishOlderList = resolve;
          }),
      )
      .mockResolvedValueOnce([issuedFixture.info]);
    tokensApiMock.createToken.mockResolvedValue(issuedFixture);
    const { wrapper } = await mountView();

    await wrapper.find("input[placeholder='例如：blog-client']").setValue("blog-client");
    await wrapper.find("form.tokens-view__form").trigger("submit");
    await flushPromises();
    bodyButton("我已保存，关闭")!.click();
    await flushPromises();
    expect(document.body.textContent).not.toContain(issuedFixture.token);

    finishOlderList([]);
    await flushPromises();
    expect(tokensApiMock.listTokens).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).toContain(issuedFixture.info.name);
    expect(document.body.textContent).not.toContain(issuedFixture.token);
  });

  it("discards a queued post-create refresh when the session ends", async () => {
    let finishOlderList!: (value: TokenView[]) => void;
    tokensApiMock.listTokens.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishOlderList = resolve;
        }),
    );
    tokensApiMock.createToken.mockResolvedValue(issuedFixture);
    const { wrapper } = await mountView();
    await wrapper.find("input[placeholder='例如：blog-client']").setValue("blog-client");
    await wrapper.find("form.tokens-view__form").trigger("submit");
    await flushPromises();
    bodyButton("我已保存，关闭")!.click();
    await flushPromises();
    window.dispatchEvent(new Event("imgnest:session-cleared"));
    finishOlderList([{ ...webToken, name: "Ended session credential" }]);
    await flushPromises();
    expect(tokensApiMock.listTokens).toHaveBeenCalledTimes(1);
    expect(wrapper.text()).not.toContain("Ended session credential");
    expect(document.body.textContent).not.toContain(issuedFixture.token);
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

    // 仅弹出确认框（teleport 到 body），尚未调用 DELETE
    expect(tokensApiMock.revokeToken).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("吊销后该 Token 立即失效");

    const confirmButton = Array.from(document.body.querySelectorAll("button")).find(
      (button) => button.textContent?.trim() === "确认吊销",
    );
    expect(confirmButton).toBeTruthy();
    confirmButton!.click();
    await flushPromises();

    expect(tokensApiMock.revokeToken).toHaveBeenCalledWith(apiToken.id);
    expect(tokensApiMock.listTokens).toHaveBeenCalledTimes(2);
  });
  it("reactively translates headings, columns, validation, and integration help", async () => {
    tokensApiMock.listTokens.mockResolvedValue([apiToken]);
    const { wrapper } = await mountView();
    await wrapper.find("form.tokens-view__form").trigger("submit");
    await flushPromises();
    i18n.global.locale.value = "en-US";
    await nextTick();
    await flushPromises();
    expect(wrapper.find("h1").text()).toBe("Tokens & security");
    expect(wrapper.text()).toContain("Enter a token name");
    expect(wrapper.text()).toContain("Last used");
    expect(wrapper.text()).toContain("Oct 4, 2027");
    expect(wrapper.text()).toContain("PicGo / Lsky setup");
    expect(wrapper.text()).toContain(`${window.location.origin}/api/v1/upload`);
    expect(wrapper.text()).toContain("Bearer YOUR_API_TOKEN");
    expect(wrapper.text()).toContain("Native token management remains available");
    expect(wrapper.text()).not.toContain("创建 API Token");
    expect(tokensApiMock.listTokens).toHaveBeenCalledTimes(1);
  });

  it("guards rapid duplicate creation before async validation finishes", async () => {
    tokensApiMock.listTokens.mockResolvedValue([]);
    tokensApiMock.createToken.mockResolvedValue(issuedFixture);
    const { wrapper } = await mountView();
    await wrapper.find("input[placeholder='例如：blog-client']").setValue("blog-client");
    const form = wrapper.find("form.tokens-view__form");
    await Promise.all([form.trigger("submit"), form.trigger("submit")]);
    await flushPromises();
    expect(tokensApiMock.createToken).toHaveBeenCalledTimes(1);
    expect(localStorage.length).toBe(0);
    expect(sessionStorage.length).toBe(0);
    const close = bodyButton("我已保存，关闭")!;
    close.click();
    close.click();
    await flushPromises();
    expect(tokensApiMock.listTokens).toHaveBeenCalledTimes(2);
  });

  it("rejects past expiry without sending a token request", async () => {
    tokensApiMock.listTokens.mockResolvedValue([]);
    const { wrapper } = await mountView();
    await wrapper.find("input[placeholder='例如：blog-client']").setValue("blog-client");
    wrapper.findComponent(NDatePicker).vm.$emit("update:value", Date.now() - 60000);
    await wrapper.find("form.tokens-view__form").trigger("submit");
    await flushPromises();
    expect(tokensApiMock.createToken).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("过期时间必须晚于当前时间");
  });

  it("submits explicit future expiry as UTC without adding token capabilities", async () => {
    tokensApiMock.listTokens.mockResolvedValue([]);
    tokensApiMock.createToken.mockResolvedValue(issuedFixture);
    const { wrapper } = await mountView();
    await wrapper.find("input[placeholder='例如：blog-client']").setValue("  blog-client  ");
    wrapper.findComponent(NDatePicker).vm.$emit("update:value", Date.parse("2099-10-04T12:00:00Z"));
    await wrapper.find("form.tokens-view__form").trigger("submit");
    await flushPromises();
    expect(tokensApiMock.createToken).toHaveBeenCalledWith({
      name: "blog-client",
      expires_at: "2099-10-04T12:00:00.000Z",
    });
  });

  it("localizes load failures on locale change without exposing server diagnostics", async () => {
    tokensApiMock.listTokens.mockRejectedValue(
      new ApiError(50001, "internal secret diagnostic", 500),
    );
    const { wrapper } = await mountView();
    expect(wrapper.text()).not.toContain("internal secret diagnostic");
    i18n.global.locale.value = "en-US";
    await nextTick();
    expect(wrapper.text()).toContain("The service is temporarily unavailable.");
  });

  it("guards repeated revoke confirmation and cancellation sends no request", async () => {
    tokensApiMock.listTokens.mockResolvedValue([apiToken]);
    let complete!: (value: null) => void;
    tokensApiMock.revokeToken.mockImplementation(
      () =>
        new Promise((resolve) => {
          complete = resolve;
        }),
    );
    const { wrapper } = await mountView();
    const open = () => wrapper.findAll("button").find((button) => button.text() === "吊销")!;
    await open().trigger("click");
    await flushPromises();
    const cancel = Array.from(document.body.querySelectorAll("button")).find(
      (button) => button.textContent?.trim() === "取消",
    )!;
    await cancel.click();
    await flushPromises();
    expect(tokensApiMock.revokeToken).not.toHaveBeenCalled();
    await open().trigger("click");
    await flushPromises();
    const confirm = Array.from(document.body.querySelectorAll("button")).find(
      (button) => button.textContent?.trim() === "确认吊销",
    )!;
    await Promise.all([confirm.click(), confirm.click()]);
    await flushPromises();
    expect(tokensApiMock.revokeToken).toHaveBeenCalledTimes(1);
    complete(null);
    await flushPromises();
    expect(tokensApiMock.listTokens).toHaveBeenCalledTimes(2);
  });

  it("guards repeated copy and safely handles clipboard rejection", async () => {
    tokensApiMock.listTokens.mockResolvedValue([]);
    tokensApiMock.createToken.mockResolvedValue(issuedFixture);
    let rejectCopy!: (reason: Error) => void;
    const writeText = vi.fn(
      () =>
        new Promise<void>((_resolve, reject) => {
          rejectCopy = reject;
        }),
    );
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    const { wrapper } = await mountView();
    await wrapper.find("input[placeholder='例如：blog-client']").setValue("blog-client");
    await wrapper.find("form.tokens-view__form").trigger("submit");
    await flushPromises();
    const copy = bodyButton("复制 Token")!;
    copy.click();
    copy.click();
    await flushPromises();
    expect(writeText).toHaveBeenCalledTimes(1);
    rejectCopy(new Error("clipboard diagnostic"));
    await flushPromises();
    expect(document.body.textContent).toContain("复制失败，请手动全选复制");
    expect(document.body.textContent).not.toContain("clipboard diagnostic");
  });

  it("clears one-time plaintext on navigation and never reopens it on back", async () => {
    tokensApiMock.listTokens.mockResolvedValue([]);
    tokensApiMock.createToken.mockResolvedValue(issuedFixture);
    const { wrapper, router } = await mountView();
    await wrapper.find("input[placeholder='例如：blog-client']").setValue("blog-client");
    await wrapper.find("form.tokens-view__form").trigger("submit");
    await flushPromises();
    expect(document.body.textContent).toContain(issuedFixture.token);
    await router.push("/login");
    await flushPromises();
    expect(document.body.textContent).not.toContain(issuedFixture.token);
    await router.push("/tokens");
    await flushPromises();
    expect(document.body.textContent).not.toContain(issuedFixture.token);
    expect(tokensApiMock.createToken).toHaveBeenCalledTimes(1);
  });

  it("discards an in-flight plaintext response after leaving the page", async () => {
    tokensApiMock.listTokens.mockResolvedValue([]);
    let complete!: (value: IssuedToken) => void;
    tokensApiMock.createToken.mockImplementation(
      () =>
        new Promise((resolve) => {
          complete = resolve;
        }),
    );
    const { wrapper, router } = await mountView();
    await wrapper.find("input[placeholder='例如：blog-client']").setValue("blog-client");
    await wrapper.find("form.tokens-view__form").trigger("submit");
    await flushPromises();
    await router.push("/login");
    complete(issuedFixture);
    await flushPromises();
    expect(document.body.textContent).not.toContain(issuedFixture.token);
    expect(localStorage.length).toBe(0);
  });

  it("clears plaintext immediately when the session is cleared", async () => {
    tokensApiMock.listTokens.mockResolvedValue([]);
    tokensApiMock.createToken.mockResolvedValue(issuedFixture);
    const { wrapper } = await mountView();
    await wrapper.find("input[placeholder='例如：blog-client']").setValue("blog-client");
    await wrapper.find("form.tokens-view__form").trigger("submit");
    await flushPromises();
    window.dispatchEvent(new Event("imgnest:session-cleared"));
    await nextTick();
    expect(document.body.textContent).not.toContain(issuedFixture.token);
  });

  it("discards a late create response after session clearing", async () => {
    tokensApiMock.listTokens.mockResolvedValue([]);
    let complete!: (value: IssuedToken) => void;
    tokensApiMock.createToken.mockImplementation(
      () =>
        new Promise((resolve) => {
          complete = resolve;
        }),
    );
    const { wrapper } = await mountView();
    await wrapper.find("input[placeholder='例如：blog-client']").setValue("blog-client");
    await wrapper.find("form.tokens-view__form").trigger("submit");
    await flushPromises();
    window.dispatchEvent(new Event("imgnest:session-cleared"));
    complete(issuedFixture);
    await flushPromises();
    expect(document.body.textContent).not.toContain(issuedFixture.token);
    expect(localStorage.length).toBe(0);
    expect(sessionStorage.length).toBe(0);
  });
});
