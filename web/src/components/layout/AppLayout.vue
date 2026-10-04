<script setup lang="ts">
import {
  NButton,
  NDropdown,
  NLayout,
  NLayoutContent,
  NLayoutHeader,
  NMenu,
  type DropdownOption,
  type MenuOption,
} from "naive-ui";
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const site = useSiteStore();

void site.ensureLoaded();

const menuOptions: MenuOption[] = [
  { label: "上传", key: "/upload" },
  { label: "图片", key: "/images" },
  { label: "Token", key: "/tokens" },
];

const activeMenuKey = computed(() => route.path);

const userOptions = computed<DropdownOption[]>(() => [
  { label: auth.user?.username ?? "未登录", key: "username", disabled: true },
  { type: "divider", key: "user-divider" },
  { label: "修改密码", key: "change-password" },
  { label: "退出登录", key: "logout" },
]);

function onMenuSelect(key: string | number): void {
  void router.push(String(key));
}

async function onUserSelect(key: string | number): Promise<void> {
  if (key === "change-password") {
    await router.push("/tokens");
    return;
  }
  if (key === "logout") {
    await auth.logout();
    await router.push("/login");
  }
}
</script>

<template>
  <NLayout style="min-height: 100vh">
    <NLayoutHeader bordered>
      <div class="app-header">
        <span class="app-brand">{{ site.siteName }}</span>
        <NMenu
          class="app-menu"
          mode="horizontal"
          responsive
          :value="activeMenuKey"
          :options="menuOptions"
          @update:value="onMenuSelect"
        />
        <NDropdown trigger="click" :options="userOptions" @select="onUserSelect">
          <NButton quaternary>{{ auth.user?.username ?? "账户" }}</NButton>
        </NDropdown>
      </div>
    </NLayoutHeader>
    <NLayoutContent content-style="padding: 24px;">
      <RouterView />
    </NLayoutContent>
  </NLayout>
</template>

<style scoped>
.app-header {
  display: flex;
  align-items: center;
  gap: 16px;
  max-width: 1080px;
  margin: 0 auto;
  padding: 0 16px;
}

.app-brand {
  font-size: 18px;
  font-weight: 600;
  white-space: nowrap;
}

.app-menu {
  flex: 1;
}

@media (max-width: 640px) {
  .app-header {
    flex-wrap: wrap;
  }
}
</style>
