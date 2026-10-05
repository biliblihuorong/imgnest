<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { computed } from "vue";
import { useRoute } from "vue-router";
import { useNaiveDesignTokens } from "@vben/hooks";
import { usePreferences } from "@vben/preferences";
import {
  darkTheme,
  dateZhCN,
  dateEnUS,
  enUS,
  NConfigProvider,
  NDialogProvider,
  NGlobalStyle,
  NMessageProvider,
  NNotificationProvider,
  zhCN,
} from "naive-ui";
import AppLayout from "@/components/layout/AppLayout.vue";

const { locale } = useI18n();
const naiveLocale = computed(() => (locale.value === "en-US" ? enUS : zhCN));
const naiveDateLocale = computed(() => (locale.value === "en-US" ? dateEnUS : dateZhCN));
const { isDark } = usePreferences();
const { commonTokens } = useNaiveDesignTokens();
const theme = computed(() => (isDark.value ? darkTheme : null));
const themeOverrides = computed(() => ({ common: commonTokens }));
const route = useRoute();
const isBare = computed(() => route.meta.bare === true);
</script>

<template>
  <NConfigProvider
    :theme="theme"
    :theme-overrides="themeOverrides"
    :locale="naiveLocale"
    :date-locale="naiveDateLocale"
    class="h-full"
  >
    <NNotificationProvider>
      <NMessageProvider>
        <NDialogProvider>
          <NGlobalStyle />
          <RouterView v-if="isBare" />
          <AppLayout v-else />
        </NDialogProvider>
      </NMessageProvider>
    </NNotificationProvider>
  </NConfigProvider>
</template>
