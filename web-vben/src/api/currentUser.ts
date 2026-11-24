import type { CurrentUser, UserView } from "./types";

/** 补上外壳（admin-kit）显示用的 displayName；空显示名回退用户名由外壳处理。 */
export function toCurrentUser(user: UserView): CurrentUser {
  return { ...user, displayName: user.display_name };
}
