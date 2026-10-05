/**
 * 图片域测试夹具：makeImage / makeExif。
 * 类型来自 @/api/images（schema 派生），新增必填字段时在这里补齐。
 */
import type { ImageExif, ImageView } from "@/api/images";

export function makeImage(overrides: Partial<ImageView> = {}): ImageView {
  return {
    id: 1,
    key: "2026/10/04/k1",
    user_id: 1,
    album_id: 0,
    policy_id: 1,
    storage_id: 1,
    name: "photo.png",
    ext: "png",
    mime: "image/png",
    size: 2048,
    webp_size: 1024,
    charged_bytes: 3072,
    width: 100,
    height: 80,
    frames: 1,
    has_original: true,
    has_webp: true,
    has_thumb: true,
    scrubbed: false,
    is_public: false,
    md5: "md5-stored",
    sha1: "sha1-stored",
    src_md5: "md5-source",
    links: {
      url: "/i/1/2026/10/04/k1.png",
      original: "/i/1/2026/10/04/k1.png",
      webp: "/i/1/2026/10/04/k1.webp",
      thumbnail_url: "",
    },
    local_thumb_url: "/t/1/2026/10/04/k1.webp",
    deleted_at: null,
    purge_at: null,
    created_at: "2026-10-04T00:00:00Z",
    ...overrides,
  };
}

export function makeExif(overrides: Partial<ImageExif> = {}): ImageExif {
  return {
    image_id: 1,
    make: "Canon",
    model: "EOS R6",
    lens: "RF24-70mm F2.8 L IS USM",
    exposure: "1/250",
    f_number: "f/2.8",
    focal_length: "35mm",
    iso: 400,
    orientation: 1,
    taken_at: "2026-10-01T10:00:00Z",
    gps_lat: 31.2304,
    gps_lng: 121.4737,
    gps_alt: 4,
    raw: { Make: "Canon", Model: "EOS R6", GPSLatitude: 31.2304 },
    created_at: "2026-10-04T00:00:00Z",
    updated_at: "2026-10-04T00:00:00Z",
    ...overrides,
  };
}
