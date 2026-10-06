<script setup lang="ts">
import { useI18n } from "@vben/locales";
import {
  NAlert,
  NButton,
  NDropdown,
  NEmpty,
  NImageGroup,
  NPagination,
  NPopconfirm,
  NSelect,
  NSpin,
  useDialog,
  useMessage,
  type DropdownOption,
} from "naive-ui";
import { computed, nextTick, ref } from "vue";
import ImageCard from "./ImageCard.vue";
import ImageDetailDrawer from "./ImageDetailDrawer.vue";
import { useImageLibrary } from "./useImageLibrary";
import UnifiedImageSearch from "./UnifiedImageSearch.vue";
import { buildLinkText, resolveImageLink } from "@/components/upload/linkText";
import type { ImageView } from "@/api/images";
import { copyText } from "@/lib/clipboard";
import { formatApiError } from "@/locales/errors";

/**
 * 「我的图片」与相册详情共用的图片库：筛选/搜索工具栏、多选批量、
 * 卡片网格、灯箱大图与自定义右键菜单。lockedAlbumId 锁定相册时隐藏相册筛选。
 */
const props = defineProps<{ lockedAlbumId?: number | string; lockedAlbumName?: string }>();

const { t } = useI18n();
const message = useMessage();
const dialog = useDialog();
const lib = useImageLibrary({ lockedAlbumId: props.lockedAlbumId });

/* ---------------- 灯箱大图（NImageGroup 分组预览：左右箭头 + 键盘 ←/→） ---------------- */

/* ---------------- 自定义右键菜单 ---------------- */
const menuShow = ref(false);
const menuX = ref(0);
const menuY = ref(0);
const menuImage = ref<ImageView | null>(null);
async function openMenu(image: ImageView, event: MouseEvent): Promise<void> {
  menuImage.value = image;
  // 先收起再展开，保证重复右键时菜单位置刷新。
  menuShow.value = false;
  await nextTick();
  menuX.value = event.clientX;
  menuY.value = event.clientY;
  menuShow.value = true;
}

const menuFormats: { value: "url" | "markdown" | "html" | "bbcode"; label: string }[] = [
  { value: "url", label: "URL" },
  { value: "markdown", label: "Markdown" },
  { value: "html", label: "HTML" },
  { value: "bbcode", label: "BBCode" },
];

const menuVersions: {
  value: "original" | "webp" | "thumbnail";
  label: string;
  linkKey: "original" | "webp" | "thumbnail_url";
}[] = [
  { value: "original", label: t("user.upload.original"), linkKey: "original" },
  { value: "webp", label: t("user.upload.webp"), linkKey: "webp" },
  { value: "thumbnail", label: t("user.upload.thumbnail"), linkKey: "thumbnail_url" },
];

/** 复制链接二级菜单：原图/WebP/缩略图 → 复制 URL/Markdown/HTML/BBCode。 */
const menuOptions = computed<DropdownOption[]>(() => {
  const image = menuImage.value;
  const copyGroups: DropdownOption[] = menuVersions.map((version) => ({
    label: version.label,
    key: `copy-${version.value}`,
    disabled: !image || image.links[version.linkKey] === "",
    children: menuFormats.map((format) => ({
      label: t("user.images.menuCopyFormat", { format: format.label }),
      key: `copy-${version.value}:${format.value}`,
    })),
  }));
  return [
    ...copyGroups,
    { type: "divider", key: "divider" },
    { label: t("user.images.menuProps"), key: "props" },
    {
      label: t(
        menuImage.value?.is_public ? "user.images.menuMakePrivate" : "user.images.menuMakePublic",
      ),
      key: "toggle",
    },
    {
      label: t("user.common.remove"),
      key: "remove",
      props: { style: "color: hsl(var(--destructive))" },
    },
  ];
});

async function copyMenuText(text: string): Promise<void> {
  const ok = await copyText(text);
  if (ok) message.success(() => t("user.upload.copied"));
  else message.error(() => t("user.upload.copyError"));
}

function onMenuSelect(key: string | number): void {
  const image = menuImage.value;
  menuShow.value = false;
  if (!image) return;
  const name = image.name || image.key;
  // 二级复制项：copy-<version>:<format>
  const combo = /^copy-(original|webp|thumbnail):(url|markdown|html|bbcode)$/.exec(String(key));
  if (combo) {
    const version = combo[1] as "original" | "webp" | "thumbnail";
    const format = combo[2] as "url" | "markdown" | "html" | "bbcode";
    void copyMenuText(buildLinkText(format, name, resolveImageLink(image, version)));
    return;
  }
  switch (key) {
    case "props":
      lib.openDrawer(image);
      break;
    case "toggle":
      void lib.handleToggle(image, !image.is_public);
      break;
    case "remove":
      dialog.warning({
        title: t("user.images.deleteConfirmTitle", { name }),
        content: t("user.images.deleteConfirm"),
        positiveText: t("user.common.confirmDelete"),
        negativeText: t("user.common.cancel"),
        onPositiveClick: () => {
          void lib.handleRemove(image);
        },
      });
      break;
  }
}

const emptyText = computed(() =>
  lib.hasActiveFilters.value
    ? t("user.images.filteredEmpty")
    : t(props.lockedAlbumId !== undefined ? "user.images.albumEmpty" : "user.images.empty"),
);
</script>

<template>
  <div class="image-library">
    <div class="image-library__toolbar">
      <span class="image-library__total">{{ t("search.total", { count: lib.total.value }) }}</span>
      <div class="image-library__filters">
        <NSelect
          class="image-library__size"
          size="small"
          :value="lib.size.value"
          :options="lib.sizeOptions.value"
          :aria-label="t('user.images.pageSize')"
          @update:value="lib.handleSizeChange"
        />
        <NButton
          size="small"
          :loading="lib.loading.value"
          :disabled="lib.loading.value || lib.batchLoading.value"
          @click="lib.load"
          >{{ t("user.common.refresh") }}</NButton
        >
      </div>
    </div>
    <UnifiedImageSearch
      v-model="lib.draftRaw.value"
      :timezone="lib.timezone.value"
      :authorized-albums="lib.metadata.value?.authorizedAlbums ?? []"
      :locked-album-id="lockedAlbumId"
      :locked-album-name="lockedAlbumName"
      :diagnostics="lib.diagnostics.value"
      :submitted="lib.submitted.value"
      :unapplied="lib.unapplied.value"
      :stale-results="lib.staleResults.value"
      :loading="lib.loading.value"
      @submit="lib.submit"
      @clear="lib.resetFilters"
    />

    <div v-if="lib.selectedIds.value.length > 0" class="image-library__batchbar">
      <span class="image-library__batchbar-count">{{
        t("user.images.selected", { count: lib.selectedIds.value.length })
      }}</span>
      <NSelect
        v-model:value="lib.batchTarget.value"
        class="image-library__batchbar-album"
        size="small"
        :options="lib.batchTargetOptions.value"
        :placeholder="t('user.images.targetAlbum')"
        :disabled="lib.batchLoading.value"
      />
      <NButton
        size="small"
        type="primary"
        :loading="lib.batchLoading.value"
        @click="lib.moveSelected"
      >
        {{ t("user.images.moveAlbum") }}
      </NButton>
      <NPopconfirm
        :positive-text="t('user.common.confirmDelete')"
        :negative-text="t('user.common.cancel')"
        @positive-click="lib.removeSelected"
      >
        <template #trigger>
          <NButton size="small" type="error" :loading="lib.batchLoading.value">{{
            t("user.images.batchDelete")
          }}</NButton>
        </template>
        {{ t("user.images.batchDeleteConfirm", { count: lib.selectedIds.value.length }) }}
      </NPopconfirm>
      <NButton size="small" :disabled="lib.batchLoading.value" @click="lib.batchVisibility(true)">
        {{ t("user.images.makePublic") }}
      </NButton>
      <NButton size="small" :disabled="lib.batchLoading.value" @click="lib.batchVisibility(false)">
        {{ t("user.images.makePrivate") }}
      </NButton>
      <NButton
        size="small"
        quaternary
        :disabled="lib.batchLoading.value"
        @click="lib.clearSelection"
      >
        {{ t("user.common.cancel") }}
      </NButton>
    </div>

    <NAlert v-if="lib.loadError.value" type="error" class="image-library__error">
      {{ formatApiError(lib.loadError.value, "user.images.listError") }}
      <NButton text :disabled="lib.loading.value" @click="lib.retry">{{
        t("user.common.retry")
      }}</NButton>
    </NAlert>
    <NSpin :show="lib.loading.value">
      <NEmpty
        v-if="
          !lib.loading.value &&
          !lib.loadError.value &&
          lib.diagnostics.value.length === 0 &&
          lib.images.value.length === 0
        "
        class="image-library__empty"
        :description="emptyText"
      />
      <NImageGroup v-else>
        <div class="image-library__grid">
          <ImageCard
            v-for="image in lib.images.value"
            :key="image.id"
            :image="image"
            :busy="
              lib.loading.value ||
              !!lib.loadError.value ||
              lib.staleResults.value ||
              lib.batchLoading.value ||
              lib.busyIds.has(image.id)
            "
            selectable
            :selected="lib.selectedIds.value.includes(image.id)"
            :album-options="lib.cardMoveOptions.value"
            @menu="(event) => openMenu(image, event)"
            @open="lib.openDrawer(image)"
            @toggle="(isPublic) => lib.handleToggle(image, isPublic)"
            @remove="lib.handleRemove(image)"
            @select="(checked) => lib.handleSelect(image, checked)"
            @move="(albumId) => lib.handleMove(image, albumId)"
          />
        </div>
      </NImageGroup>
    </NSpin>
    <div class="image-library__pagination">
      <NPagination
        :disabled="lib.loading.value || !!lib.loadError.value || lib.diagnostics.value.length > 0"
        :page-slot="5"
        :page="lib.page.value"
        :item-count="lib.total.value"
        :page-size="lib.size.value"
        @update:page="lib.handlePageChange"
      />
    </div>

    <NDropdown
      trigger="manual"
      placement="bottom-start"
      :show="menuShow"
      :x="menuX"
      :y="menuY"
      :options="menuOptions"
      @select="onMenuSelect"
      @clickoutside="menuShow = false"
    />
    <ImageDetailDrawer v-model:show="lib.drawerShow.value" :image="lib.drawerImage.value" />
  </div>
</template>

<style scoped>
.image-library {
  min-width: 0;
}

.image-library__toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 8px;
}

.image-library__filters {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.image-library__total {
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}

.image-library__size {
  width: 120px;
}

.image-library__batchbar {
  position: sticky;
  top: 0;
  z-index: 5;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
  padding: 8px 12px;
  border: 1px solid hsl(var(--border));
  border-radius: 6px;
  background: hsl(var(--muted) / 0.5);
}

.image-library__batchbar-count {
  font-size: 13px;
  font-weight: 600;
}

.image-library__batchbar-album {
  width: 180px;
}

.image-library__empty {
  padding: 48px 0;
}

.image-library__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(220px, 100%), 1fr));
  gap: 14px;
}

.image-library__pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
  max-width: 100%;
}
.image-library__pagination :deep(.n-pagination) {
  flex-wrap: wrap;
  justify-content: flex-end;
}
</style>
