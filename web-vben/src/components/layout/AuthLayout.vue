<script setup lang="ts">
import { AuthPageLayout } from "@vben/layouts";
import logo from "@/assets/imgnest-mark.svg";
import { useSiteStore } from "@/stores/site";

const site = useSiteStore();
void site.ensureLoaded();
</script>

<template>
  <AuthPageLayout
    :app-name="site.siteName"
    :logo="logo"
    :logo-dark="logo"
    :toolbar-list="['language', 'theme']"
    :copyright="false"
  />
</template>
