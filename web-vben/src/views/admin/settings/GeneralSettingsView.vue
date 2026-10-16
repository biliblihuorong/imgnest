<script setup lang="ts">
import SettingsFieldsForm from "./SettingsFieldsForm.vue";
import { API_ENABLED, AVATAR_PROVIDER, SITE_NAME, TRASH_DAYS } from "./fields";

const fields = [SITE_NAME, AVATAR_PROVIDER, API_ENABLED, TRASH_DAYS];
</script>

<template>
  <SettingsFieldsForm :fields="fields" />
</template>
