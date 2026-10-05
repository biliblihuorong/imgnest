import type { UserView } from "@/api/types";
export interface WorkspaceMenu {
  name: string;
  path: string;
  icon: string;
  children?: WorkspaceMenu[];
}
export function workspaceMenus(
  role: UserView["role"] | undefined,
  galleryEnabled = false,
): WorkspaceMenu[] {
  const gallery = { name: "common.nav.gallery", path: "/gallery", icon: "LayoutGrid" };
  if (!role) return galleryEnabled ? [gallery] : [];
  const menus: WorkspaceMenu[] = [
    { name: "common.nav.upload", path: "/upload", icon: "ImagePlus" },
    { name: "common.nav.images", path: "/images", icon: "LayoutGrid" },
    { name: "common.nav.albums", path: "/albums", icon: "Inbox" },
    { name: "common.nav.dashboard", path: "/dashboard", icon: "InspectionPanel" },
    { name: "common.nav.tokens", path: "/tokens", icon: "LockKeyhole" },
  ];
  if (galleryEnabled) menus.push(gallery);
  if (role === "admin")
    menus.push({
      name: "common.nav.admin",
      path: "/admin",
      icon: "Settings",
      children: [
        { name: "common.nav.users", path: "/admin/users", icon: "UserRoundPen" },
        { name: "common.nav.groups", path: "/admin/groups", icon: "InspectionPanel" },
        { name: "common.nav.storages", path: "/admin/storages", icon: "Inbox" },
        { name: "common.nav.policies", path: "/admin/policies", icon: "List" },
        { name: "common.nav.settings", path: "/admin/settings", icon: "Settings" },
        { name: "common.nav.allImages", path: "/admin/images", icon: "LayoutGrid" },
      ],
    });
  return menus;
}
