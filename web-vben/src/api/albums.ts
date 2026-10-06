/**
 * 相册域（/api/albums）的 API 封装。
 * 契约来源：docs/superpowers/specs/2026-10-05-m5-albums-gallery-design.md
 * （契约先于后端实现冻结；native /api/albums 段补进 openapi 之前类型为手写，
 * 与 admin.ts 同模式，后续由父代理 gen:api 切换为派生类型）。
 *
 * 全部走 client.request：自动携带 Bearer、解包 {code,message,data} 外壳、
 * 业务失败抛 ApiError（10001 名称非法/引用图片非本人、20003 他人资源）。
 */
import { request } from "./client";
import type { components } from "./schema";

/** 相册安全视图：契约 AlbumView；cover_image_id 0=无封面。 */
export interface AlbumView {
  id: number;
  name: string;
  intro: string;
  is_public: boolean;
  cover_image_id: number;
  image_count: number;
  /** 实时拼接的封面缩略图链接；无封面为空串。 */
  cover_thumb_url: string;
  created_at: string;
  updated_at: string;
}

/** POST /api/albums 请求体：name 必填，其余缺省走后端默认值。 */
export interface AlbumInput {
  name: string;
  intro?: string;
  is_public?: boolean;
  cover_image_id?: number;
}

/** PATCH /api/albums/{id} 请求体（部分更新）。 */
export type AlbumPatch = Partial<AlbumInput>;

/** listAlbums 的过滤参数。 */
export interface AlbumListParams {
  page?: number;
  size?: number;
  /** 按名称模糊搜索。 */
  keyword?: string;
}

/** 相册分页数据。 */
export interface AlbumPage {
  items: AlbumView[];
  total: number;
  page: number;
  size: number;
}

const JSON_HEADERS = { "Content-Type": "application/json" } as const;

/** 组装查询串；无参数时返回空串。 */
function albumQuery(params: AlbumListParams): string {
  const search = new URLSearchParams();
  if (params.page !== undefined) {
    search.set("page", String(params.page));
  }
  if (params.size !== undefined) {
    search.set("size", String(params.size));
  }
  if (params.keyword) {
    search.set("keyword", params.keyword);
  }
  const query = search.toString();
  return query ? `?${query}` : "";
}

/** 分页列出当前用户的相册（keyword 按名称模糊搜索）。 */
export function listAlbums(params: AlbumListParams = {}): Promise<AlbumPage> {
  return request<AlbumPage>(`/api/albums${albumQuery(params)}`);
}

/** 创建相册；契约返回 201 与新建的 AlbumView。 */
export function createAlbum(body: AlbumInput): Promise<AlbumView> {
  return request<AlbumView>("/api/albums", {
    method: "POST",
    headers: { ...JSON_HEADERS },
    body: JSON.stringify(body),
  });
}

/** 部分更新相册；返回更新后的 AlbumView。 */
export function updateAlbum(id: number, patch: AlbumPatch): Promise<AlbumView> {
  return request<AlbumView>(`/api/albums/${id}`, {
    method: "PATCH",
    headers: { ...JSON_HEADERS },
    body: JSON.stringify(patch),
  });
}

/**
 * 删除相册；相册内图片保留（album_id 置 0），成功 data 为 null。
 */
export function deleteAlbum(id: number): Promise<null> {
  return request<null>(`/api/albums/${id}`, { method: "DELETE" });
}

/** Bounded current-owner suggestions, independent of legacy first-100 move options. */
export type AlbumSuggestion = components["schemas"]["SearchAlbum"];
export type AlbumSuggestionPage = components["schemas"]["AlbumSuggestions"];
export function suggestAlbums(
  keyword: string,
  page = 1,
  signal?: AbortSignal,
  scopeAlbumId?: string,
): Promise<AlbumSuggestionPage> {
  const query = new URLSearchParams({ keyword, page: String(page), size: "20" });
  if (scopeAlbumId !== undefined) query.set("scope_album_id", scopeAlbumId);
  return request<AlbumSuggestionPage>(`/api/albums/suggestions?${query}`, { signal });
}
