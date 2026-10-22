<script setup lang="ts">
import { useI18n } from "@vben/locales";
import SettingsFieldsForm from "./SettingsFieldsForm.vue";
import GalleryPreviewPanel from "./GalleryPreviewPanel.vue";
import {
  GALLERY,
  GALLERY_PUBLIC_ALBUMS_ONLY,
  GUEST_GROUP,
  GUEST_UPLOAD,
  ROBOTS_CUSTOM,
  ROBOTS_MODE,
  SITEMAP,
} from "./fields";

const { t } = useI18n();

const fields = [
  GUEST_UPLOAD,
  GUEST_GROUP,
  GALLERY,
  GALLERY_PUBLIC_ALBUMS_ONLY,
  ROBOTS_MODE,
  ROBOTS_CUSTOM,
  SITEMAP,
];
</script>

<template>
  <SettingsFieldsForm :fields="fields">
    <template #after-gallery_public_albums_only="{ value, loaded }">
      <GalleryPreviewPanel v-if="loaded" :public-albums-only="value as boolean" />
    </template>
    <template #after-robots_mode>
      <a class="crawler-link" href="/robots.txt" target="_blank" rel="noopener">{{
        t("admin.settings.robotsOpen")
      }}</a>
    </template>
    <template #after-sitemap_enabled>
      <a class="crawler-link" href="/sitemap.xml" target="_blank" rel="noopener">{{
        t("admin.settings.sitemapOpen")
      }}</a>
    </template>
  </SettingsFieldsForm>
</template>

<style scoped>
.crawler-link {
  display: inline-block;
  margin-top: 4px;
  font-size: 12px;
  color: hsl(var(--primary));
}
</style>
