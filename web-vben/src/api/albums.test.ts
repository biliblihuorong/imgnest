import { beforeEach, describe, expect, it, vi } from "vitest";
import { request } from "./client";
import {
  createAlbum,
  deleteAlbum,
  deleteRandomLink,
  getRandomLink,
  listAlbums,
  putRandomLink,
  resetRandomLink,
  updateAlbum,
} from "./albums";

vi.mock("./client", () => ({
  request: vi.fn(),
}));

const requestMock = vi.mocked(request);

beforeEach(() => {
  requestMock.mockReset();
  requestMock.mockResolvedValue(null);
});

describe("listAlbums", () => {
  it("无参数时请求 /api/albums", async () => {
    await listAlbums();

    expect(requestMock).toHaveBeenCalledWith("/api/albums");
  });

  it("传递 page 与 size 查询参数", async () => {
    await listAlbums({ page: 2, size: 50 });

    expect(requestMock).toHaveBeenCalledWith("/api/albums?page=2&size=50");
  });

  it("keyword 参与查询串且做 URL 编码", async () => {
    await listAlbums({ page: 1, size: 20, keyword: "旅行" });

    expect(requestMock).toHaveBeenCalledWith(
      `/api/albums?page=1&size=20&keyword=${encodeURIComponent("旅行")}`,
    );
  });

  it("空串 keyword 视为未提供", async () => {
    await listAlbums({ keyword: "" });

    expect(requestMock).toHaveBeenCalledWith("/api/albums");
  });
});

describe("createAlbum", () => {
  it("POST JSON 体到 /api/albums（201 语义由调用方持有）", async () => {
    await createAlbum({ name: "旅行", intro: "2026 夏", is_public: true, cover_image_id: 7 });

    expect(requestMock).toHaveBeenCalledWith("/api/albums", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: '{"name":"旅行","intro":"2026 夏","is_public":true,"cover_image_id":7}',
    });
  });

  it("可选字段未提供时不出现请求体里", async () => {
    await createAlbum({ name: "默认相册" });

    expect(requestMock).toHaveBeenCalledWith("/api/albums", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: '{"name":"默认相册"}',
    });
  });
});

describe("updateAlbum", () => {
  it("PATCH JSON 体到 /api/albums/{id}（部分更新）", async () => {
    await updateAlbum(3, { name: "新名字" });

    expect(requestMock).toHaveBeenCalledWith("/api/albums/3", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: '{"name":"新名字"}',
    });
  });

  it("cover_image_id 传 0 表示清除封面", async () => {
    await updateAlbum(3, { cover_image_id: 0, is_public: false });

    expect(requestMock).toHaveBeenCalledWith("/api/albums/3", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: '{"cover_image_id":0,"is_public":false}',
    });
  });
});

describe("deleteAlbum", () => {
  it("DELETE 请求且无请求体（图片保留由后端语义保证）", async () => {
    await deleteAlbum(9);

    expect(requestMock).toHaveBeenCalledWith("/api/albums/9", { method: "DELETE" });
  });
});

import { suggestAlbums } from "./albums";
it("album suggestions use string IDs, bounded pages and an independent fixed scope", async () => {
  const signal = new AbortController().signal;
  await suggestAlbums("暑假", 2, signal, "9007199254740993");
  expect(vi.mocked(request)).toHaveBeenCalledWith(
    "/api/albums/suggestions?keyword=%E6%9A%91%E5%81%87&page=2&size=20&scope_album_id=9007199254740993",
    { signal },
  );
});

describe("random image link", () => {
  // 相册 ID 以字符串传递：路由里的十进制 ID 可能超过 JS 安全整数。
  const id = "9007199254740993";

  it("getRandomLink 读取相册的随机链接", async () => {
    await getRandomLink(id);

    expect(requestMock).toHaveBeenCalledWith(`/api/albums/${id}/random-link`);
  });

  it("putRandomLink 以 PUT 提交 enabled", async () => {
    await putRandomLink(id, true);

    expect(requestMock).toHaveBeenCalledWith(`/api/albums/${id}/random-link`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: '{"enabled":true}',
    });
  });

  it("resetRandomLink 以 POST 请求 reset", async () => {
    await resetRandomLink(id);

    expect(requestMock).toHaveBeenCalledWith(`/api/albums/${id}/random-link/reset`, {
      method: "POST",
    });
  });

  it("deleteRandomLink 以 DELETE 删除链接", async () => {
    await deleteRandomLink(id);

    expect(requestMock).toHaveBeenCalledWith(`/api/albums/${id}/random-link`, {
      method: "DELETE",
    });
  });
});
