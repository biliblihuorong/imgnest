<script setup lang="ts">
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import AdminBilling from "@/components/billing/AdminBilling.vue";
const { t } = useI18n();
</script>
<template>
  <Page
    ><template #title
      ><h1 class="text-xl font-semibold">{{ t("billing.adminTitle") }}</h1></template
    ><AdminBilling
  /></Page>
</template>
