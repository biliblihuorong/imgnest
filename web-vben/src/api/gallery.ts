/**
 * 公开画廊域的 API 封装。
 * 契约唯一事实来源：docs/superpowers/specs/2026-10-05-m5-albums-gallery-design.md
 * （GET /api/gallery 分页外壳与 ImagePage 同形，items 为窄化的 GalleryItem）。
 *
 * 公开接口：后端按 is_public + 未删除 + 账号启用过滤（可选仅公开相册），无需鉴权；站点开关
 * gallery_enabled 关闭时返回 200 空页（items:[]、total:0），前端据此区分空态。
 */
import { request } from "./client";
import type { ImagePage, ImageView, PageParams } from "./images";

/**
 * 画廊项：匿名访客可见的窄视图，只含展示所需字段与上传者用户名；
 * 不含属主、相册、规则、存储、内容哈希等 ImageView 字段。
 */
export type GalleryItem = Pick<
  ImageView,
  "id" | "name" | "ext" | "mime" | "size" | "width" | "height" | "frames" | "links" | "created_at"
> & { uploader: string };

/** 公开画廊分页：与 ImagePage 同形（items/total/page/size），items 换成 GalleryItem。 */
export type GalleryPage = Omit<ImagePage, "items"> & { items: GalleryItem[] };

function galleryQuery(params: PageParams): string {
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

/** 分页列出公开画廊图片；匿名可访问，client 自动附加的 token 无碍。 */
export function listGallery(params: PageParams = {}): Promise<GalleryPage> {
  return request<GalleryPage>(`/api/gallery${galleryQuery(params)}`);
}
