/**
 * 相册域测试夹具：makeAlbum。
 * 类型来自 @/api/albums（手写契约），新增必填字段时在这里补齐。
 */
import type { AlbumView } from "@/api/albums";

export function makeAlbum(overrides: Partial<AlbumView> = {}): AlbumView {
  return {
    id: 1,
    name: "旅行",
    intro: "2026 年的旅途记录",
    is_public: false,
    cover_image_id: 0,
    image_count: 5,
    cover_thumb_url: "",
    created_at: "2026-10-04T00:00:00Z",
    updated_at: "2026-10-04T00:00:00Z",
    ...overrides,
  };
}
