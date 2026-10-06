<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { useMessage } from "naive-ui";
import {
  computed,
  defineAsyncComponent,
  reactive,
  shallowReactive,
  watch,
  type Component,
} from "vue";
import { shell, type Shell } from "@/integrations/shell/useShell";

const { t } = useI18n();
const message = useMessage();

const loaders: Record<Shell, () => Promise<{ default: Component }>> = {
  marvis: () => import("./MarvisLayout.vue"),
  classic: () => import("./ClassicLayout.vue"),
};
const other = (name: Shell): Shell => (name === "marvis" ? "classic" : "marvis");

/** 本次会话里加载失败的布局。只影响当前渲染，不改用户保存的选择。 */
const failed = reactive(new Set<Shell>());

function create(name: Shell): Component {
  return defineAsyncComponent({
    loader: loaders[name],
    onError(_error, _retry, fail) {
      failed.add(name);
      if (!failed.has(other(name))) message.warning(() => t("shell.layoutLoadFailed"));
      fail();
    },
  });
}
const layouts = shallowReactive<Record<Shell, Component>>({
  marvis: create("marvis"),
  classic: create("classic"),
});

// 用户再次明确选择失败过的布局时重新加载；异步组件会缓存失败结果，必须重建。
watch(shell, (next) => {
  if (!failed.has(next)) return;
  failed.delete(next);
  layouts[next] = create(next);
});

const rendered = computed<Shell | null>(() => {
  if (!failed.has(shell.value)) return shell.value;
  return failed.has(other(shell.value)) ? null : other(shell.value);
});

function reload(): void {
  window.location.reload();
}
</script>

<template>
  <component :is="layouts[rendered]" v-if="rendered" />
  <div v-else class="app-layout-failed" role="alert" data-testid="layout-failed">
    <p>{{ t("shell.layoutBothFailed") }}</p>
    <button type="button" @click="reload">{{ t("shell.reload") }}</button>
  </div>
</template>

<style scoped>
.app-layout-failed {
  display: grid;
  gap: 12px;
  place-content: center;
  height: 100%;
  padding: 24px;
  text-align: center;
}
.app-layout-failed button {
  justify-self: center;
  padding: 6px 16px;
  color: hsl(var(--primary));
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
}
</style>
