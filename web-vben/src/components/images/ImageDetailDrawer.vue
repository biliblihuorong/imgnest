<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NDrawer, NDrawerContent } from "naive-ui";
import type { ImageView } from "@/api/images";
import ImageDetailContent from "./ImageDetailContent.vue";

const { t } = useI18n();

const props = defineProps<{
  show: boolean;
  image: ImageView | null;
}>();

const emit = defineEmits<{
  "update:show": [value: boolean];
}>();

function handleClose(value: boolean): void {
  emit("update:show", value);
}
</script>

<template>
  <NDrawer :show="props.show" width="min(520px, 100vw)" @update:show="handleClose">
    <NDrawerContent :title="props.image?.name ?? t('user.detail.title')" closable>
      <ImageDetailContent :image="props.image" :active="props.show" />
    </NDrawerContent>
  </NDrawer>
</template>
