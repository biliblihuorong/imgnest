<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { useMessage } from "naive-ui";
import { defineAsyncComponent, type Component } from "vue";
import { setShell, shell, type Shell } from "@/integrations/shell/useShell";

const { t } = useI18n();
const message = useMessage();
const failed = new Set<Shell>();

/** 布局 chunk 加载失败时回退到另一套，只回退一次，避免两套都失败时来回切换。 */
function layout(name: Shell, loader: () => Promise<{ default: Component }>) {
  return defineAsyncComponent({
    loader,
    onError(error, _retry, fail) {
      failed.add(name);
      const other: Shell = name === "marvis" ? "classic" : "marvis";
      if (!failed.has(other)) {
        setShell(other);
        message.warning(() => t("shell.layoutLoadFailed"));
      }
      fail();
      void error;
    },
  });
}

const MarvisLayout = layout("marvis", () => import("./MarvisLayout.vue"));
const ClassicLayout = layout("classic", () => import("./ClassicLayout.vue"));
</script>

<template>
  <MarvisLayout v-if="shell === 'marvis'" />
  <ClassicLayout v-else />
</template>
