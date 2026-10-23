<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";
import AccountPage from "@/components/account/AccountPage.vue";
import UserPluginSettings from "@/components/account/UserPluginSettings.vue";

const route = useRoute();
const name = computed(() => (typeof route.params.plugin === "string" ? route.params.plugin : ""));
</script>

<template>
  <AccountPage>
    <UserPluginSettings :name="name" />
  </AccountPage>
</template>
