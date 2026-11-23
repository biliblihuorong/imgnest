import {
  inject,
  onActivated,
  onBeforeUnmount,
  onDeactivated,
  provide,
  ref,
  shallowRef,
  watch,
  type InjectionKey,
} from "vue";
import { listUserPluginSettings, type UserPluginSettings } from "@/api/plugins";
import { formatApiError } from "@/locales/errors";
import { useAuthStore } from "@/stores/auth";

/** Account-page data shared by navigation and its selected settings card. */
function createUserPlugins() {
  const auth = useAuthStore();
  const plugins = ref<UserPluginSettings[]>([]);
  const error = shallowRef("");
  const loading = shallowRef(false);
  let controller: AbortController | undefined;
  let active = true;
  function clear(): void {
    controller?.abort();
    plugins.value = [];
    error.value = "";
    loading.value = false;
  }
  async function load(): Promise<void> {
    clear();
    if (!active || !auth.user || !auth.token) return;
    const request = new AbortController();
    controller = request;
    loading.value = true;
    try {
      const result = await listUserPluginSettings(request.signal);
      if (!request.signal.aborted) plugins.value = result;
    } catch (cause) {
      if (!request.signal.aborted) error.value = formatApiError(cause, "account.plugins.loadError");
    } finally {
      if (!request.signal.aborted) loading.value = false;
    }
  }
  function saved(plugin: UserPluginSettings): void {
    plugins.value = plugins.value.map((item) => (item.name === plugin.name ? plugin : item));
  }
  watch(
    () => [auth.token, auth.user?.id, auth.user?.group_id],
    () => void load(),
    { immediate: true, flush: "sync" },
  );
  onDeactivated(() => {
    active = false;
    clear();
  });
  onActivated(() => {
    if (!active) {
      active = true;
      void load();
    }
  });
  onBeforeUnmount(clear);
  return { plugins, error, loading, load, saved };
}

type UserPlugins = ReturnType<typeof createUserPlugins>;
const userPluginsKey: InjectionKey<UserPlugins> = Symbol("account-personal-settings");

/** Own the account page's request lifecycle and make it available to cards. */
export function provideUserPlugins(): UserPlugins {
  const state = createUserPlugins();
  provide(userPluginsKey, state);
  return state;
}

/** Standalone cards own their state; cards in an account page reuse it. */
export function useUserPlugins() {
  const provided = inject(userPluginsKey, null);
  return { state: provided ?? createUserPlugins(), provided: provided !== null };
}
