<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NEmpty, NSpin } from "naive-ui";
import { computed } from "vue";
import { useAuthStore } from "@/stores/auth";
import UserPluginCard from "./UserPluginCard.vue";
import { useUserPlugins } from "./useUserPlugins";

const props = defineProps<{ name?: string }>();
const { t } = useI18n();
const auth = useAuthStore();
const { state, provided } = useUserPlugins();
const { plugins, loading, error, load, saved } = state;
const selected = computed(() =>
  props.name ? plugins.value.filter((item) => item.name === props.name) : plugins.value,
);
</script>

<template>
  <NSpin v-if="loading && !provided" class="mt-5" />
  <NAlert v-if="error && !provided" type="error" class="mt-5">
    {{ error }} <NButton size="small" @click="load">{{ t("account.retry") }}</NButton>
  </NAlert>
  <NEmpty
    v-if="name && !loading && !error && !selected.length"
    class="mt-5"
    :description="t('account.plugins.unavailable')"
  />
  <UserPluginCard
    v-for="plugin in selected"
    :key="`${auth.user?.id}:${auth.user?.group_id}:${plugin.name}`"
    :plugin="plugin"
    @saved="saved"
  />
</template>
