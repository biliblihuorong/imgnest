<script setup lang="ts">
import { ChevronDown, Plus } from "@vben/icons";
import { useI18n } from "@vben/locales";
import { shallowRef, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import type { AlbumView } from "@/api/albums";
import AlbumFormModal from "@/components/albums/AlbumFormModal.vue";
import { useSidebarAlbums } from "./useSidebarAlbums";

/** 侧栏「相册」项下方的分组：相册列表 + 新建 + 全部。编辑/删除仍在相册页。 */
const emit = defineEmits<{ navigate: [] }>();
const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const { albums, total, loading, failed, load } = useSidebarAlbums();
const expanded = shallowRef(true);
const formShow = shallowRef(false);

// 相册的编辑、删除、设封面都在 /albums 页完成；离开该页时刷新一次侧栏列表。
watch(
  () => route.path,
  (_next, previous) => {
    if (previous === "/albums") void load();
  },
);

async function onSaved(album: AlbumView): Promise<void> {
  emit("navigate");
  void load();
  await router.push(`/albums/${album.id}`);
}
</script>

<template>
  <div class="mv-albums">
    <button
      type="button"
      class="mv-albums__toggle"
      data-testid="album-toggle"
      :aria-expanded="expanded"
      :aria-label="t(expanded ? 'shell.albums.collapse' : 'shell.albums.expand')"
      @click="expanded = !expanded"
    >
      <ChevronDown class="mv-albums__chev" :class="{ 'is-open': expanded }" />
    </button>
    <div v-if="expanded" class="mv-albums__list">
      <RouterLink
        v-for="album in albums"
        :key="album.id"
        :to="`/albums/${album.id}`"
        class="mv-albums__item"
        data-testid="album-link"
        :title="album.name"
        @click="emit('navigate')"
        >{{ album.name }}</RouterLink
      >
      <div v-if="failed" class="mv-albums__note">
        {{ t("shell.albums.loadFailed") }}
        <button type="button" data-testid="album-retry" :disabled="loading" @click="load">
          {{ t("common.retry") }}
        </button>
      </div>
      <button
        type="button"
        class="mv-albums__item mv-albums__create"
        data-testid="album-create"
        @click="formShow = true"
      >
        <Plus class="mv-albums__plus" />{{ t("shell.albums.create") }}
      </button>
      <RouterLink
        to="/albums"
        class="mv-albums__item mv-albums__all"
        data-testid="album-all"
        @click="emit('navigate')"
      >
        {{ t("shell.albums.all") }}
        <span v-if="total > albums.length" class="mv-albums__count">{{ total }}</span>
      </RouterLink>
    </div>
    <AlbumFormModal v-model:show="formShow" :album="null" @saved="onSaved" />
  </div>
</template>

<style scoped>
.mv-albums {
  position: relative;
}

/* 折叠按钮叠在上一行「相册」导航项的右端。 */
.mv-albums__toggle {
  position: absolute;
  top: -33px;
  right: 6px;
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  color: hsl(var(--muted-foreground));
  border-radius: 8px;
}
.mv-albums__toggle:hover {
  background: hsl(var(--background));
}
.mv-albums__chev {
  width: 14px;
  height: 14px;
  transition: transform 0.25s var(--mv-ease);
}
.mv-albums__chev.is-open {
  transform: rotate(180deg);
}
.mv-albums__list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.mv-albums__item {
  display: flex;
  gap: 6px;
  align-items: center;
  padding: 7px 10px 7px 36px;
  overflow: hidden;
  font-size: 13px;
  color: hsl(var(--muted-foreground));
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
  border-radius: 10px;
}
.mv-albums__item:hover {
  background: hsl(var(--muted) / 0.7);
}
.mv-albums__item.router-link-exact-active:not(.mv-albums__all) {
  font-weight: 600;
  color: hsl(var(--foreground));
  background: hsl(var(--muted));
}
.mv-albums__plus {
  width: 13px;
  height: 13px;
}
.mv-albums__count {
  margin-left: auto;
  font-size: 11.5px;
  font-variant-numeric: tabular-nums;
}
.mv-albums__note {
  padding: 6px 10px 6px 36px;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}
.mv-albums__note button {
  color: hsl(var(--primary));
}
</style>
