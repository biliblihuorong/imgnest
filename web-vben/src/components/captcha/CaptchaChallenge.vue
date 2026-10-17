<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { usePreferences } from "@vben/preferences";
import { NButton } from "naive-ui";
import { computed, onBeforeUnmount, shallowRef, useTemplateRef, watch } from "vue";
import type { CaptchaAction, PublicCaptchaConfig } from "@/api/captcha";
import { loadTurnstile, type TurnstileApi } from "./turnstile";

const props = withDefaults(
  defineProps<{
    config: PublicCaptchaConfig;
    action: CaptchaAction;
    resetKey?: number;
    size?: "flexible" | "compact";
  }>(),
  { resetKey: 0, size: "flexible" },
);
const emit = defineEmits<{ token: [value: string | null] }>();
const { t, locale } = useI18n();
const container = useTemplateRef<HTMLElement>("container");
const state = shallowRef<"loading" | "waiting" | "verified" | "expired" | "unavailable">("loading");
const language = computed(() => (locale.value === "en-US" ? "en" : "zh-CN"));
const { isDark } = usePreferences();
const theme = computed(() => (isDark.value ? "dark" : "light"));
let api: TurnstileApi | undefined;
let widgetId: string | undefined;
let generation = 0;
let disposed = false;
let acceptingToken = false;
// In-memory only; a provider callback must never reintroduce a consumed/expired response.
const seenTokens = new Set<string>();
function clearToken(): void {
  emit("token", null);
}
function remove(): void {
  generation++;
  acceptingToken = false;
  clearToken();
  if (api && widgetId !== undefined) {
    try {
      api.remove(widgetId);
    } catch {
      /* A failed provider cleanup cannot authorize submission. */
    }
  }
  widgetId = undefined;
}
async function render(): Promise<void> {
  remove();
  const current = generation;
  if (!props.config.enabled) return;
  if (props.config.provider !== "turnstile" || !props.config.site_key.trim()) {
    state.value = "unavailable";
    return;
  }
  state.value = "loading";
  try {
    const provider = await loadTurnstile();
    if (disposed || generation !== current || !container.value) return;
    api = provider;
    state.value = "waiting";
    acceptingToken = true;
    const invalidate = (next: "expired" | "unavailable") => {
      if (disposed || generation !== current) return;
      acceptingToken = false;
      clearToken();
      state.value = next;
    };
    widgetId = provider.render(container.value, {
      sitekey: props.config.site_key,
      action: props.action,
      language: language.value,
      size: props.size,
      theme: theme.value,
      "response-field": false,
      retry: "never",
      "refresh-expired": "manual",
      callback: (token) => {
        if (disposed || generation !== current || !acceptingToken) return;
        if (
          typeof token !== "string" ||
          !token.trim() ||
          token.length > 2048 ||
          seenTokens.has(token)
        ) {
          invalidate("expired");
          return;
        }
        seenTokens.add(token);
        acceptingToken = false;
        state.value = "verified";
        emit("token", token);
      },
      "expired-callback": () => invalidate("expired"),
      "error-callback": () => invalidate("unavailable"),
      "timeout-callback": () => invalidate("expired"),
    });
  } catch {
    if (disposed || generation !== current) return;
    acceptingToken = false;
    clearToken();
    state.value = "unavailable";
  }
}
function reset(): void {
  acceptingToken = false;
  clearToken();
  if (!api || widgetId === undefined) {
    void render();
    return;
  }
  state.value = "waiting";
  acceptingToken = true;
  try {
    api.reset(widgetId);
  } catch {
    acceptingToken = false;
    state.value = "unavailable";
  }
}
// theme 只在渲染时读取：Turnstile 不能原地换主题，重建会作废已通过或进行中的验证。
watch(
  () => [
    container.value,
    props.config.enabled,
    props.config.provider,
    props.config.site_key,
    props.config.version,
    props.action,
    props.size,
    language.value,
  ],
  () => {
    void render();
  },
  { flush: "post" },
);
watch(() => props.resetKey, reset, { flush: "sync" });
onBeforeUnmount(() => {
  disposed = true;
  remove();
  seenTokens.clear();
});
</script>

<template>
  <section class="captcha-challenge" :aria-label="t('captcha.challenge.label')">
    <div ref="container" class="captcha-container" />
    <p
      v-if="state === 'loading' || state === 'waiting' || state === 'verified'"
      class="captcha-status"
      :class="`is-${state}`"
      role="status"
      aria-live="polite"
    >
      <span class="captcha-status__dot" aria-hidden="true" />{{ t(`captcha.challenge.${state}`) }}
    </p>
    <div v-else class="captcha-problem" role="alert">
      <span>{{ t(`captcha.challenge.${state}`) }}</span>
      <NButton size="tiny" secondary attr-type="button" @click="reset">{{
        t("captcha.retry")
      }}</NButton>
    </div>
    <p class="captcha-privacy">
      {{ t("captcha.challenge.privacy") }}
      <a
        href="https://www.cloudflare.com/turnstile-privacy-policy/"
        target="_blank"
        rel="noopener noreferrer"
        >{{ t("captcha.challenge.privacyLink") }}</a
      >
    </p>
  </section>
</template>

<style scoped>
.captcha-challenge {
  display: grid;
  gap: 6px;
  min-width: 0;
  margin: 0 0 18px;
}
/* Turnstile 自带边框；外面只裁圆角，与输入框保持一致。 */
.captcha-container {
  width: 100%;
  min-width: 0;
  overflow: hidden;
  line-height: 0;
  border-radius: 8px;
}
.captcha-container:empty {
  display: none;
}
.captcha-status,
.captcha-privacy {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: hsl(var(--muted-foreground));
  overflow-wrap: anywhere;
}
.captcha-status {
  display: flex;
  gap: 6px;
  align-items: center;
}
.captcha-status__dot {
  flex-shrink: 0;
  width: 6px;
  height: 6px;
  background: hsl(var(--muted-foreground) / 0.6);
  border-radius: 50%;
}
.captcha-status.is-verified {
  color: hsl(var(--success));
}
.captcha-status.is-verified .captcha-status__dot {
  background: hsl(var(--success));
}
.captcha-problem {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  font-size: 12px;
  line-height: 1.5;
  color: hsl(var(--destructive));
  overflow-wrap: anywhere;
  background: hsl(var(--destructive) / 0.08);
  border-radius: 8px;
}
.captcha-privacy {
  font-size: 11px;
  opacity: 0.85;
}
.captcha-privacy a {
  color: hsl(var(--primary));
  white-space: nowrap;
}
.captcha-privacy a:hover {
  text-decoration: underline;
}
</style>
