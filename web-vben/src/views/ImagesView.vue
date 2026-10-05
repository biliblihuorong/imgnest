<script setup lang="ts">
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import { NCard, NTabPane, NTabs } from "naive-ui";
import TrashView from "./TrashView.vue";
import ImageLibrary from "@/components/images/ImageLibrary.vue";
const { t } = useI18n();
</script>

<template>
  <Page
    class="images-view"
    :description="t('user.images.description')"
    header-class="flex-wrap gap-4"
  >
    <template #title
      ><h1 class="images-view__title">{{ t("user.images.title") }}</h1></template
    >
    <NCard :bordered="false">
      <NTabs default-value="images" type="line">
        <NTabPane name="images" :tab="t('user.images.tab')">
          <ImageLibrary />
        </NTabPane>
        <NTabPane name="trash" :tab="t('user.trash.title')" display-directive="show:lazy">
          <TrashView />
        </NTabPane>
      </NTabs>
    </NCard>
  </Page>
</template>

<style scoped>
.images-view__title {
  margin: 0 0 8px;
  font-size: 18px;
  font-weight: 600;
}
</style>
