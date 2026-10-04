import { beforeEach, describe, expect, it, vi } from "vitest";
import { request } from "./client";
import {
  deleteImage,
  getImage,
  getImageExif,
  listImages,
  listTrash,
  purgeImages,
  restoreImages,
  setImageVisibility,
} from "./images";

vi.mock("./client", () => ({
  request: vi.fn(),
}));

const requestMock = vi.mocked(request);

beforeEach(() => {
  requestMock.mockReset();
  requestMock.mockResolvedValue(null);
});

describe("listImages", () => {
  it("无参数时请求 /api/images", async () => {
    await listImages();

    expect(requestMock).toHaveBeenCalledWith("/api/images");
  });

  it("传递 page 与 size 查询参数", async () => {
    await listImages({ page: 2, size: 50 });

    expect(requestMock).toHaveBeenCalledWith("/api/images?page=2&size=50");
  });

  it("只传部分参数时省略缺省项", async () => {
    await listImages({ size: 100 });

    expect(requestMock).toHaveBeenCalledWith("/api/images?size=100");
  });
});

describe("getImage", () => {
  it("按 id 请求详情", async () => {
    await getImage(7);

    expect(requestMock).toHaveBeenCalledWith("/api/images/7");
  });
});

describe("setImageVisibility", () => {
  it("PATCH JSON 体 { is_public }", async () => {
    await setImageVisibility(7, true);

    expect(requestMock).toHaveBeenCalledWith("/api/images/7", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: '{"is_public":true}',
    });
  });

  it("私有为 is_public=false", async () => {
    await setImageVisibility(7, false);

    expect(requestMock).toHaveBeenCalledWith("/api/images/7", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: '{"is_public":false}',
    });
  });
});

describe("deleteImage", () => {
  it("DELETE 请求且无请求体", async () => {
    await deleteImage(7);

    expect(requestMock).toHaveBeenCalledWith("/api/images/7", { method: "DELETE" });
  });
});

describe("getImageExif", () => {
  it("请求 /api/images/{id}/exif", async () => {
    await getImageExif(7);

    expect(requestMock).toHaveBeenCalledWith("/api/images/7/exif");
  });
});

describe("listTrash", () => {
  it("无参数时请求 /api/trash", async () => {
    await listTrash();

    expect(requestMock).toHaveBeenCalledWith("/api/trash");
  });

  it("传递 page 与 size 查询参数", async () => {
    await listTrash({ page: 3, size: 100 });

    expect(requestMock).toHaveBeenCalledWith("/api/trash?page=3&size=100");
  });
});

describe("restoreImages", () => {
  it("POST JSON 体 { ids } 到 /api/trash/restore", async () => {
    await restoreImages([1, 2, 3]);

    expect(requestMock).toHaveBeenCalledWith("/api/trash/restore", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: '{"ids":[1,2,3]}',
    });
  });
});

describe("purgeImages", () => {
  it("POST JSON 体 { ids } 到 /api/trash/purge", async () => {
    await purgeImages([5]);

    expect(requestMock).toHaveBeenCalledWith("/api/trash/purge", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: '{"ids":[5]}',
    });
  });
});
