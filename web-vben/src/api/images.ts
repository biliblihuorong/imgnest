/**
 * 图片与回收站域的 API 封装。
 * 契约唯一事实来源：docs/openapi.yaml（/api/images、/api/trash 系列与
 * ImageView / ImagePage / ImageExif / ImageBatchItem schema）。
 */
import { request } from "./client";
import type { components } from "./schema";

/** 图片安全视图：不含 EXIF/GPS/源 IP/存储凭证。 */
export type ImageView = components["schemas"]["ImageView"];
/** 图片分页数据。 */
export type ImagePage = components["schemas"]["ImagePage"];
/** 完整私有元数据（含 GPS 与 raw 归档），仅本人可见。 */
export type ImageExif = components["schemas"]["ImageExif"];
/** 207 逐项结果（上传/删除/恢复/彻底删除/改可见性共用）。 */
export type ImageBatchItem = components["schemas"]["ImageBatchItem"];

/** 列表分页参数（size 上限 100，契约默认 page=1、size=20）。 */
export interface PageParams {
  page?: number;
  size?: number;
}

/**
 * 图片列表参数：分页 + 相册/关键词/排序/大小/时间/EXIF 过滤。
 * album_id 缺省（undefined）=全部；显式 0=未归类（不属于任何相册）。
 */
export interface ListParams extends PageParams {
  album_id?: number;
  /** 统一搜索：文件名 OR 相机品牌/型号/镜头，大小写不敏感子串。 */
  q?: string;
  /** 文件名（显示名或存储路径+扩展名）大小写不敏感的子串匹配。 */
  keyword?: string;
  /** 排序；缺省=newest。 */
  order?: "newest" | "oldest" | "largest" | "smallest";
  /** 原图字节数下限（含）；0/缺省=不限。 */
  min_size?: number;
  /** 原图字节数上限（含）；0/缺省=不限。 */
  max_size?: number;
  /** 上传时间下限（含，RFC3339）。 */
  from?: string;
  /** 上传时间上限（含，RFC3339）。 */
  to?: string;
  /** EXIF 相机品牌/型号/镜头的子串匹配（仅对存有元数据的图片）。 */
  exif?: string;
}

function pageQuery(params: ListParams): string {
  const search = new URLSearchParams();
  if (params.page !== undefined) {
    search.set("page", String(params.page));
  }
  if (params.size !== undefined) {
    search.set("size", String(params.size));
  }
  // 显式 0 必须序列化（0=未归类），只有 undefined 才省略
  if (params.album_id !== undefined) {
    search.set("album_id", String(params.album_id));
  }
  if (params.q) {
    search.set("q", params.q);
  }
  if (params.keyword) {
    search.set("keyword", params.keyword);
  }
  if (params.order) {
    search.set("order", params.order);
  }
  if (params.min_size) {
    search.set("min_size", String(params.min_size));
  }
  if (params.max_size) {
    search.set("max_size", String(params.max_size));
  }
  if (params.from) {
    search.set("from", params.from);
  }
  if (params.to) {
    search.set("to", params.to);
  }
  if (params.exif) {
    search.set("exif", params.exif);
  }
  const query = search.toString();
  return query ? `?${query}` : "";
}

/** 分页列出当前用户的未删除图片（可按相册过滤）。 */
export function listImages(params: ListParams = {}): Promise<ImagePage> {
  return request<ImagePage>(`/api/images${pageQuery(params)}`);
}

/** 读取单张图片的安全元数据。 */
export function getImage(id: number): Promise<ImageView> {
  return request<ImageView>(`/api/images/${id}`);
}

/** 修改公私可见性；私有仅从公共列表/画廊隐藏，直链仍可访问。 */
export function setImageVisibility(id: number, isPublic: boolean): Promise<ImageView> {
  return request<ImageView>(`/api/images/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ is_public: isPublic }),
  });
}

/** 把图片移入回收站（原 URL 立即 404）；成功 data 为 null。 */
export function deleteImage(id: number): Promise<null> {
  return request<null>(`/api/images/${id}`, { method: "DELETE" });
}

/** 读取完整私有元数据（含 EXIF/GPS/raw），仅本人可见、响应不缓存。 */
export function getImageExif(id: number): Promise<ImageExif> {
  return request<ImageExif>(`/api/images/${id}/exif`);
}

/** 分页列出回收站（响应与图片列表同为 ImagePage，items 带 deleted_at/purge_at）。 */
export function listTrash(params: PageParams = {}): Promise<ImagePage> {
  return request<ImagePage>(`/api/trash${pageQuery(params)}`);
}

/** 回收站选中集（ImageSelection：ids 数组，1–100 个、去重）。 */
export interface ImageSelection {
  ids: number[];
}

/** 批量恢复选中的回收站图片到原路径；207 逐项结果。 */
export function restoreImages(ids: number[]): Promise<ImageBatchItem[]> {
  const body: ImageSelection = { ids };
  return request<ImageBatchItem[]>("/api/trash/restore", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

/** 批量彻底删除选中的回收站图片（不可恢复）；207 逐项结果。 */
export function purgeImages(ids: number[]): Promise<ImageBatchItem[]> {
  const body: ImageSelection = { ids };
  return request<ImageBatchItem[]>("/api/trash/purge", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

/* ---------------- 原生批量操作（POST /api/images/batch，207 逐项） ---------------- */

/** POST /api/images/batch 的请求体；响应按请求 ids 顺序逐项返回。 */
interface ImageBatchBody {
  action: "delete" | "permission" | "album";
  ids: number[];
  /** action=permission 时必带；delete/album 时省略。 */
  is_public?: boolean;
  /** action=album 时必带；0=移出相册。 */
  album_id?: number;
}

function postBatch(body: ImageBatchBody): Promise<ImageBatchItem[]> {
  return request<ImageBatchItem[]>("/api/images/batch", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

/** 批量把图片移入回收站（原 URL 立即 404）；207 逐项结果。 */
export function batchDelete(ids: number[]): Promise<ImageBatchItem[]> {
  return postBatch({ action: "delete", ids });
}

/** 批量修改公私可见性；207 逐项结果。 */
export function batchPermission(ids: number[], isPublic: boolean): Promise<ImageBatchItem[]> {
  return postBatch({ action: "permission", ids, is_public: isPublic });
}

/** 批量移动图片到相册（albumId=0 移出相册）；207 逐项结果。 */
export function batchAlbums(ids: number[], albumId: number): Promise<ImageBatchItem[]> {
  return postBatch({ action: "album", ids, album_id: albumId });
}

/** Versioned search always sends q, even when empty; fixed album scope uses a separate endpoint. */
export interface ImageSearchParams {
  qv: 1;
  q: string;
  tz: string;
  page: number;
  size: number;
}
export type ImageSearchMetadata = components["schemas"]["SearchMetadata"];
export type ImageSearchPage = ImagePage & { search: ImageSearchMetadata };
export function searchImages(
  params: ImageSearchParams,
  signal?: AbortSignal,
  albumId?: number | string,
): Promise<ImageSearchPage> {
  const query = new URLSearchParams({
    qv: "1",
    q: params.q,
    tz: params.tz,
    page: String(params.page),
    size: String(params.size),
  });
  const path = albumId === undefined ? "/api/images" : `/api/albums/${albumId}/images`;
  return request<ImageSearchPage>(`${path}?${query}`, { signal });
}
