import { createPinia, setActivePinia } from "pinia";
import { flushPromises } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/api/client";
import * as siteApi from "@/api/site";
import { useSiteStore } from "./site";
import type { SiteInfo } from "@/api/types";

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
    siteApiMock.fetchSite.mockResolvedValue({
      site_name: "我的图床",
      register_enabled: true,
      gallery_enabled: true,
      email_verification: false,
      password_reset: false,
      guest_upload: false,
      login_providers: [],
    });
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

  it("并发调用共用同一次在途请求", async () => {
    siteApiMock.fetchSite.mockResolvedValue({
      site_name: "ImgNest",
      register_enabled: false,
      gallery_enabled: false,
      email_verification: false,
      password_reset: false,
      guest_upload: false,
      login_providers: [],
    });
    const store = useSiteStore();

    await Promise.all([store.ensureLoaded(), store.ensureLoaded(), store.refresh()]);
    expect(siteApiMock.fetchSite).toHaveBeenCalledTimes(1);
  });

  it("成功加载后重复调用不再发起请求", async () => {
    siteApiMock.fetchSite.mockResolvedValue({
      site_name: "ImgNest",
      register_enabled: false,
      gallery_enabled: false,
      email_verification: false,
      password_reset: false,
      guest_upload: false,
      login_providers: [],
    });
    const store = useSiteStore();

    await store.ensureLoaded();
    await store.ensureLoaded();

    expect(siteApiMock.fetchSite).toHaveBeenCalledTimes(1);
  });

  it("a forced refresh publishes the saved configuration before an older read settles", async () => {
    let finishOlder!: (value: SiteInfo) => void;
    siteApiMock.fetchSite.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishOlder = resolve;
        }),
    );
    siteApiMock.fetchSite.mockResolvedValueOnce({
      site_name: "Saved site",
      register_enabled: true,
      gallery_enabled: true,
      email_verification: false,
      password_reset: false,
      guest_upload: false,
      login_providers: [],
    });
    const store = useSiteStore();
    const older = store.refresh();

    void store.refresh({ force: true });
    await flushPromises();
    expect(store.siteName).toBe("Saved site");
    finishOlder({
      site_name: "Old site",
      register_enabled: false,
      gallery_enabled: false,
      email_verification: false,
      password_reset: false,
      guest_upload: false,
      login_providers: [],
    });
    await older;
    expect(store.siteName).toBe("Saved site");
    expect(store.registerEnabled).toBe(true);
    expect(store.galleryEnabled).toBe(true);
  });

  it("settling a superseded read keeps the newer read available for deduplication", async () => {
    let finishOlder!: (value: SiteInfo) => void;
    let finishNewer!: (value: SiteInfo) => void;
    siteApiMock.fetchSite
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            finishOlder = resolve;
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            finishNewer = resolve;
          }),
      );
    const store = useSiteStore();
    const older = store.refresh();
    const newer = store.refresh({ force: true });
    await flushPromises();
    finishOlder({
      site_name: "Old site",
      register_enabled: false,
      gallery_enabled: false,
      email_verification: false,
      password_reset: false,
      guest_upload: false,
      login_providers: [],
    });
    await older;
    expect(store.loaded).toBe(false);
    const joined = store.refresh();
    await flushPromises();
    expect(siteApiMock.fetchSite).toHaveBeenCalledTimes(2);
    finishNewer({
      site_name: "Saved site",
      register_enabled: true,
      gallery_enabled: true,
      email_verification: false,
      password_reset: false,
      guest_upload: false,
      login_providers: [],
    });
    await Promise.all([newer, joined]);
    expect(store.siteName).toBe("Saved site");
    expect(store.galleryEnabled).toBe(true);
  });
});
