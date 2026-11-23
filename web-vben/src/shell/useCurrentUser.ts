import { computed } from "vue";
import { useI18n } from "@vben/locales";
import { useAvatar } from "@/app.hooks";
import { useAuthStore } from "@/stores/auth";

/** 外壳里显示的用户名与头像（头像来自 app.hooks.ts 的 useAvatar）。 */
export function useCurrentUser() {
  const auth = useAuthStore();
  const { t } = useI18n();
  const user = computed(() => (auth.token ? auth.user : null));
  const displayName = computed(
    () => user.value?.displayName || user.value?.username || t("common.account"),
  );
  const avatar = useAvatar(user);
  return { user, displayName, avatar };
}
