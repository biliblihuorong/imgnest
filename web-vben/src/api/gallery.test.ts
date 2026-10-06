import { beforeEach, describe, expect, it, vi } from "vitest";
import { request } from "./client";
import { listGallery } from "./gallery";

vi.mock("./client", () => ({
  request: vi.fn(),
}));

const requestMock = vi.mocked(request);

beforeEach(() => {
  requestMock.mockReset();
  requestMock.mockResolvedValue({ items: [], total: 0, page: 1, size: 30 });
});

describe("listGallery", () => {
  it("无参数时请求 /api/gallery", async () => {
    await listGallery();

    expect(requestMock).toHaveBeenCalledWith("/api/gallery");
  });

  it("传递 page 与 size 查询参数", async () => {
    await listGallery({ page: 2, size: 30 });

    expect(requestMock).toHaveBeenCalledWith("/api/gallery?page=2&size=30");
  });

  it("只传部分参数时省略缺省项", async () => {
    await listGallery({ size: 50 });

    expect(requestMock).toHaveBeenCalledWith("/api/gallery?size=50");
  });

  it("返回解包后的 GalleryPage（items 含可选 uploader）", async () => {
    const page = {
      items: [
        {
          id: 1,
          key: "k1",
          name: "a.png",
          links: { url: "/i/1/k1.png", original: "", webp: "", thumbnail_url: "/t/1/k1.webp" },
          uploader: "alice",
        },
      ],
      total: 1,
      page: 1,
      size: 30,
    };
    requestMock.mockResolvedValue(page);

    await expect(listGallery()).resolves.toEqual(page);
    expect(requestMock).toHaveBeenCalledTimes(1);
  });
});
