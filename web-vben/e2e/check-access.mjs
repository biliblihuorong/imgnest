import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import { resolve } from "node:path";
import { parseArgs } from "node:util";

const { values } = parseArgs({
  options: {
    origin: { type: "string", default: "http://127.0.0.1:4173" },
    chromium: { type: "string" },
    output: { type: "string", default: "/tmp/imgnest-access-browser" },
  },
});
const origin = new URL(values.origin);
assert(
  ["127.0.0.1", "localhost"].includes(origin.hostname),
  "Only an isolated local frontend may use these fixtures",
);
const { chromium } = await import(process.env.IMGNEST_E2E_PLAYWRIGHT_MODULE || "playwright");
const output = resolve(values.output);
await mkdir(output, { recursive: true });
const browser = await chromium.launch({
  headless: true,
  executablePath: values.chromium,
  args: ["--no-sandbox"],
});
const failures = [];
const results = [];
const date = "2026-10-20T00:00:00Z";
const image = {
  id: 1,
  key: "fixture-key",
  name: "Public fixture.png",
  ext: "png",
  mime: "image/png",
  size: 1024,
  width: 480,
  height: 320,
  frames: 1,
  is_public: true,
  created_at: date,
  uploader: "Fixture author",
  links: {
    url: "/fixture-image.svg",
    original: "/fixture-image.svg",
    webp: "",
    thumbnail_url: "/fixture-image.svg",
  },
};
const user = {
  id: 1,
  username: "Fixture admin",
  display_name: "Fixture admin",
  email: "admin@example.test",
  role: "admin",
  status: "enabled",
  group_id: 1,
  used_bytes: 0,
  created_at: date,
  avatar_provider: "weavatar",
  avatar_url: null,
  avatar_config_version: 0,
};

function state(patch = {}) {
  return {
    settings: {
      site_name: "ImgNest",
      registration_enabled: true,
      guest_upload_enabled: false,
      guest_group_id: 1,
      gallery_enabled: false,
      gallery_public_albums_only: false,
      api_enabled: true,
      trash_days: 7,
      default_group_id: 1,
      avatar_provider: "weavatar",
      email_verification_enabled: false,
      robots_mode: "block_all",
      robots_custom: "",
      sitemap_enabled: false,
      ...patch,
    },
    guestGroupAvailable: true,
    writes: 0,
    galleryFailures: 0,
    previewFailures: 0,
    albums: [],
    randomLink: null,
  };
}

async function fixture(context, s) {
  await context.route("**/fixture-image.svg", (r) =>
    r.fulfill({
      contentType: "image/svg+xml",
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="480" height="320"><rect width="480" height="320" fill="#dde6f6"/><path d="M0 320L180 110L290 245L370 160L480 320" fill="#6389c9"/></svg>',
    }),
  );
  await context.route("**/api/**", async (route) => {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname;
    const send = (data, status = 200, code = 0) =>
      route.fulfill({
        status,
        contentType: "application/json",
        headers: { "Cache-Control": "no-store" },
        body: JSON.stringify({ code, message: code ? "fixture error" : "ok", data }),
      });
    if (path === "/api/site")
      return send({
        site_name: s.settings.site_name,
        register_enabled: s.settings.registration_enabled,
        gallery_enabled: s.settings.gallery_enabled,
        guest_upload: s.settings.guest_upload_enabled && s.guestGroupAvailable,
        email_verification: false,
        password_reset: false,
        login_providers: [],
      });
    if (path === "/api/auth/captcha")
      return send({ enabled: false, provider: "", site_key: "", version: 0 });
    if (path === "/api/gallery") {
      if (s.galleryFailures > 0) {
        s.galleryFailures--;
        return send(null, 503, 50001);
      }
      return send({
        items: s.settings.gallery_enabled ? [image] : [],
        total: s.settings.gallery_enabled ? 1 : 0,
        page: 1,
        size: Number(url.searchParams.get("size") || 30),
      });
    }
    if (path === "/api/guest/upload/limits")
      return send({ max_file_bytes: 20971520, max_files: 20, upload_per_min: 10 });
    if (path === "/api/auth/login")
      return send({ token: "fixture-session", user, expires_at: "2027-01-01T00:00:00Z" });
    if (req.headers().authorization !== "Bearer fixture-session") return send(null, 401, 20001);
    if (path === "/api/auth/me") return send(user);
    if (path === "/api/admin/settings") {
      if (req.method() === "PUT") {
        s.writes++;
        Object.assign(s.settings, req.postDataJSON());
      }
      return send(s.settings);
    }
    if (path === "/api/admin/gallery/preview") {
      if (s.previewFailures > 0) {
        s.previewFailures--;
        return send(null, 503, 50001);
      }
      const albums = url.searchParams.get("public_albums_only") === "true";
      return send({ items: [image], total: albums ? 1 : 3, page: 1, size: 12 });
    }
    if (path === "/api/admin/groups")
      return send([
        {
          id: 1,
          name: "Fixture group",
          is_default: true,
          is_guest: true,
          capacity_bytes: 0,
          max_file_bytes: 20971520,
          allowed_exts: ["png", "jpg"],
          upload_per_min: 10,
          default_policy_id: 1,
          policy_ids: [1],
          user_count: 1,
          created_at: date,
          updated_at: date,
        },
      ]);
    if (path === "/api/admin/backups")
      return send({
        items: [],
        schedule: { enabled: false, interval_hours: 24, keep: 7 },
        running: false,
        location: "data/backup",
      });
    if (path === "/api/admin/policies" || path === "/api/admin/storages") return send([]);
    if (path === "/api/admin/migrations/lsky")
      return send({
        loaded: false,
        running: false,
        phase: "idle",
        options: null,
        summary: null,
        accounts: null,
        total: 0,
        processed: 0,
        imported: 0,
        existing: 0,
        skipped: 0,
        failed: 0,
        failures: [],
        skips: [],
        started_at: null,
        finished_at: null,
        error: "",
      });
    if (path === "/api/admin/system/version")
      return send({
        current: "0.1.2",
        latest: null,
        update_available: false,
        check_enabled: false,
        checked_at: null,
        check_failed: false,
        repository_url: "https://github.com/biliblihuorong/imgnest",
        docs_url: "https://github.com/biliblihuorong/imgnest/tree/main/website/docs",
        releases_url: "https://github.com/biliblihuorong/imgnest/releases",
        issues_url: "https://github.com/biliblihuorong/imgnest/issues",
      });
    if (path === "/api/account/activity")
      return send({
        last_seen_at: null,
        last_login: null,
        logins: [],
        active_sessions: 1,
        tokens: [],
        api_usage: Array.from({ length: 30 }, (_, i) => ({
          day: new Date(Date.UTC(2026, 8, 21 + i)).toISOString().slice(0, 10),
          requests: 0,
        })),
        api_requests: 0,
      });
    if (path === "/api/albums/1/random-link") {
      if (req.method() === "PUT") {
        s.randomLink = {
          path: "/random/Ab3dE6gH9j/0123456789abcdefghijABCD",
          enabled: req.postDataJSON().enabled,
        };
      }
      return send(s.randomLink);
    }
    if (path === "/api/albums")
      return send({ items: s.albums, total: s.albums.length, page: 1, size: 30 });
    if (path === "/api/trash") return send({ items: [], total: 0, page: 1, size: 20 });
    if (path === "/api/images" || path === "/api/albums/1/images")
      return send({
        items: [],
        total: 0,
        page: 1,
        size: 20,
        search: {
          appliedVersion: 1,
          canonicalQ: url.searchParams.get("q") || "",
          tz: url.searchParams.get("tz") || "UTC",
          authorizedAlbums: path.includes("/albums/1/") ? [{ id: "1", name: "博客配图" }] : [],
          appliedRange: { afterUtc: null, beforeUtc: null },
        },
      });
    if (path === "/api/policies")
      return send([{ id: 1, name: "Fixture policy", webp_mode: "both", link_prefer: "webp" }]);
    failures.push(`Unexpected fixture API: ${req.method()} ${path}`);
    return send(null, 404, 10001);
  });
}

async function run(name, s, options, check) {
  const context = await browser.newContext({
    locale: "zh-CN",
    viewport: { width: 1440, height: 980 },
    ...options,
  });
  await fixture(context, s);
  const page = await context.newPage();
  page.setDefaultTimeout(10000);
  page.on("pageerror", (e) => failures.push(`${name}: ${e.message}`));
  try {
    await check(page);
    assert(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      `${name}: horizontal overflow`,
    );
    results.push({ name, passed: true });
  } catch (error) {
    failures.push(`${name}: ${error.stack || error}`);
    results.push({ name, passed: false });
  } finally {
    await page.screenshot({ path: `${output}/${name}.png`, fullPage: false });
    await context.close();
  }
}

try {
  for (const gallery of [false, true])
    for (const guest of [false, true]) {
      const s = state({ gallery_enabled: gallery, guest_upload_enabled: guest });
      await run(`gallery-${gallery}-guest-${guest}`, s, {}, async (page) => {
        await page.goto(origin.href, { waitUntil: "networkidle" });
        assert.equal(new URL(page.url()).pathname, gallery || guest ? "/home" : "/login");
        assert.equal(await page.locator('[data-test="guest-upload"]').isVisible(), guest);
        assert.equal(await page.locator('[data-test="gallery-preview"]').isVisible(), gallery);
        if (gallery) await page.locator(".gallery-card").waitFor();
        await page.goto(new URL("/gallery", origin).href, { waitUntil: "networkidle" });
        if (gallery) await page.locator(".gallery-card").waitFor();
        else {
          assert.equal(await page.locator(".gallery-card").count(), 0);
          assert.match(await page.locator(".gallery-empty").innerText(), /未开放|closed/i);
        }
      });
    }
  const missingGuestGroup = state({ guest_upload_enabled: true, guest_group_id: 0 });
  missingGuestGroup.guestGroupAvailable = false;
  await run("guest-group-missing", missingGuestGroup, {}, async (page) => {
    await page.goto(origin.href, { waitUntil: "networkidle" });
    assert.equal(new URL(page.url()).pathname, "/login");
  });
  const errorState = state({ gallery_enabled: true });
  errorState.galleryFailures = 1;
  await run("home-error-retry", errorState, {}, async (page) => {
    await page.goto(origin.href, { waitUntil: "networkidle" });
    await page.locator('[data-test="gallery-error"]').waitFor();
    assert.equal(await page.locator(".public-home__empty").count(), 0);
    await page.locator('[data-test="gallery-error"] button').click();
    await page.locator(".gallery-card").waitFor();
    assert.equal(await page.locator('[data-test="gallery-error"]').count(), 0);
  });
  await run(
    "mobile-login",
    state({ site_name: "ImgNest 图片托管与分享中心 · 很长的站点名称" }),
    { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true },
    async (page) => {
      await page.goto(new URL("/login", origin).href, { waitUntil: "networkidle" });
      await page.locator('input[autocomplete="email"]').fill("admin@example.test");
      await page.locator('input[autocomplete="current-password"]').fill("fixture-password");
      assert(await page.locator('button[type="submit"]').isEnabled());
      const collision = await page.evaluate(() => {
        const header = document.querySelector(".absolute.top-0.left-0");
        const toolbar = document.querySelector(".absolute.top-4.right-2");
        if (!header || !toolbar) throw new Error("Authentication header or toolbar is missing");
        const a = header.getBoundingClientRect(),
          b = toolbar.getBoundingClientRect();
        return a.right > b.left && a.left < b.right && a.bottom > b.top && a.top < b.bottom;
      });
      assert(!collision, "Mobile site name overlaps the toolbar");
    },
  );
  const adminState = state();
  adminState.previewFailures = 1;
  await run("admin-gallery-preview", adminState, {}, async (page) => {
    // Seed only the isolated fixture session; never send these credentials to a real server.
    await page.addInitScript(() => localStorage.setItem("imgnest.token", "fixture-session"));
    await page.goto(new URL("/admin/settings/access", origin).href, { waitUntil: "networkidle" });
    await page.locator("[data-gallery-preview-error]").waitFor();
    await page.locator('[data-action="retry-preview"]').click();
    await page.locator("[data-gallery-count]").filter({ hasText: "3" }).waitFor();
    await page.getByRole("switch", { name: "画廊仅展示公开相册", exact: true }).click();
    await page.locator("[data-gallery-count]").filter({ hasText: "1" }).waitFor();
    await page.locator('[data-action="toggle-preview"]').click();
    await page.locator(".gallery-card").waitFor();
    assert.equal(adminState.writes, 0, "Preview must not save settings");
    assert.equal(
      adminState.settings.gallery_enabled,
      false,
      "Preview must not publish the gallery",
    );
    await page.getByRole("switch", { name: "公共画廊", exact: true }).click();
    await page.locator('[data-action="save-settings"]').click();
    await page.waitForFunction(
      () => !document.querySelector('[data-action="save-settings"]').disabled,
    );
    assert.equal(adminState.writes, 1);
    assert.equal(adminState.settings.gallery_enabled, true);
    assert.equal(adminState.settings.gallery_public_albums_only, true);
    await page.locator("[data-gallery-count]").waitFor();
    await page.locator(".gallery-preview .n-spin").waitFor({ state: "hidden" });
  });
  for (const mobile of [false, true]) {
    const albumState = state();
    albumState.albums = [
      {
        id: 1,
        name: "博客配图",
        intro: "",
        is_public: false,
        cover_image_id: 0,
        image_count: 0,
        cover_thumb_url: "",
        created_at: date,
        updated_at: date,
      },
    ];
    await run(
      `album-random-entry-${mobile ? "mobile" : "desktop"}`,
      albumState,
      mobile ? { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true } : {},
      async (page) => {
        await page.addInitScript(() => localStorage.setItem("imgnest.token", "fixture-session"));
        await page.goto(new URL("/albums", origin).href, { waitUntil: "networkidle" });
        await page.locator(".album-card__name").filter({ hasText: "博客配图" }).click();
        await page.waitForURL((url) => url.pathname === "/albums/1");
        await page.getByRole("button", { name: "随机图片链接", exact: true }).click();
        await page.getByRole("button", { name: "启用随机链接", exact: true }).click();
        await page.getByRole("button", { name: "确认启用", exact: true }).click();
        const urlField = page.locator(".album-random-link input");
        await urlField.waitFor();
        assert.equal(await urlField.inputValue(), new URL(albumState.randomLink.path, origin).href);
        assert.equal(albumState.randomLink.enabled, true);
        assert.equal(
          albumState.settings.gallery_enabled,
          false,
          "Album sharing must not need the public gallery",
        );
        assert(await page.getByRole("button", { name: "复制链接", exact: true }).isVisible());
      },
    );
  }
  await run("recycle-bin-entry", state(), {}, async (page) => {
    await page.addInitScript(() => localStorage.setItem("imgnest.token", "fixture-session"));
    await page.goto(new URL("/images", origin).href, { waitUntil: "networkidle" });
    await page.locator(".n-tabs-tab").filter({ hasText: "回收站" }).click();
    await page.locator(".trash-view").waitFor();
    assert(await page.locator(".trash-toolbar").isVisible());
  });
  await run("backup-exclusions", state(), {}, async (page) => {
    await page.addInitScript(() => localStorage.setItem("imgnest.token", "fixture-session"));
    await page.goto(new URL("/admin/settings/backup", origin).href, { waitUntil: "networkidle" });
    await page.locator("[data-backup-exclusions]").waitFor();
    assert.match(await page.locator("[data-backup-exclusions]").innerText(), /不包含图片文件/);
  });
} finally {
  await browser.close();
  await writeFile(`${output}/results.json`, JSON.stringify({ results, failures }, null, 2) + "\n");
}
assert.equal(failures.length, 0, failures.join("\n"));
console.log(`Passed ${results.length} browser scenarios; screenshots: ${output}`);
