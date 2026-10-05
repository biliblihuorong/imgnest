<script setup lang="ts">
import { NImage } from "naive-ui";
import { useProtectedThumbnail } from "@/components/images/useProtectedThumbnail";
const props = defineProps<{ url?: string; name: string; extension: string }>();
const { thumbnailUrl } = useProtectedThumbnail(() => props.url);
</script>
<template>
  <NImage
    v-if="thumbnailUrl"
    :src="thumbnailUrl"
    object-fit="cover"
    preview-disabled
    :img-props="{ alt: name }"
    class="admin-thumbnail"
  />
  <span v-else class="admin-thumbnail admin-thumbnail-fallback">{{ extension }}</span>
</template>
<style scoped>
.admin-thumbnail {
  display: inline-flex;
  width: 56px;
  height: 56px;
  border-radius: 4px;
}
.admin-thumbnail-fallback {
  align-items: center;
  justify-content: center;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
  background: hsl(var(--muted));
}
</style>
