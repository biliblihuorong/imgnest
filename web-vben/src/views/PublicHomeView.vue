<script setup lang="ts">
import { useI18n } from "@vben/locales";
import {
  NAlert,
  NButton,
  NCard,
  NEmpty,
  NImageGroup,
  NRadioButton,
  NRadioGroup,
  NSpin,
  NTag,
  useMessage,
} from "naive-ui";
import { computed, onBeforeUnmount, onMounted, ref, shallowRef } from "vue";
import { getToken } from "@/api/client";
import { listGallery, type GalleryItem } from "@/api/gallery";
import { getGuestUploadLimits, MAX_UPLOAD_FILES, type ImageView } from "@/api/upload";
import logo from "@/assets/imgnest-mark.svg";
import GalleryCard from "@/components/gallery/GalleryCard.vue";
import ImageCopyDrawer from "@/components/images/ImageCopyDrawer.vue";
import { usePreviewSwipe } from "@/components/images/usePreviewSwipe";
import UploadDropzone from "@/components/upload/UploadDropzone.vue";
import UploadQueueList from "@/components/upload/UploadQueueList.vue";
import {
  buildLinkText,
  resolveImageLink,
  type LinkFormat,
  type LinkVersion,
} from "@/components/upload/linkText";
import { useUploadQueue } from "@/components/upload/useUploadQueue";
import { copyText } from "@/lib/clipboard";
import { formatBytes } from "@/lib/format";
import { landingPath } from "@/router/landing";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";

/**
 * 公开图床首页：站点开放游客上传时，访客不登录也能拖图上传并复制链接；
 * 开放画廊时下面展示最新的公开图片。已登录用户看到进入控制台的入口。
 */
const PREVIEW_SIZE = 12;

const { t } = useI18n();
const message = useMessage();
const site = useSiteStore();
const auth = useAuthStore();
const signedIn = computed(() => getToken() !== null);
const consolePath = computed(() => landingPath(auth.user?.role));
const renderPreviewToolbar = usePreviewSwipe();

const maxFileBytes = ref<number | null>(null);
const linkFormat = ref<LinkFormat>("url");
const linkVersion = ref<LinkVersion>("webp");
const drawerShow = ref(false);
const drawerImage = ref<ImageView | null>(null);
const preview = shallowRef<GalleryItem[]>([]);
const previewLoading = ref(false);
let active = true;
onBeforeUnmount(() => {
  active = false;
});

const canGuestUpload = computed(() => site.guestUpload && !signedIn.value);
const {
  items,
  uploading,
  canUpload,
  preflightErrors,
  clearPreflight,
  onFiles,
  startUpload,
  retryItem,
} = useUploadQueue(
  () => ({ policyId: null, albumId: null, isPublic: false, guest: true }),
  () => maxFileBytes.value,
);
const successItems = computed(() =>
  items.value.filter((item) => item.state === "success" && item.image),
);

function openDrawer(image: ImageView): void {
  drawerImage.value = image;
  drawerShow.value = true;
}

async function copyAllLinks(): Promise<void> {
  const targets = successItems.value;
  if (!targets.length) return;
  const text = targets
    .map((item) =>
      buildLinkText(
        linkFormat.value,
        item.image!.name || item.image!.key,
        resolveImageLink(item.image!, linkVersion.value),
      ),
    )
    .join("\n");
  const ok = await copyText(text);
  if (!active) return;
  if (ok) message.success(t("user.upload.copiedAll", { count: targets.length }));
  else message.error(t("user.upload.copyError"));
}

async function loadLimits(): Promise<void> {
  try {
    const limits = await getGuestUploadLimits();
    if (active) maxFileBytes.value = limits.max_file_bytes;
  } catch {
    // 读不到就不在客户端预检单文件大小，服务端仍按游客组上限拒绝。
  }
}

async function loadPreview(): Promise<void> {
  previewLoading.value = true;
  try {
    const page = await listGallery({ page: 1, size: PREVIEW_SIZE });
    if (active) preview.value = page.items;
  } catch {
    if (active) preview.value = [];
  } finally {
    if (active) previewLoading.value = false;
  }
}

onMounted(async () => {
  await site.ensureLoaded();
  if (!active) return;
  if (canGuestUpload.value) void loadLimits();
  if (site.galleryEnabled) void loadPreview();
});
</script>

<template>
  <div class="public-home">
    <header class="public-home__header">
      <RouterLink to="/home" class="public-home__brand">
        <img :src="logo" alt="" class="public-home__logo" />
        <span>{{ site.siteName }}</span>
      </RouterLink>
      <nav class="public-home__nav" :aria-label="t('home.nav')">
        <RouterLink v-if="site.galleryEnabled" to="/gallery">
          <NButton quaternary>{{ t("home.gallery") }}</NButton>
        </RouterLink>
        <RouterLink v-if="signedIn" :to="consolePath">
          <NButton type="primary">{{ t("home.console") }}</NButton>
        </RouterLink>
        <template v-else>
          <RouterLink v-if="site.registerEnabled" to="/register">
            <NButton quaternary>{{ t("common.register") }}</NButton>
          </RouterLink>
          <RouterLink to="/login">
            <NButton type="primary">{{ t("common.login") }}</NButton>
          </RouterLink>
        </template>
      </nav>
    </header>

    <main class="public-home__main">
      <section class="public-home__hero">
        <h1 class="public-home__title">{{ site.siteName }}</h1>
        <p class="public-home__subtitle">{{ t("home.subtitle") }}</p>
      </section>

      <NCard
        v-if="canGuestUpload"
        class="public-home__card"
        :bordered="false"
        data-test="guest-upload"
      >
        <NAlert
          v-if="preflightErrors.length > 0"
          type="error"
          closable
          class="public-home__alert"
          @close="clearPreflight"
        >
          <div v-for="error in preflightErrors" :key="error">{{ error }}</div>
        </NAlert>
        <UploadDropzone :disabled="uploading" @files="onFiles" />
        <div class="public-home__actions">
          <span class="public-home__limits">
            {{
              maxFileBytes === null
                ? t("user.upload.limitsCount", { count: MAX_UPLOAD_FILES })
                : t("user.upload.limits", {
                    count: MAX_UPLOAD_FILES,
                    fileSize: formatBytes(maxFileBytes),
                  })
            }}
          </span>
          <NButton type="primary" :loading="uploading" :disabled="!canUpload" @click="startUpload">
            {{ t("user.upload.start") }}
          </NButton>
        </div>
        <p class="public-home__note">{{ t("home.guestNote") }}</p>

        <template v-if="items.length > 0">
          <div class="public-home__queue-head">
            <NTag :bordered="false" size="small">
              {{ t("user.upload.files", { count: items.length }) }}
            </NTag>
            <NButton
              v-if="successItems.length > 0"
              size="small"
              type="primary"
              secondary
              @click="copyAllLinks"
            >
              {{ t("user.upload.copyAll") }}
            </NButton>
          </div>
          <div
            v-if="successItems.length > 0"
            class="public-home__link-controls"
            role="group"
            :aria-label="t('user.upload.linkFormat')"
          >
            <NRadioGroup v-model:value="linkVersion" size="small">
              <NRadioButton value="original">{{ t("user.upload.original") }}</NRadioButton>
              <NRadioButton value="webp">{{ t("user.upload.webp") }}</NRadioButton>
              <NRadioButton value="thumbnail">{{ t("user.upload.thumbnail") }}</NRadioButton>
            </NRadioGroup>
            <NRadioGroup v-model:value="linkFormat" size="small">
              <NRadioButton value="url">URL</NRadioButton>
              <NRadioButton value="markdown">Markdown</NRadioButton>
              <NRadioButton value="html">HTML</NRadioButton>
              <NRadioButton value="bbcode">BBCode</NRadioButton>
            </NRadioGroup>
          </div>
          <UploadQueueList
            v-model:format="linkFormat"
            v-model:version="linkVersion"
            :items="items"
            :busy="uploading"
            @retry="retryItem"
            @detail="openDrawer"
          />
        </template>
      </NCard>

      <NCard v-else class="public-home__card" :bordered="false" data-test="sign-in-to-upload">
        <div class="public-home__closed">
          <p>{{ signedIn ? t("home.signedIn") : t("home.guestClosed") }}</p>
          <RouterLink :to="signedIn ? consolePath : '/login'">
            <NButton type="primary">
              {{ signedIn ? t("home.console") : t("home.signInToUpload") }}
            </NButton>
          </RouterLink>
        </div>
      </NCard>

      <section v-if="site.galleryEnabled" class="public-home__gallery" data-test="gallery-preview">
        <div class="public-home__gallery-head">
          <h2>{{ t("home.latest") }}</h2>
          <RouterLink to="/gallery">
            <NButton text type="primary">{{ t("home.moreGallery") }}</NButton>
          </RouterLink>
        </div>
        <NSpin :show="previewLoading">
          <NEmpty
            v-if="!previewLoading && preview.length === 0"
            class="public-home__empty"
            :description="t('gallery.empty')"
          />
          <NImageGroup v-else :render-toolbar="renderPreviewToolbar">
            <div class="public-home__masonry">
              <GalleryCard v-for="item in preview" :key="item.id" :item="item" />
            </div>
          </NImageGroup>
        </NSpin>
      </section>
    </main>

    <ImageCopyDrawer v-model:show="drawerShow" :image="drawerImage" />
  </div>
</template>

<style scoped>
.public-home {
  min-height: 100%;
  background: var(--n-color, hsl(var(--background)));
}
.public-home__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  max-width: 1120px;
  margin: 0 auto;
  padding: 16px;
}
.public-home__brand {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  font-size: 18px;
  font-weight: 600;
  color: inherit;
  text-decoration: none;
}
.public-home__brand span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.public-home__logo {
  width: 28px;
  height: 28px;
  flex: none;
}
.public-home__nav {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: none;
}
.public-home__main {
  display: flex;
  flex-direction: column;
  gap: 24px;
  max-width: 1120px;
  margin: 0 auto;
  padding: 8px 16px 48px;
}
.public-home__hero {
  text-align: center;
  padding: 24px 0 8px;
}
.public-home__title {
  margin: 0;
  font-size: clamp(28px, 5vw, 40px);
  font-weight: 700;
  overflow-wrap: anywhere;
}
.public-home__subtitle {
  margin: 8px 0 0;
  opacity: 0.7;
}
.public-home__card {
  width: 100%;
  max-width: 820px;
  margin: 0 auto;
}
.public-home__alert {
  margin-bottom: 12px;
}
.public-home__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 12px;
}
.public-home__limits,
.public-home__note {
  font-size: 13px;
  opacity: 0.7;
}
.public-home__note {
  margin: 8px 0 0;
}
.public-home__queue-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 20px;
}
.public-home__link-controls {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 12px 0;
}
.public-home__closed {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 24px 0;
  text-align: center;
}
.public-home__closed p {
  margin: 0;
}
.public-home__gallery-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.public-home__gallery-head h2 {
  margin: 0;
  font-size: 18px;
}
.public-home__empty {
  padding: 48px 16px;
}
.public-home__masonry {
  columns: 4;
  column-gap: 16px;
}
@media (max-width: 1024px) {
  .public-home__masonry {
    columns: 3;
  }
}
@media (max-width: 720px) {
  .public-home__masonry {
    columns: 2;
  }
}
</style>
