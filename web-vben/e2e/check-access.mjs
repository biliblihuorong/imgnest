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
        billing_enabled: Boolean(s.billingEnabled),
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
      return send({ token: "fixture-session", user: s.user || user, expires_at: "2027-01-01T00:00:00Z" });
    if (req.headers().authorization !== "Bearer fixture-session") return send(null, 401, 20001);
    if (path === "/api/auth/me") return send(s.user || user);
    if (path.startsWith("/api/user/billing") || path.startsWith("/api/admin/billing")) {
      if (!s.billingEnabled) return send(null,404,10001);
      s.billingBalance ??= 10000;
      s.billingReceipts ??= new Map();
      s.billingEntries ??= [];
      const owner = s.billingOwner || (s.user || user).id;
      const wallet = {user_id:owner,username:"Fixture buyer",balance_cents:s.billingBalance,version:s.billingEntries.length,updated_at:date};
      const plan = {id:1,name:"标准套餐",description:"网站图片上传与外链使用",price_cents:3000,duration_days:30,group_id:1,published:true,archived:false,version:1,created_at:date,updated_at:date,group:{name:"Fixture group",capacity_bytes:1073741824,max_file_bytes:20971520,upload_per_min:30}};
      const page = items => ({items,total:items.length,page:1,size:20});
      if(path.endsWith("/plans")) return send([plan]);
      if(path === "/api/user/billing") return send({currency:"CNY",wallet,membership:s.billingMembership||null});
      if(path.endsWith("/ledger")) return send(page(s.billingEntries));
      if(path.endsWith("/wallets")) return send(page([wallet]));
      if(path.endsWith("/memberships")) return send(page(s.billingMembership?[s.billingMembership]:[]));
      if(path.endsWith("/orders")) return send(page([...s.billingReceipts.values()].filter(order=>!url.searchParams.get("request_id")||order.request_id===url.searchParams.get("request_id"))));
      if(path.endsWith("/purchases")) {
        const input=req.postDataJSON();s.billingAttempts=(s.billingAttempts||0)+1;
        const previous=s.billingReceipts.get(input.request_id);if(previous)return send(previous);
        const order={currency:"CNY",id:s.billingReceipts.size+1,user_id:owner,username:"Fixture buyer",plan_id:1,plan_name:plan.name,plan_version:1,price_cents:3000,duration_days:30,group_id:1,previous_expires_at:null,expires_at:"2026-11-22T00:00:00Z",requires_login:false,request_id:input.request_id,created_at:date};
        s.billingReceipts.set(input.request_id,order);s.billingBalance-=3000;s.billingWrites=(s.billingWrites||0)+1;
        s.billingMembership={user_id:owner,plan_id:1,group_id:1,base_group_id:1,starts_at:date,expires_at:order.expires_at,status:"active",updated_at:date};
        if(s.failFirstPurchase){s.failFirstPurchase=false;return route.abort("failed")}
        return send(order);
      }
      if(path.endsWith("/adjustments")) {
        const input=req.postDataJSON();const before=s.billingBalance;s.billingBalance+=input.amount_cents;s.billingWrites=(s.billingWrites||0)+1;
        const entry={id:s.billingEntries.length+1,user_id:input.user_id,username:"Fixture buyer",sequence:s.billingEntries.length+1,amount_cents:input.amount_cents,before_cents:before,after_cents:s.billingBalance,kind:input.amount_cents>0?"admin_credit":"admin_debit",reason:input.reason,actor_id:1,actor_name:"Fixture admin",order_id:null,reverses_id:null,created_at:date};s.billingEntries.unshift(entry);return send(entry);
      }
    }
    if (path === "/api/user/plugins") return send(s.userPlugin ? [s.userPlugin] : []);
    if (path === "/api/user/plugins/example/settings") {
      s.userPlugin.values = req.postDataJSON().values;
      return send(s.userPlugin);
    }
    if (path === "/api/user/plugins/example/asset") {
      if (req.method() === "GET")
        return route.fulfill({
          contentType: "image/png",
          body: Buffer.from(
            "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aCHsAAAAASUVORK5CYII=",
            "base64",
          ),
        });
      s.userPlugin.has_asset = req.method() !== "DELETE";
      return send(s.userPlugin);
    }
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
          is_guest: !s.billingEnabled,
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
  for(const mobile of [false,true]) {
    const account=state();account.billingEnabled=true;account.user={...user,role:"user"};account.failFirstPurchase=true;
    await run(`billing-purchase-${mobile?"mobile":"desktop"}`,account,mobile?{viewport:{width:390,height:844},isMobile:true,hasTouch:true}:{},async page=>{
      await page.addInitScript(()=>localStorage.setItem("imgnest.token","fixture-session"));
      await page.goto(new URL("/account/billing",origin).href,{waitUntil:"networkidle"});
      await page.getByRole("link",{name:"余额与套餐",exact:true}).waitFor();
      await page.locator('[data-buy-plan="1"]').click();await page.locator('[data-confirm-purchase]').click();
      await page.getByText("上次请求结果尚未确认",{exact:false}).waitFor();
      await page.locator('[data-confirm-purchase]').click();await page.getByText("购买成功，订单 #1。",{exact:true}).waitFor();
      assert.equal(account.billingAttempts,2);assert.equal(account.billingWrites,1);assert.equal(account.billingBalance,7000);
      assert(await page.locator('.account-billing > .n-card').evaluate(el=>el.getBoundingClientRect().right<=innerWidth),"wallet card must fit the viewport without clipped content");
    });
  }
  const recovery=state();recovery.billingEnabled=true;recovery.user={...user,role:"user"};recovery.failFirstPurchase=true;
  await run("billing-purchase-refresh-recovery",recovery,{},async page=>{
    await page.addInitScript(()=>localStorage.setItem("imgnest.token","fixture-session"));
    await page.goto(new URL("/account/billing",origin).href,{waitUntil:"networkidle"});await page.locator('[data-buy-plan="1"]').click();await page.locator('[data-confirm-purchase]').click();await page.getByText("上次请求结果尚未确认",{exact:false}).waitFor();
    await page.reload({waitUntil:"networkidle"});await page.getByText("购买成功，订单 #1。",{exact:true}).waitFor();
    assert.equal(recovery.billingAttempts,1);assert.equal(recovery.billingBalance,7000);
  });
  const billingAdmin=state();billingAdmin.billingEnabled=true;billingAdmin.billingOwner=2;
  await run("billing-administrator-credit",billingAdmin,{},async page=>{
    await page.addInitScript(()=>localStorage.setItem("imgnest.token","fixture-session"));await page.goto(new URL("/admin/billing",origin).href,{waitUntil:"networkidle"});
    await page.getByRole("heading",{name:"套餐与余额管理",exact:true}).waitFor();await page.locator('[data-adjust-user]').fill("2");await page.locator('[data-adjust-amount]').fill("5.25");await page.locator('[data-adjust-reason]').fill("人工确认入账");await page.locator('[data-review-adjustment]').click();
    assert.equal(billingAdmin.billingWrites||0,0);await page.locator('[data-confirm-adjustment]').click();await page.getByText("记账成功，流水 #1。",{exact:true}).waitFor();assert.equal(billingAdmin.billingWrites,1);assert.equal(billingAdmin.billingBalance,10525);
  });
  await run("billing-ordinary-edition",state(),{},async page=>{
    await page.addInitScript(()=>localStorage.setItem("imgnest.token","fixture-session"));await page.goto(new URL("/account/settings",origin).href,{waitUntil:"networkidle"});assert.equal(await page.locator('a[href="/account/billing"]').count(),0);assert.equal(await page.locator('a[href="/admin/billing"]').count(),0);
    await page.goto(new URL("/account/billing",origin).href,{waitUntil:"networkidle"});await page.getByText("余额与套餐当前不可用",{exact:true}).waitFor();assert.equal(await page.locator('[data-buy-plan]').count(),0);
  });
  const accountExtension = () => ({
    name: "example",
    title: "我的水印",
    description: "自定义文字与私有 Logo。",
    fields: [{ key: "text", type: "text", label: "自定义文字", default: "" }],
    values: { text: "Alice" },
    read_only: false,
    has_asset: false,
    asset_label: "Logo",
  });
  for (const mobile of [false, true]) {
    const personal = state();
    personal.userPlugin = accountExtension();
    await run(
      `account-extension-${mobile ? "mobile" : "desktop"}`,
      personal,
      mobile ? { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true } : {},
      async (page) => {
        await page.addInitScript(() => localStorage.setItem("imgnest.token", "fixture-session"));
        await page.goto(new URL("/account/settings", origin).href, { waitUntil: "networkidle" });
        const card = page.locator('[data-user-plugin="example"]');
        assert.equal(await card.count(), 0, "Profile page should not contain personal forms");
        await page.getByRole("link", { name: "我的水印", exact: true }).click();
        await page.waitForURL((url) => url.pathname === "/account/plugins/example");
        await card.waitFor();
        await card.getByLabel("自定义文字", { exact: true }).fill("我的文字");
        await Promise.all([
          page.waitForResponse(
            (response) => response.url().endsWith("/example/settings") && response.status() === 200,
          ),
          card.locator('[data-action="save-user-plugin"]').click(),
        ]);
        assert.equal(personal.userPlugin.values.text, "我的文字");
        await page.reload({ waitUntil: "networkidle" });
        assert.equal(await card.getByLabel("自定义文字", { exact: true }).inputValue(), "我的文字");
        await card.locator('input[type="file"]').setInputFiles({
          name: "logo.png",
          mimeType: "image/png",
          buffer: Buffer.from("fixture-logo"),
        });
        await card.locator("img").waitFor();
        assert((await card.locator("img").getAttribute("src")).startsWith("blob:"));
        await card.getByRole("button", { name: "移除 Logo", exact: true }).click();
        await card.locator("img").waitFor({ state: "detached" });
        await card.scrollIntoViewIfNeeded();
      },
    );
  }
  const controlled = state();
  controlled.userPlugin = {
    ...accountExtension(),
    read_only: true,
    notice: "当前组由管理员统一控制",
  };
  await run("account-extension-group-controlled", controlled, {}, async (page) => {
    await page.addInitScript(() => localStorage.setItem("imgnest.token", "fixture-session"));
    await page.goto(new URL("/account/plugins/example", origin).href, { waitUntil: "networkidle" });
    const card = page.locator('[data-user-plugin="example"]');
    await card.waitFor();
    assert(await card.getByLabel("自定义文字", { exact: true }).isDisabled());
    assert.equal(await card.locator('input[type="file"]').count(), 0);
    assert.equal(await card.locator('[data-action="save-user-plugin"]').count(), 0);
    await card.scrollIntoViewIfNeeded();
  });
  await run("account-extension-ordinary-edition", state(), {}, async (page) => {
    await page.addInitScript(() => localStorage.setItem("imgnest.token", "fixture-session"));
    await page.goto(new URL("/account/settings", origin).href, { waitUntil: "networkidle" });
    assert.equal(await page.locator("[data-user-plugin]").count(), 0);
    assert.equal(await page.getByRole("link", { name: "我的水印", exact: true }).count(), 0);
    await page.goto(new URL("/account/plugins/example", origin).href, { waitUntil: "networkidle" });
    assert.equal(await page.locator("[data-user-plugin]").count(), 0);
    assert.match(await page.locator(".n-empty").innerText(), /此设置当前不可用/);
  });
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
