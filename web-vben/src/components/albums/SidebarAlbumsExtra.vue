<script setup lang="ts">
import type { MenuItem } from "@/app.config";
import SidebarAlbums from "./SidebarAlbums.vue";

/** Marvis 侧栏「相册」菜单下的可展开相册列表（经 app.hooks.ts 的 shellExtensions 挂进外壳）。 */
defineProps<{ item: MenuItem }>();
const emit = defineEmits<{ navigate: [] }>();
</script>

<template>
  <SidebarAlbums v-if="item.path === '/albums'" @navigate="emit('navigate')" />
</template>
