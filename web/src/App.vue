<script setup lang="ts">
import {
  darkTheme,
  dateZhCN,
  NConfigProvider,
  NDialogProvider,
  NGlobalStyle,
  NMessageProvider,
  useOsTheme,
  zhCN,
} from "naive-ui";
import { computed } from "vue";
import { useRoute } from "vue-router";
import AppLayout from "@/components/layout/AppLayout.vue";

const osTheme = useOsTheme();
const theme = computed(() => (osTheme.value === "dark" ? darkTheme : null));
const route = useRoute();
const isBare = computed(() => route.meta.bare === true);
</script>

<template>
  <NConfigProvider :theme="theme" :locale="zhCN" :date-locale="dateZhCN">
    <NMessageProvider>
      <NDialogProvider>
        <NGlobalStyle />
        <RouterView v-if="isBare" />
        <AppLayout v-else />
      </NDialogProvider>
    </NMessageProvider>
  </NConfigProvider>
</template>
