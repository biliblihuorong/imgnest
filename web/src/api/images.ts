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

function pageQuery(params: PageParams): string {
  const search = new URLSearchParams();
  if (params.page !== undefined) {
    search.set("page", String(params.page));
  }
  if (params.size !== undefined) {
    search.set("size", String(params.size));
  }
  const query = search.toString();
  return query ? `?${query}` : "";
}

/** 分页列出当前用户的未删除图片。 */
export function listImages(params: PageParams = {}): Promise<ImagePage> {
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
