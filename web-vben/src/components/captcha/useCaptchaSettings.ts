import { computed, onBeforeUnmount, onMounted, shallowRef, watch } from "vue";
import {
  getCaptchaSettings,
  saveCaptchaDraft,
  testCaptchaDraft,
  setCaptchaActivation,
  type CaptchaAction,
  type CaptchaAdminView,
  type PublicCaptchaConfig,
} from "@/api/captcha";
import { ApiError } from "@/api/client";
import { formatApiError } from "@/locales/errors";

/** Administrator draft state is local to this mounted card, including write-only secrets. */
export function useCaptchaSettings() {
  const settings = shallowRef<CaptchaAdminView | null>(null);
  const siteKey = shallowRef("");
  const hostnameInput = shallowRef("");
  const secret = shallowRef("");
  const clearSecret = shallowRef(false);
  const legacyAcknowledged = shallowRef(false);
  const v1Acknowledged = shallowRef(false);
  const loading = shallowRef(true);
  const pending = shallowRef(false);
  const stale = shallowRef(false);
  const serverError = shallowRef<unknown>(null);
  const notice = shallowRef<"saved" | "activated" | "disabled" | null>(null);
  let controller: AbortController | undefined;
  let disposed = false;
  const busy = computed(() => loading.value || pending.value);
  const errorMessage = computed(() => (serverError.value ? formatApiError(serverError.value) : ""));
  const hostnames = computed(() =>
    hostnameInput.value
      .split(/[\n,]/)
      .map((name) => name.trim())
      .filter(Boolean),
  );
  const candidate = computed(() => settings.value?.draft ?? settings.value?.active);
  const dirty = computed(
    () =>
      siteKey.value.trim() !== (candidate.value?.site_key ?? "") ||
      JSON.stringify(hostnames.value) !== JSON.stringify(candidate.value?.hostnames ?? []) ||
      secret.value !== "" ||
      clearSecret.value,
  );
  const available = computed(
    () => !!settings.value?.configuration_available && !busy.value && !stale.value,
  );
  const canSave = computed(
    () =>
      available.value &&
      !!siteKey.value.trim() &&
      hostnames.value.length > 0 &&
      (clearSecret.value
        ? !settings.value?.enabled && !secret.value
        : !!secret.value || !!candidate.value?.secret_configured),
  );
  const canTest = computed(
    () => available.value && !dirty.value && !!settings.value?.draft?.secret_configured,
  );
  const canActivate = computed(
    () =>
      canTest.value &&
      !!settings.value?.draft?.tested &&
      settings.value.draft.tested_actions.includes("login") &&
      settings.value.draft.tested_actions.includes("register") &&
      legacyAcknowledged.value &&
      v1Acknowledged.value,
  );
  const draftConfig = computed<PublicCaptchaConfig | null>(() =>
    settings.value?.draft
      ? {
          enabled: true,
          provider: settings.value.draft.provider,
          site_key: settings.value.draft.site_key,
          version: settings.value.draft.version,
        }
      : null,
  );

  function resetAcknowledgements(): void {
    legacyAcknowledged.value = false;
    v1Acknowledged.value = false;
  }
  function syncForm(): void {
    siteKey.value = candidate.value?.site_key ?? "";
    hostnameInput.value = candidate.value?.hostnames.join("\n") ?? "";
    secret.value = "";
    clearSecret.value = false;
  }
  async function reload(preserveEdits = true): Promise<void> {
    if (pending.value) return;
    controller?.abort();
    controller = new AbortController();
    const request = controller;
    const keepEdits = preserveEdits && settings.value !== null;
    loading.value = true;
    serverError.value = null;
    notice.value = null;
    resetAcknowledgements();
    try {
      const next = await getCaptchaSettings(request.signal);
      if (disposed || controller !== request) return;
      settings.value = next;
      stale.value = false;
      if (!keepEdits) syncForm();
      // A concurrently enabled policy makes secret clearing invalid.
      if (next.enabled) clearSecret.value = false;
    } catch (error) {
      if (!disposed && controller === request) {
        serverError.value = error;
        stale.value = true;
      }
    } finally {
      if (!disposed && controller === request) loading.value = false;
    }
  }
  async function mutate(
    operation: (signal: AbortSignal) => Promise<CaptchaAdminView>,
  ): Promise<boolean> {
    if (busy.value || stale.value || !settings.value) return false;
    pending.value = true;
    serverError.value = null;
    notice.value = null;
    controller = new AbortController();
    const request = controller;
    try {
      const next = await operation(request.signal);
      if (disposed || controller !== request) return false;
      settings.value = next;
      return true;
    } catch (error) {
      if (!disposed && controller === request) {
        serverError.value = error;
        if (error instanceof ApiError && [30011, 30012].includes(error.code)) {
          stale.value = true;
          resetAcknowledgements();
        }
      }
      return false;
    } finally {
      if (!disposed && controller === request) pending.value = false;
    }
  }
  async function save(): Promise<void> {
    if (!canSave.value || !settings.value) return;
    const payload = {
      expected_version: settings.value.version,
      provider: "turnstile" as const,
      site_key: siteKey.value.trim(),
      hostnames: hostnames.value,
      ...(clearSecret.value
        ? { clear_secret: true }
        : secret.value
          ? { secret: secret.value }
          : {}),
    };
    if (await mutate((signal) => saveCaptchaDraft(payload, signal))) {
      syncForm();
      resetAcknowledgements();
      notice.value = "saved";
    }
  }
  async function verify(action: CaptchaAction, token: string): Promise<void> {
    if (!canTest.value || !settings.value || !token || token.length > 2048) return;
    const payload = { expected_version: settings.value.version, captcha_token: token, action };
    await mutate((signal) => testCaptchaDraft(payload, signal));
  }
  async function activate(): Promise<void> {
    if (!canActivate.value || !settings.value) return;
    const payload = {
      expected_version: settings.value.version,
      enabled: true,
      acknowledge_legacy_incompatibility: true,
      acknowledge_v1_unprotected: true,
    };
    if (await mutate((signal) => setCaptchaActivation(payload, signal))) {
      resetAcknowledgements();
      notice.value = "activated";
    }
  }
  async function disable(): Promise<void> {
    if (!settings.value?.enabled) return;
    const payload = {
      expected_version: settings.value.version,
      enabled: false,
      acknowledge_legacy_incompatibility: false,
      acknowledge_v1_unprotected: false,
    };
    if (await mutate((signal) => setCaptchaActivation(payload, signal))) {
      resetAcknowledgements();
      notice.value = "disabled";
    }
  }
  watch(dirty, (changed) => {
    if (changed) {
      resetAcknowledgements();
      notice.value = null;
    }
  });
  onMounted(() => {
    void reload(false);
  });
  onBeforeUnmount(() => {
    disposed = true;
    controller?.abort();
    secret.value = "";
    settings.value = null;
  });
  return {
    settings,
    siteKey,
    hostnameInput,
    secret,
    clearSecret,
    legacyAcknowledged,
    v1Acknowledged,
    loading,
    pending,
    busy,
    stale,
    errorMessage,
    notice,
    dirty,
    canSave,
    canTest,
    canActivate,
    draftConfig,
    reload,
    save,
    verify,
    activate,
    disable,
  };
}
