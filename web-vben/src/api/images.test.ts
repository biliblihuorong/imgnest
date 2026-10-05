import { beforeEach, describe, expect, it, vi } from "vitest";
import { request } from "./client";
import {
  batchAlbums,
  batchDelete,
  batchPermission,
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

describe("listImages album_id 过滤", () => {
  it("不传 album_id 时不出现查询参数（=全部）", async () => {
    await listImages({ page: 1, size: 20 });

    expect(requestMock).toHaveBeenCalledWith("/api/images?page=1&size=20");
  });

  it("显式 album_id=0 必须序列化（0=未归类）", async () => {
    await listImages({ page: 1, size: 20, album_id: 0 });

    expect(requestMock).toHaveBeenCalledWith("/api/images?page=1&size=20&album_id=0");
  });

  it("显式相册 id 参与查询串", async () => {
    await listImages({ album_id: 7 });

    expect(requestMock).toHaveBeenCalledWith("/api/images?album_id=7");
  });
});

describe("batchDelete", () => {
  it("POST {action:delete, ids} 到 /api/images/batch", async () => {
    await batchDelete([1, 2]);

    expect(requestMock).toHaveBeenCalledWith("/api/images/batch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: '{"action":"delete","ids":[1,2]}',
    });
  });
});

describe("batchPermission", () => {
  it("POST {action:permission, ids, is_public} 到 /api/images/batch", async () => {
    await batchPermission([3], true);

    expect(requestMock).toHaveBeenCalledWith("/api/images/batch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: '{"action":"permission","ids":[3],"is_public":true}',
    });
  });

  it("is_public=false 按字面量发送", async () => {
    await batchPermission([3, 4], false);

    expect(requestMock).toHaveBeenCalledWith("/api/images/batch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: '{"action":"permission","ids":[3,4],"is_public":false}',
    });
  });
});

describe("batchAlbums", () => {
  it("POST {action:album, ids, album_id} 到 /api/images/batch", async () => {
    await batchAlbums([1, 2], 5);

    expect(requestMock).toHaveBeenCalledWith("/api/images/batch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: '{"action":"album","ids":[1,2],"album_id":5}',
    });
  });

  it("album_id=0 表示移出相册且必须序列化", async () => {
    await batchAlbums([6], 0);

    expect(requestMock).toHaveBeenCalledWith("/api/images/batch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: '{"action":"album","ids":[6],"album_id":0}',
    });
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

import { searchImages } from "./images";
it("versioned empty search always sends qv,q,tz and supports abort", async () => {
  const signal = new AbortController().signal;
  await searchImages({ qv: 1, q: "", tz: "Asia/Shanghai", page: 1, size: 20 }, signal);
  expect(requestMock).toHaveBeenCalledWith(
    "/api/images?qv=1&q=&tz=Asia%2FShanghai&page=1&size=20",
    { signal },
  );
});
it("fixed album scope uses a separate endpoint and query is encoded exactly once", async () => {
  await searchImages({ qv: 1, q: '"100%_" album:#42', tz: "UTC", page: 2, size: 50 }, undefined, 7);
  const url = new URL(String(requestMock.mock.calls[0]?.[0]), "http://localhost");
  expect(url.pathname).toBe("/api/albums/7/images");
  expect(url.searchParams.get("q")).toBe('"100%_" album:#42');
  expect(url.searchParams.has("album_id")).toBe(false);
});
