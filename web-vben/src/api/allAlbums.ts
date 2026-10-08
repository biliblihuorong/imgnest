import { listAlbums, type AlbumView } from "./albums";

/** 后端单页上限（native /api/albums size ≤ 100）。 */
const PAGE_SIZE = 100;
/** 翻页上限，防止 total 异常时无限请求；100 页即 10000 个相册。 */
const MAX_PAGES = 100;

/**
 * 逐页取回当前用户的全部相册，供上传、移动等选择器使用。
 * 单页只能取 100 个，超出的相册否则无法被选为目标。
 */
export async function listAllAlbums(): Promise<AlbumView[]> {
  const albums: AlbumView[] = [];
  for (let page = 1; page <= MAX_PAGES; page++) {
    const data = await listAlbums({ page, size: PAGE_SIZE });
    albums.push(...data.items);
    if (data.items.length < PAGE_SIZE || albums.length >= data.total) {
      break;
    }
  }
  return albums;
}
