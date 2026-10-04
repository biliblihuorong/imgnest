import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  createGroup,
  createPolicy,
  createStorage,
  deleteAdminImage,
  deleteGroup,
  deletePolicy,
  deleteStorage,
  getSettings,
  listAdminImages,
  listGroups,
  listPolicies,
  listStorages,
  listUsers,
  patchUser,
  previewPolicy,
  purgeAllTrash,
  putSettings,
  testStorage,
  updateGroup,
  updatePolicy,
  updateStorage,
} from "./admin";
import type {
  AdminSettings,
  AdminUserView,
  GroupView,
  PolicyView,
  StorageView,
} from "./admin";
import { ApiError, request } from "./client";

vi.mock("./client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./client")>();
  return { ...actual, request: vi.fn() };
});

const requestMock = vi.mocked(request);

const userFixture: AdminUserView = {
  id: 7,
  username: "alice",
  email: "alice@example.com",
  role: "user",
  status: "enabled",
  group_id: 1,
  used_bytes: 1024,
  created_at: "2026-10-04T00:00:00Z",
};

const groupFixture: GroupView = {
  id: 1,
  name: "默认组",
  is_default: true,
  is_guest: false,
  capacity_bytes: 1073741824,
  max_file_bytes: 10485760,
  allowed_exts: ["jpg", "png"],
  upload_per_min: 20,
  default_policy_id: 1,
  policy_ids: [1],
  user_count: 3,
};

const storageFixture: StorageView = {
  id: 2,
  name: "main",
  driver: "local",
  base_url: "http://localhost:9000/imgnest",
  enabled: true,
};

const policyFixture: PolicyView = {
  id: 1,
  name: "默认规则",
  storage_id: 2,
  enabled: true,
  path_tpl: "{Y}/{m}/{d}",
  name_tpl: "{uniqid}",
  webp_mode: "both",
  webp_quality: 80,
  webp_lossless: false,
  webp_effort: 4,
  max_width: 0,
  max_height: 0,
  thumb_enabled: true,
  thumb_size: 400,
  scrub_mode: "gps",
  heif_mode: "webp_only",
  link_prefer: "webp",
  on_conflict: "rename",
  strip_meta: true,
  skip_if_larger: true,
  created_at: "2026-10-04T00:00:00Z",
  updated_at: "2026-10-04T00:00:00Z",
};

const settingsFixture: AdminSettings = {
  site_name: "ImgNest",
  registration_enabled: true,
  guest_upload_enabled: false,
  gallery_enabled: true,
  trash_days: 7,
  api_enabled: true,
  guest_group_id: 2,
  default_group_id: 1,
};

const JSON_HEADERS = { "Content-Type": "application/json" };

describe("api/admin 用户域", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it("listUsers 无参数时请求 /api/admin/users 且不带查询串", async () => {
    const page = { items: [userFixture], total: 1, page: 1, size: 20 };
    requestMock.mockResolvedValueOnce(page);

    await expect(listUsers()).resolves.toEqual(page);
    expect(requestMock).toHaveBeenCalledWith("/api/admin/users");
  });

  it("listUsers 把 page/size/keyword 组装成查询串", async () => {
    requestMock.mockResolvedValueOnce({ items: [], total: 0, page: 2, size: 50 });

    await listUsers({ page: 2, size: 50, keyword: "ab" });
    expect(requestMock).toHaveBeenCalledWith("/api/admin/users?page=2&size=50&keyword=ab");
  });

  it("patchUser 以 PATCH 提交状态与所属组并返回更新后的视图", async () => {
    const updated: AdminUserView = { ...userFixture, status: "disabled", group_id: 2 };
    requestMock.mockResolvedValueOnce(updated);

    await expect(patchUser(7, { status: "disabled", group_id: 2 })).resolves.toEqual(updated);
    expect(requestMock).toHaveBeenCalledWith("/api/admin/users/7", {
      method: "PATCH",
      headers: JSON_HEADERS,
      body: JSON.stringify({ status: "disabled", group_id: 2 }),
    });
  });

  it("listUsers 业务失败时原样抛出 ApiError", async () => {
    const error = new ApiError(20003, "需要管理员权限", 403);
    requestMock.mockRejectedValueOnce(error);

    await expect(listUsers()).rejects.toMatchObject({ code: 20003, status: 403 });
  });
});

describe("api/admin 用户组域", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it("listGroups 请求 /api/admin/groups 并返回数组", async () => {
    requestMock.mockResolvedValueOnce([groupFixture]);

    await expect(listGroups()).resolves.toEqual([groupFixture]);
    expect(requestMock).toHaveBeenCalledWith("/api/admin/groups");
  });

  it("createGroup 以 POST 提交完整请求体", async () => {
    requestMock.mockResolvedValueOnce(groupFixture);
    const body = {
      name: "默认组",
      capacity_bytes: 1073741824,
      max_file_bytes: 10485760,
      allowed_exts: ["jpg", "png"],
      upload_per_min: 20,
      default_policy_id: 1,
      policy_ids: [1],
    };

    await expect(createGroup(body)).resolves.toEqual(groupFixture);
    expect(requestMock).toHaveBeenCalledWith("/api/admin/groups", {
      method: "POST",
      headers: JSON_HEADERS,
      body: JSON.stringify(body),
    });
  });

  it("updateGroup 以 PATCH 提交部分字段", async () => {
    requestMock.mockResolvedValueOnce(groupFixture);

    await updateGroup(1, { upload_per_min: 30 });
    expect(requestMock).toHaveBeenCalledWith("/api/admin/groups/1", {
      method: "PATCH",
      headers: JSON_HEADERS,
      body: JSON.stringify({ upload_per_min: 30 }),
    });
  });

  it("deleteGroup 以 DELETE 请求；组内仍有成员时抛出 30008", async () => {
    requestMock.mockResolvedValueOnce(null);
    await expect(deleteGroup(1)).resolves.toBeNull();
    expect(requestMock).toHaveBeenCalledWith("/api/admin/groups/1", { method: "DELETE" });

    requestMock.mockRejectedValueOnce(new ApiError(30008, "用户组下仍有成员", 409));
    await expect(deleteGroup(1)).rejects.toMatchObject({ code: 30008, status: 409 });
  });
});

describe("api/admin 存储域", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it("listStorages 请求 /api/admin/storages 并返回数组", async () => {
    requestMock.mockResolvedValueOnce([storageFixture]);

    await expect(listStorages()).resolves.toEqual([storageFixture]);
    expect(requestMock).toHaveBeenCalledWith("/api/admin/storages");
  });

  it("createStorage 以 POST 提交 name/driver/base_url/config", async () => {
    requestMock.mockResolvedValueOnce(storageFixture);
    const body = { name: "main", driver: "local" as const, base_url: storageFixture.base_url };

    await expect(createStorage(body)).resolves.toEqual(storageFixture);
    expect(requestMock).toHaveBeenCalledWith("/api/admin/storages", {
      method: "POST",
      headers: JSON_HEADERS,
      body: JSON.stringify(body),
    });
  });

  it("updateStorage 以 PATCH 提交部分字段", async () => {
    requestMock.mockResolvedValueOnce(storageFixture);

    await updateStorage(2, { enabled: false });
    expect(requestMock).toHaveBeenCalledWith("/api/admin/storages/2", {
      method: "PATCH",
      headers: JSON_HEADERS,
      body: JSON.stringify({ enabled: false }),
    });
  });

  it("deleteStorage 以 DELETE 请求；被规则引用时抛出 30009", async () => {
    requestMock.mockResolvedValueOnce(null);
    await expect(deleteStorage(2)).resolves.toBeNull();
    expect(requestMock).toHaveBeenCalledWith("/api/admin/storages/2", { method: "DELETE" });

    requestMock.mockRejectedValueOnce(new ApiError(30009, "存储仍被规则引用", 409));
    await expect(deleteStorage(2)).rejects.toMatchObject({ code: 30009, status: 409 });
  });

  it("testStorage 以 POST 请求 /test 并返回逐项检查结果", async () => {
    const result = { ok: true, checks: { put: true, copy: true, delete: false } };
    requestMock.mockResolvedValueOnce(result);

    await expect(testStorage(2)).resolves.toEqual(result);
    expect(requestMock).toHaveBeenCalledWith("/api/admin/storages/2/test", { method: "POST" });
  });

  it("testStorage 网络层失败时抛出 ApiError(-1, 0)", async () => {
    requestMock.mockRejectedValue(new ApiError(-1, "网络错误", 0));

    const error = await testStorage(2).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({ code: -1, status: 0 });
  });
});

describe("api/admin 规则域", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it("listPolicies 请求 /api/admin/policies 并返回数组", async () => {
    requestMock.mockResolvedValueOnce([policyFixture]);

    await expect(listPolicies()).resolves.toEqual([policyFixture]);
    expect(requestMock).toHaveBeenCalledWith("/api/admin/policies");
  });

  it("createPolicy 以 POST 提交 name/storage_id 与可选处理参数", async () => {
    requestMock.mockResolvedValueOnce(policyFixture);
    const body = { name: "默认规则", storage_id: 2 };

    await expect(createPolicy(body)).resolves.toEqual(policyFixture);
    expect(requestMock).toHaveBeenCalledWith("/api/admin/policies", {
      method: "POST",
      headers: JSON_HEADERS,
      body: JSON.stringify(body),
    });
  });

  it("updatePolicy 以 PATCH 提交部分字段", async () => {
    requestMock.mockResolvedValueOnce(policyFixture);

    await updatePolicy(1, { webp_quality: 90, enabled: false });
    expect(requestMock).toHaveBeenCalledWith("/api/admin/policies/1", {
      method: "PATCH",
      headers: JSON_HEADERS,
      body: JSON.stringify({ webp_quality: 90, enabled: false }),
    });
  });

  it("deletePolicy 以 DELETE 请求对应资源", async () => {
    requestMock.mockResolvedValueOnce(null);

    await expect(deletePolicy(1)).resolves.toBeNull();
    expect(requestMock).toHaveBeenCalledWith("/api/admin/policies/1", { method: "DELETE" });
  });

  it("previewPolicy 以 POST 请求 /preview 并返回样例", async () => {
    const result = { sample: "2026/10/04/ab12cd34.webp", error: "" };
    requestMock.mockResolvedValueOnce(result);

    await expect(previewPolicy({ path_tpl: "{Y}/{m}/{d}", name_tpl: "{uniqid}" })).resolves.toEqual(
      result,
    );
    expect(requestMock).toHaveBeenCalledWith("/api/admin/policies/preview", {
      method: "POST",
      headers: JSON_HEADERS,
      body: JSON.stringify({ path_tpl: "{Y}/{m}/{d}", name_tpl: "{uniqid}" }),
    });
  });
});

describe("api/admin 设置与全站图片域", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it("getSettings 以 GET 请求并返回八字段设置", async () => {
    requestMock.mockResolvedValueOnce(settingsFixture);

    await expect(getSettings()).resolves.toEqual(settingsFixture);
    expect(requestMock).toHaveBeenCalledWith("/api/admin/settings");
  });

  it("putSettings 以 PUT 提交部分字段并返回保存后的完整设置", async () => {
    requestMock.mockResolvedValueOnce(settingsFixture);

    await expect(putSettings({ site_name: "新名字", trash_days: 14 })).resolves.toEqual(
      settingsFixture,
    );
    expect(requestMock).toHaveBeenCalledWith("/api/admin/settings", {
      method: "PUT",
      headers: JSON_HEADERS,
      body: JSON.stringify({ site_name: "新名字", trash_days: 14 }),
    });
  });

  it("putSettings 业务失败时原样抛出 ApiError", async () => {
    requestMock.mockRejectedValueOnce(new ApiError(10001, "参数不合法", 400));

    await expect(putSettings({ trash_days: -1 })).rejects.toMatchObject({ code: 10001 });
  });

  it("listAdminImages 把 user_id/keyword 组装成查询串", async () => {
    requestMock.mockResolvedValueOnce({ items: [], total: 0, page: 1, size: 20 });

    await listAdminImages({ user_id: 9, keyword: "cat" });
    expect(requestMock).toHaveBeenCalledWith("/api/admin/images?user_id=9&keyword=cat");
  });

  it("deleteAdminImage 以 DELETE 请求对应资源", async () => {
    requestMock.mockResolvedValueOnce(null);

    await expect(deleteAdminImage(11)).resolves.toBeNull();
    expect(requestMock).toHaveBeenCalledWith("/api/admin/images/11", { method: "DELETE" });
  });

  it("purgeAllTrash 以 POST 请求 /api/admin/trash/purge-all 并返回清理数量", async () => {
    requestMock.mockResolvedValueOnce({ purged: 42 });

    await expect(purgeAllTrash()).resolves.toEqual({ purged: 42 });
    expect(requestMock).toHaveBeenCalledWith("/api/admin/trash/purge-all", { method: "POST" });
  });
});
