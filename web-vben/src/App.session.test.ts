import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { defineComponent, h, onUnmounted } from "vue";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { updatePreferences } from "@vben/preferences";
import { me } from "@/api/auth";
import { TOKEN_STORAGE_KEY } from "@/api/client";
import type { UserView } from "@/api/types";
import { useAccountScope } from "@/components/account/useAccountScope";
import { useUploadQueue } from "@/components/upload/useUploadQueue";
import { initShell, setShell } from "@/integrations/shell/useShell";
import { connectVbenSession } from "@/integrations/vben/session";
import { imgnestPreferences } from "@/integrations/vben/preferences";
import { setupRouter } from "@/router";
import { useAuthStore } from "@/stores/auth";
import App from "./App.vue";

vi.mock("@/api/auth", () => ({
  me: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  exchangeSsoTicket: vi.fn(),
}));
vi.mock("@/api/site", () => ({
  fetchSite: vi
    .fn()
    .mockResolvedValue({
      site_name: "ImgNest",
      register_enabled: false,
      gallery_enabled: false,
      login_providers: [],
    }),
}));
vi.mock("@/api/albums", () => ({
  listAlbums: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, size: 20 }),
}));
// 保留真实 ClassicLayout 与 RouterView；Vben 导航栏的 DOM 测量不属于会话生命周期契约。
vi.mock("@vben/layouts", async () => {
  const { RouterView } = await import("vue-router");
  return {
    BasicLayout: { inheritAttrs: false, setup: () => () => h(RouterView) },
    UserDropdown: { setup: () => () => null },
  };
});
enableAutoUnmount(afterEach);

beforeEach(() => {
  localStorage.clear();
  vi.stubGlobal("matchMedia", (media: string) => ({
    matches: false,
    media,
    onchange: null,
    addEventListener() {},
    removeEventListener() {},
    addListener() {},
    removeListener() {},
    dispatchEvent: () => false,
  }));
  updatePreferences(imgnestPreferences);
  initShell();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
});
afterEach(() => vi.unstubAllGlobals());

const alice: UserView = {
  id: 1,
  role: "user",
  username: "alice",
  email: "alice@example.com",
  status: "enabled",
  display_name: "",
  group_id: 1,
  used_bytes: 0,
  created_at: "2026-10-13T00:00:00Z",
  avatar_provider: "weavatar",
  avatar_url: "",
  avatar_config_version: 0,
};

it.each(["marvis", "classic"] as const)(
  "%s 会话切换后同一上传路由建立新队列和账户作用域",
  async (shell) => {
    setShell(shell);
    localStorage.setItem(TOKEN_STORAGE_KEY, "A");
    const pinia = createPinia();
    const auth = useAuthStore(pinia);
    auth.user = alice;
    const bob = { ...alice, id: 2, username: "bob" };
    vi.mocked(me).mockResolvedValue(bob);
    let queue!: ReturnType<typeof useUploadQueue>;
    let scope!: ReturnType<typeof useAccountScope>;
    const unmounted = vi.fn();
    const cleared = vi.fn();
    const upload = defineComponent({
      setup() {
        queue = useUploadQueue(() => ({ policyId: 1, albumId: null, isPublic: false }));
        scope = useAccountScope(cleared);
        onUnmounted(unmounted);
        return () =>
          h(
            "div",
            { "data-testid": "session-probe" },
            queue.items.value.map((item) => item.name).join(","),
          );
      },
    });
    const router = setupRouter();
    router.removeRoute("upload");
    router.addRoute({
      path: "/upload",
      name: "session-upload",
      component: upload,
      meta: { title: "common.nav.upload" },
    });
    await router.push("/upload");
    const stop = connectVbenSession(pinia, router);
    const wrapper = mount(App, { attachTo: document.body, global: { plugins: [pinia, router] } });
    try {
      await vi.waitFor(
        () => expect(wrapper.find('[data-testid="session-probe"]').exists()).toBe(true),
        { timeout: 12000 },
      );
      const oldQueue = queue;
      const oldScope = scope;
      oldQueue.onFiles([new File(["a"], "alice.png")]);
      expect(oldQueue.items.value).toHaveLength(1);
      localStorage.setItem(TOKEN_STORAGE_KEY, "B");
      window.dispatchEvent(
        new StorageEvent("storage", {
          key: TOKEN_STORAGE_KEY,
          oldValue: "A",
          newValue: "B",
          storageArea: localStorage,
        }),
      );
      await flushPromises();
      await flushPromises();
      expect(auth.user?.id).toBe(2);
      expect(router.currentRoute.value.path).toBe("/upload");
      expect(oldQueue.items.value).toHaveLength(0);
      expect(oldScope.available.value).toBe(false);
      queue.onFiles([new File(["b"], "bob.png")]);
      expect(queue.items.value).toHaveLength(1);
      expect(scope.available.value).toBe(true);
      expect(unmounted).toHaveBeenCalled();
      const currentQueue = queue;
      auth.setUser({ ...bob, display_name: "Bob" });
      await flushPromises();
      expect(queue).toBe(currentQueue);
      expect(queue.items.value[0]?.name).toBe("bob.png");
    } finally {
      wrapper.unmount();
      stop();
    }
  },
);
