<script setup lang="ts">
import SettingsFieldsForm from "./SettingsFieldsForm.vue";
import GalleryPreviewPanel from "./GalleryPreviewPanel.vue";
import { GALLERY, GALLERY_PUBLIC_ALBUMS_ONLY, GUEST_GROUP, GUEST_UPLOAD } from "./fields";

const fields = [GUEST_UPLOAD, GUEST_GROUP, GALLERY, GALLERY_PUBLIC_ALBUMS_ONLY];
</script>

<template>
  <SettingsFieldsForm :fields="fields">
    <template #after-gallery_public_albums_only="{ value, loaded }">
      <GalleryPreviewPanel v-if="loaded" :public-albums-only="value as boolean" />
    </template>
  </SettingsFieldsForm>
</template>
