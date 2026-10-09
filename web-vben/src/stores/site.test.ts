import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/api/client";
import * as siteApi from "@/api/site";
import { useSiteStore } from "./site";

vi.mock("@/api/site", () => ({
  fetchSite: vi.fn(),
}));

const siteApiMock = vi.mocked(siteApi);

describe("site store", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    setActivePinia(createPinia());
  });

  it("ensureLoaded 拉取站点名与注册开关", async () => {
    siteApiMock.fetchSite.mockResolvedValue({ site_name: "我的图床", register_enabled: true, gallery_enabled: true, login_providers: [] });
    const store = useSiteStore();

    await store.ensureLoaded();

    expect(store.siteName).toBe("我的图床");
    expect(store.registerEnabled).toBe(true);
    expect(store.galleryEnabled).toBe(true);
  });

  it("ensureLoaded 失败时保持默认值且不抛错", async () => {
    siteApiMock.fetchSite.mockRejectedValue(new ApiError(-1, "网络错误", 0));
    const store = useSiteStore();

    await expect(store.ensureLoaded()).resolves.toBeUndefined();
    expect(store.siteName).toBe("ImgNest");
    expect(store.registerEnabled).toBe(false);
  });

  it("成功加载后重复调用不再发起请求", async () => {
    siteApiMock.fetchSite.mockResolvedValue({ site_name: "ImgNest", register_enabled: false, gallery_enabled: false, login_providers: [] });
    const store = useSiteStore();

    await store.ensureLoaded();
    await store.ensureLoaded();

    expect(siteApiMock.fetchSite).toHaveBeenCalledTimes(1);
  });
});
