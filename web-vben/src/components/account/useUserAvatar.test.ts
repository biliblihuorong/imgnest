import { i18n } from "@vben/locales";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { defineComponent, h, nextTick, ref } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { UserView } from "@/api/types";
import fallbackAvatar from "@/assets/account-avatar.svg";
import { AVATAR_LOAD_TIMEOUT_MS, useUserAvatar } from "./useUserAvatar";

interface FakeImage {
  src: string;
  referrerPolicy: string;
  onload: (() => void) | null;
  onerror: (() => void) | null;
}

let probes: FakeImage[];

class ImageStub implements FakeImage {
  src = "";
  referrerPolicy = "";
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  constructor() {
    probes.push(this);
  }
}

vi.stubGlobal("Image", ImageStub);

function makeUser(overrides: Partial<UserView> = {}): UserView {
  return {
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
    avatar_url: null,
    avatar_config_version: 0,
    ...overrides,
  };
}

async function mountHarness(user: ReturnType<typeof ref<UserView | null>>) {
  const Harness = defineComponent({
    setup() {
      const avatar = useUserAvatar(user);
      return () => h("span", { "data-avatar": avatar.value });
    },
  });
  const wrapper = mount(Harness);
  await flushPromises();
  return wrapper;
}

const avatarText = (wrapper: VueWrapper) => wrapper.find("span").attributes("data-avatar");

beforeEach(() => {
  probes = [];
  void i18n.global.locale;
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useUserAvatar", () => {
  it("无头像地址时保持本地默认图，不发起远程请求", async () => {
    const user = ref<UserView | null>(makeUser({ avatar_url: null }));
    const wrapper = await mountHarness(user);
    expect(avatarText(wrapper)).toBe(fallbackAvatar);
    expect(probes).toHaveLength(0);
  });

  it("远程加载成功后替换为远程地址并禁用来源页referrer", async () => {
    const remote = "https://weavatar.com/avatar/abc?s=160&d=404";
    const user = ref<UserView | null>(makeUser({ avatar_url: remote }));
    const wrapper = await mountHarness(user);
    expect(avatarText(wrapper)).toBe(fallbackAvatar);
    expect(probes).toHaveLength(1);
    expect(probes[0].src).toBe(remote);
    expect(probes[0].referrerPolicy).toBe("no-referrer");
    probes[0].onload?.();
    await nextTick();
    expect(avatarText(wrapper)).toBe(remote);
  });

  it("加载失败保持本地默认图且不重试", async () => {
    const remote = "https://weavatar.com/avatar/abc?s=160&d=404";
    const user = ref<UserView | null>(makeUser({ avatar_url: remote }));
    const wrapper = await mountHarness(user);
    probes[0].onerror?.();
    await nextTick();
    expect(avatarText(wrapper)).toBe(fallbackAvatar);
    // 一次失败后不再发起第二次探测。
    await nextTick();
    expect(probes).toHaveLength(1);
  });

  it("超时保持本地默认图，迟到的成功不覆盖", async () => {
    vi.useFakeTimers();
    const remote = "https://weavatar.com/avatar/abc?s=160&d=404";
    const user = ref<UserView | null>(makeUser({ avatar_url: remote }));
    const wrapper = await mountHarness(user);
    vi.advanceTimersByTime(AVATAR_LOAD_TIMEOUT_MS + 1);
    expect(avatarText(wrapper)).toBe(fallbackAvatar);
    // 超时后处理器已被摘除，迟到的成功不能替换头像。
    probes[0].onload?.();
    await nextTick();
    expect(avatarText(wrapper)).toBe(fallbackAvatar);
  });

  it("账户切换后旧请求的迟到成功不覆盖新头像", async () => {
    const first = "https://weavatar.com/avatar/first?s=160&d=404";
    const second = "https://weavatar.com/avatar/second?s=160&d=404";
    const user = ref<UserView | null>(makeUser({ id: 7, avatar_url: first }));
    const wrapper = await mountHarness(user);
    expect(probes).toHaveLength(1);
    user.value = makeUser({ id: 8, avatar_url: second });
    await nextTick();
    // 切换即重置为本地默认图；旧探测的成功不算数。
    expect(avatarText(wrapper)).toBe(fallbackAvatar);
    probes[0].onload?.();
    await nextTick();
    expect(avatarText(wrapper)).toBe(fallbackAvatar);
    probes[1].onload?.();
    await nextTick();
    expect(avatarText(wrapper)).toBe(second);
  });

  it("资料页尺寸把 s=160 重写为 s=320", async () => {
    const remote = "https://weavatar.com/avatar/abc?s=160&d=404";
    const user = ref<UserView | null>(makeUser({ avatar_url: remote }));
    const Harness = defineComponent({
      setup() {
        const avatar = useUserAvatar(user, 320);
        return () => h("span", { "data-avatar": avatar.value });
      },
    });
    mount(Harness);
    await flushPromises();
    expect(probes[0].src).toBe("https://weavatar.com/avatar/abc?s=320&d=404");
  });
});
