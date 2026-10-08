import { beforeEach, describe, expect, it, vi } from "vitest";
import { listAlbums, type AlbumView } from "./albums";
import { listAllAlbums } from "./allAlbums";

vi.mock("./albums", () => ({
  listAlbums: vi.fn(),
}));

const listAlbumsMock = vi.mocked(listAlbums);

function albums(from: number, count: number): AlbumView[] {
  return Array.from({ length: count }, (_, index) => ({
    id: from + index,
    name: `相册 ${from + index}`,
    intro: "",
    is_public: false,
    cover_image_id: 0,
    image_count: 0,
    cover_thumb_url: "",
    created_at: "2026-10-08T00:00:00Z",
    updated_at: "2026-10-08T00:00:00Z",
  }));
}

beforeEach(() => {
  listAlbumsMock.mockReset();
});

describe("listAllAlbums", () => {
  it("超过 100 个相册时继续翻页直到取满 total", async () => {
    listAlbumsMock
      .mockResolvedValueOnce({ items: albums(1, 100), total: 230, page: 1, size: 100 })
      .mockResolvedValueOnce({ items: albums(101, 100), total: 230, page: 2, size: 100 })
      .mockResolvedValueOnce({ items: albums(201, 30), total: 230, page: 3, size: 100 });

    const result = await listAllAlbums();

    expect(result).toHaveLength(230);
    expect(result.at(-1)?.id).toBe(230);
    expect(listAlbumsMock.mock.calls.map(([params]) => params)).toEqual([
      { page: 1, size: 100 },
      { page: 2, size: 100 },
      { page: 3, size: 100 },
    ]);
  });

  it("不足一页时只请求一次", async () => {
    listAlbumsMock.mockResolvedValueOnce({ items: albums(1, 2), total: 2, page: 1, size: 100 });

    expect(await listAllAlbums()).toHaveLength(2);
    expect(listAlbumsMock).toHaveBeenCalledTimes(1);
  });

  it("total 失真时遇到空页即停止", async () => {
    listAlbumsMock
      .mockResolvedValueOnce({ items: albums(1, 100), total: 9999, page: 1, size: 100 })
      .mockResolvedValueOnce({ items: [], total: 9999, page: 2, size: 100 });

    expect(await listAllAlbums()).toHaveLength(100);
    expect(listAlbumsMock).toHaveBeenCalledTimes(2);
  });
});
