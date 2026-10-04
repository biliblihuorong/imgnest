<script setup lang="ts">
import {
  NButton,
  NDropdown,
  NLayout,
  NLayoutContent,
  NLayoutHeader,
  NLayoutSider,
  NMenu,
  type DropdownOption,
  type MenuOption,
} from "naive-ui";
import { computed, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const site = useSiteStore();

void site.ensureLoaded();

const collapsed = ref(false);

const menuOptions: MenuOption[] = [
  { label: "用户", key: "/admin/users" },
  { label: "用户组", key: "/admin/groups" },
  { label: "存储", key: "/admin/storages" },
  { label: "规则", key: "/admin/policies" },
  { label: "站点设置", key: "/admin/settings" },
  { label: "全站图片", key: "/admin/images" },
];

const activeMenuKey = computed(() => route.path);

const userOptions = computed<DropdownOption[]>(() => [
  { label: auth.user?.username ?? "未登录", key: "username", disabled: true },
  { type: "divider", key: "user-divider" },
  { label: "返回前台", key: "front" },
  { label: "退出登录", key: "logout" },
]);

function onMenuSelect(key: string | number): void {
  void router.push(String(key));
}

async function onUserSelect(key: string | number): Promise<void> {
  if (key === "front") {
    await router.push("/upload");
    return;
  }
  if (key === "logout") {
    await auth.logout();
    await router.push("/login");
  }
}
</script>

<template>
  <NLayout has-sider style="min-height: 100vh">
    <NLayoutSider
      v-model:collapsed="collapsed"
      bordered
      collapse-mode="width"
      :collapsed-width="64"
      :width="200"
      show-trigger
    >
      <div class="admin-brand">
        <span v-if="!collapsed">{{ site.siteName }} 管理后台</span>
        <span v-else>管</span>
      </div>
      <NMenu
        :value="activeMenuKey"
        :collapsed="collapsed"
        :collapsed-width="64"
        :options="menuOptions"
        @update:value="onMenuSelect"
      />
    </NLayoutSider>
    <NLayout>
      <NLayoutHeader bordered>
        <div class="admin-header">
          <span class="admin-title">站点管理</span>
          <NDropdown trigger="click" :options="userOptions" @select="onUserSelect">
            <NButton quaternary>{{ auth.user?.username ?? "账户" }}</NButton>
          </NDropdown>
        </div>
      </NLayoutHeader>
      <NLayoutContent content-style="padding: 24px;">
        <RouterView />
      </NLayoutContent>
    </NLayout>
  </NLayout>
</template>

<style scoped>
.admin-brand {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 48px;
  font-size: 16px;
  font-weight: 600;
  white-space: nowrap;
}

.admin-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  height: 48px;
}

.admin-title {
  font-size: 16px;
  font-weight: 600;
}
</style>
