/**
 * 管理后台域（/api/admin/*）的 API 封装。
 * 契约来源：docs/superpowers/specs/2026-10-04-m4-lsky-admin-design.md 契约表。
 *
 * 类型在 openapi 补齐 /api/admin 段之前为手写（后端 worker 阶段2补 schema 后，
 * 由父代理 gen:api 统一切换为派生类型）；本模块的函数签名是阶段2页面 worker
 * 的消费契约，变更需同步 docs/superpowers/plans/2026-10-04-m4-lsky-admin.md。
 *
 * 全部走 client.request：自动携带 Bearer、解包 {code,message,data}、
 * 业务失败抛 ApiError（20003 非 admin、30008 组仍有成员、30009 存储被规则引用）。
 */
import { request } from "./client";
import type { ImagePage } from "./images";

/** 用户角色（与 openapi UserView.role 枚举一致）。 */
export type UserRole = "admin" | "user";
/** 用户状态。 */
export type UserStatus = "enabled" | "disabled";

/** 管理端用户视图：契约 AdminUserView（含个人资料与头像扩展字段）。 */
export interface AdminUserView {
  id: number;
  display_name: string;
  username: string;
  email: string;
  role: UserRole;
  status: UserStatus;
  group_id: number;
  /** 已用容量（字节）。 */
  used_bytes: number;
  created_at: string;
  avatar_provider: AvatarProvider;
  avatar_url: string | null;
  avatar_config_version: number;
}

/** PATCH /api/admin/users/{id} 的请求体。 */
export interface AdminUserPatch {
  status?: UserStatus;
  group_id?: number;
}

/** listUsers 的过滤参数。 */
export interface AdminUserListParams {
  page?: number;
  size?: number;
  /** 按 username/email 模糊搜索。 */
  keyword?: string;
}

/** 用户分页数据。 */
export interface AdminUserPage {
  items: AdminUserView[];
  total: number;
  page: number;
  size: number;
}

/** 管理端用户组视图：契约 GroupView（GET 列表无分页，返回数组）。 */
export interface GroupView {
  id: number;
  name: string;
  is_default: boolean;
  is_guest: boolean;
  capacity_bytes: number;
  max_file_bytes: number;
  allowed_exts: string[];
  /** 每分钟上传上限（游客限流同样取该值）。 */
  upload_per_min: number;
  default_policy_id: number;
  policy_ids: number[];
  user_count: number;
}

/** POST /api/admin/groups 请求体（契约表未细化 body，此处为前后端对齐形状）。 */
export interface GroupInput {
  name: string;
  capacity_bytes: number;
  max_file_bytes: number;
  allowed_exts: string[];
  upload_per_min: number;
  default_policy_id: number;
  policy_ids: number[];
  is_default?: boolean;
  is_guest?: boolean;
}

/** PATCH /api/admin/groups/{id} 请求体。 */
export type GroupPatch = Partial<GroupInput>;

/** 存储驱动。 */
export type StorageDriver = "local" | "s3";

/** 管理端存储视图：契约 StorageView；config/密钥永不回显。 */
export interface StorageView {
  id: number;
  name: string;
  driver: StorageDriver;
  base_url: string;
  enabled: boolean;
}

/** POST /api/admin/storages 请求体；config 为驱动专属 JSON（S3 凭证仅提交不回显）。 */
export interface StorageInput {
  name: string;
  driver: StorageDriver;
  base_url: string;
  config?: Record<string, unknown>;
}

/** PATCH /api/admin/storages/{id} 请求体。 */
export interface StoragePatch {
  name?: string;
  base_url?: string;
  config?: Record<string, unknown>;
  enabled?: boolean;
}

/** POST /api/admin/storages/{id}/test 的连接测试结果。 */
export interface StorageTestResult {
  ok: boolean;
  checks: {
    put: boolean;
    copy: boolean;
    delete: boolean;
  };
}

/** WebP 处理模式。 */
export type WebPMode = "both" | "webp_only" | "none";
/** EXIF 脱敏模式。 */
export type ScrubMode = "gps" | "all" | "none";
/** 直链偏好。 */
export type LinkPrefer = "webp" | "original";
/** HEIF 处理模式。 */
export type HEIFMode = "webp_only" | "keep" | "reject";
/** 同名冲突策略。 */
export type OnConflict = "rename" | "reject";

/**
 * 管理端规则视图：契约 PolicyView（规则全字段 + storage_id + enabled）。
 * 枚举值与 internal/model/policy.go 的校验保持一致。
 */
export interface PolicyView {
  id: number;
  name: string;
  storage_id: number;
  enabled: boolean;
  path_tpl: string;
  name_tpl: string;
  webp_mode: WebPMode;
  webp_quality: number;
  webp_lossless: boolean;
  webp_effort: number;
  max_width: number;
  max_height: number;
  thumb_enabled: boolean;
  thumb_size: number;
  scrub_mode: ScrubMode;
  heif_mode: HEIFMode;
  link_prefer: LinkPrefer;
  on_conflict: OnConflict;
  strip_meta: boolean;
  skip_if_larger: boolean;
  created_at: string;
  updated_at: string;
}

/** POST /api/admin/policies 请求体：name/storage_id 必填，其余缺省走后端默认值。 */
export type PolicyInput = Pick<PolicyView, "name" | "storage_id"> &
  Partial<Omit<PolicyView, "id" | "name" | "storage_id" | "created_at" | "updated_at">>;

/** PATCH /api/admin/policies/{id} 请求体。 */
export type PolicyPatch = Partial<PolicyInput>;

/** POST /api/admin/policies/preview 请求体（pathtpl 渲染样例）。 */
export interface PolicyPreviewInput {
  path_tpl: string;
  name_tpl: string;
}

/** 规则路径预览结果：sample 为样例路径，error 为模板错误描述（无错误为空串）。 */
export interface PolicyPreviewResult {
  sample: string;
  error: string;
}

/** 站点头像服务商（与 openapi AdminSettings.avatar_provider 枚举一致）。 */
export type AvatarProvider = "weavatar" | "gravatar";

/** 管理端站点设置：契约 settings 九字段。 */
export interface AdminSettings {
  site_name: string;
  registration_enabled: boolean;
  guest_upload_enabled: boolean;
  gallery_enabled: boolean;
  /** 回收站保留天数。 */
  trash_days: number;
  /** /api/v1 蓝空兼容层总开关。 */
  api_enabled: boolean;
  guest_group_id: number;
  default_group_id: number;
  /** 全站头像服务商；普通用户不可选择。 */
  avatar_provider: AvatarProvider;
}

/** listAdminImages 的过滤参数。 */
export interface AdminImageListParams {
  page?: number;
  size?: number;
  /** 按属主过滤。 */
  user_id?: number;
  /** 按 origin_name/pathname 模糊搜索。 */
  keyword?: string;
}

/** 全站图片分页数据（与用户端 ImagePage 同形状）。 */
export type AdminImagePage = ImagePage;

/** POST /api/admin/trash/purge-all 的结果。 */
export interface TrashPurgeResult {
  purged: number;
}

const JSON_HEADERS = { "Content-Type": "application/json" } as const;

/** 组装查询串；无参数时返回空串。 */
function adminQuery(params: Record<string, string | number | undefined>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined) {
      search.set(key, String(value));
    }
  }
  const query = search.toString();
  return query ? `?${query}` : "";
}

// ---------- 用户 ----------

/** 分页列出全站用户（keyword 搜 username/email）。 */
export function listUsers(params: AdminUserListParams = {}): Promise<AdminUserPage> {
  return request<AdminUserPage>(
    `/api/admin/users${adminQuery({ page: params.page, size: params.size, keyword: params.keyword })}`,
  );
}

/** 修改用户状态/所属组；返回更新后的 AdminUserView。 */
export function patchUser(id: number, patch: AdminUserPatch): Promise<AdminUserView> {
  return request<AdminUserView>(`/api/admin/users/${id}`, {
    method: "PATCH",
    headers: { ...JSON_HEADERS },
    body: JSON.stringify(patch),
  });
}

// ---------- 用户组 ----------

/** 列出全部用户组（含成员数；契约 GET 列表无分页）。 */
export function listGroups(): Promise<GroupView[]> {
  return request<GroupView[]>("/api/admin/groups");
}

/** 创建用户组。 */
export function createGroup(body: GroupInput): Promise<GroupView> {
  return request<GroupView>("/api/admin/groups", {
    method: "POST",
    headers: { ...JSON_HEADERS },
    body: JSON.stringify(body),
  });
}

/** 更新用户组。 */
export function updateGroup(id: number, patch: GroupPatch): Promise<GroupView> {
  return request<GroupView>(`/api/admin/groups/${id}`, {
    method: "PATCH",
    headers: { ...JSON_HEADERS },
    body: JSON.stringify(patch),
  });
}

/** 删除用户组；仍有成员时后端返回 30008。 */
export function deleteGroup(id: number): Promise<null> {
  return request<null>(`/api/admin/groups/${id}`, { method: "DELETE" });
}

// ---------- 存储 ----------

/** 列出全部存储（不含任何凭证/config）。 */
export function listStorages(): Promise<StorageView[]> {
  return request<StorageView[]>("/api/admin/storages");
}

/** 创建存储；S3 config 密钥加密落库，响应不回显。 */
export function createStorage(body: StorageInput): Promise<StorageView> {
  return request<StorageView>("/api/admin/storages", {
    method: "POST",
    headers: { ...JSON_HEADERS },
    body: JSON.stringify(body),
  });
}

/** 更新存储（config 传入时整体替换并重新加密）。 */
export function updateStorage(id: number, patch: StoragePatch): Promise<StorageView> {
  return request<StorageView>(`/api/admin/storages/${id}`, {
    method: "PATCH",
    headers: { ...JSON_HEADERS },
    body: JSON.stringify(patch),
  });
}

/** 删除存储；被规则引用时后端返回 30009。 */
export function deleteStorage(id: number): Promise<null> {
  return request<null>(`/api/admin/storages/${id}`, { method: "DELETE" });
}

/** 连接测试：逐项返回 put/copy/delete 结果。 */
export function testStorage(id: number): Promise<StorageTestResult> {
  return request<StorageTestResult>(`/api/admin/storages/${id}/test`, { method: "POST" });
}

// ---------- 规则 ----------

/** 列出全部规则。 */
export function listPolicies(): Promise<PolicyView[]> {
  return request<PolicyView[]>("/api/admin/policies");
}

/** 创建规则。 */
export function createPolicy(body: PolicyInput): Promise<PolicyView> {
  return request<PolicyView>("/api/admin/policies", {
    method: "POST",
    headers: { ...JSON_HEADERS },
    body: JSON.stringify(body),
  });
}

/** 更新规则。 */
export function updatePolicy(id: number, patch: PolicyPatch): Promise<PolicyView> {
  return request<PolicyView>(`/api/admin/policies/${id}`, {
    method: "PATCH",
    headers: { ...JSON_HEADERS },
    body: JSON.stringify(patch),
  });
}

/** 删除规则。 */
export function deletePolicy(id: number): Promise<null> {
  return request<null>(`/api/admin/policies/${id}`, { method: "DELETE" });
}

/** 预览路径模板渲染样例；模板非法时 error 非空。 */
export function previewPolicy(body: PolicyPreviewInput): Promise<PolicyPreviewResult> {
  return request<PolicyPreviewResult>("/api/admin/policies/preview", {
    method: "POST",
    headers: { ...JSON_HEADERS },
    body: JSON.stringify(body),
  });
}

// ---------- 站点设置 ----------

/** 读取站点设置八字段。 */
export function getSettings(): Promise<AdminSettings> {
  return request<AdminSettings>("/api/admin/settings");
}

/** 保存站点设置（PATCH 语义的 PUT：传部分字段）；返回保存后的完整设置。 */
export function putSettings(patch: Partial<AdminSettings>): Promise<AdminSettings> {
  return request<AdminSettings>("/api/admin/settings", {
    method: "PUT",
    headers: { ...JSON_HEADERS },
    body: JSON.stringify(patch),
  });
}

// ---------- 全站图片与回收站 ----------

/** 全站分页列出图片（可按属主/关键词过滤）。 */
export function listAdminImages(params: AdminImageListParams = {}): Promise<AdminImagePage> {
  return request<AdminImagePage>(
    `/api/admin/images${adminQuery({
      page: params.page,
      size: params.size,
      user_id: params.user_id,
      keyword: params.keyword,
    })}`,
  );
}

/** 把全站任一图片移入回收站（原 URL 立即 404）；成功 data 为 null。 */
export function deleteAdminImage(id: number): Promise<null> {
  return request<null>(`/api/admin/images/${id}`, { method: "DELETE" });
}

/** 清空全站回收站；返回彻底删除的图片数。 */
export function purgeAllTrash(): Promise<TrashPurgeResult> {
  return request<TrashPurgeResult>("/api/admin/trash/purge-all", { method: "POST" });
}
