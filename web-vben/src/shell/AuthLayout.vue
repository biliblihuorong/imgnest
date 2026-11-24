<script setup lang="ts">
import { AuthPageLayout } from "@vben/layouts";
import { site } from "./site";
</script>

<template>
  <AuthPageLayout
    :app-name="site.name"
    :logo="site.logo"
    :logo-dark="site.logo"
    :toolbar-list="['language', 'theme']"
    :copyright="false"
  />
</template>
