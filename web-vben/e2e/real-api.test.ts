import { readFileSync } from "node:fs";
import { beforeAll, beforeEach, describe, expect, it } from "vitest";
import * as admin from "../src/api/admin";
import {
  createAlbum,
  deleteAlbum,
  listAlbums,
  suggestAlbums,
  updateAlbum,
} from "../src/api/albums";
import { changePassword, login, logout, me, register } from "../src/api/auth";
import { fetchCaptcha, getCaptchaSettings } from "../src/api/captcha";
import { ApiError, request, setUnauthorizedHandler, TOKEN_STORAGE_KEY } from "../src/api/client";
import { fetchDashboardMetric } from "../src/api/dashboard";
import { listGallery } from "../src/api/gallery";
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
  searchImages,
  setImageVisibility,
} from "../src/api/images";
import type { ImageView } from "../src/api/images";
import { listPolicies } from "../src/api/policies";
import { fetchSite } from "../src/api/site";
import { fetchProtectedThumbnail } from "../src/api/thumbnails";
import { createToken, listTokens, revokeToken } from "../src/api/tokens";
import { uploadImages } from "../src/api/upload";
import { fixture, sensitiveValues } from "./real-api.setup";

const missingID = 999_999;
let adminToken = "";
let userToken = "";
let userID = 0;
let adminID = 0;
let policyID = 0;
let albumA = 0;
let albumB = 0;
let imageA: ImageView;
let imageB: ImageView;
let chargedBytes = 0;
let unauthorized = 0;

function useToken(value?: string) {
  if (value) localStorage.setItem(TOKEN_STORAGE_KEY, value);
  else localStorage.removeItem(TOKEN_STORAGE_KEY);
}

async function expectApiError(operation: Promise<unknown>, status: number, code: number) {
  const error: unknown = await operation.catch((value: unknown) => value);
  expect(error instanceof ApiError).toBe(true);
  if (!(error instanceof ApiError)) throw new Error("Expected the real API error mapping");
  expect({ status: error.status, code: error.code }).toEqual({ status, code });
}

function expectSafe(value: unknown) {
  const forbidden = new Set([
    "password",
    "password_hash",
    "token",
    "token_hash",
    "config",
    "secret",
    "exif",
    "gps_lat",
    "gps_lng",
    "raw",
    "ip",
    "registered_ip",
  ]);
  function visit(item: unknown) {
    if (!item || typeof item !== "object") return;
    for (const [key, child] of Object.entries(item)) {
      expect(forbidden.has(key), `unexpected private field: ${key}`).toBe(false);
      visit(child);
    }
  }
  visit(value);
}

async function albumCount(id: number) {
  return (await listAlbums()).items.find((album) => album.id === id)?.image_count;
}

// One ordered journey against one real, disposable database. Bail on the first
// failed stage rather than presenting dependent-stage failures as new defects.
describe("frontend API modules → real Go HTTP → SQLite/local storage", () => {
  beforeAll(async () => {
    expect(await fetchCaptcha()).toEqual({
      enabled: false,
      provider: "",
      site_key: "",
      version: 0,
    });
    const adminLogin = await login(fixture.adminEmail, fixture.adminPassword);
    adminToken = adminLogin.token;
    adminID = adminLogin.user.id;
    sensitiveValues.push(adminToken);
    useToken(adminToken);
    await admin.putSettings({ registration_enabled: true, gallery_enabled: true });
    const storage = (await admin.listStorages())[0];
    if (!storage) throw new Error("CLI did not provision local storage");
    await admin.updateStorage(storage.id, { base_url: `${fixture.origin}/i/${storage.id}` });
    const registered = await register({
      username: `member-${fixture.nonce}`,
      email: fixture.userEmail,
      password: fixture.userPassword,
    });
    expectSafe(registered);
    expect(registered.role).toBe("user");
    userID = registered.id;
    useToken();
    const userLogin = await login(fixture.userEmail, fixture.userPassword);
    userToken = userLogin.token;
    sensitiveValues.push(userToken);
    expect(typeof userToken === "string" && userToken.length > 20).toBe(true);
    expect(Date.parse(userLogin.expires_at) > Date.now()).toBe(true);
    useToken(userToken);
    const policy = (await listPolicies())[0];
    if (!policy) throw new Error("Member group has no usable local policy");
    policyID = policy.id;
  });

  beforeEach(() => {
    unauthorized = 0;
    setUnauthorizedHandler(() => {
      unauthorized += 1;
    });
    useToken(userToken);
  });

  it("logs in with disabled CAPTCHA and reads safe identities through persisted Bearer tokens", async () => {
    expect(await fetchCaptcha()).toEqual({
      enabled: false,
      provider: "",
      site_key: "",
      version: 0,
    });
    const member = await me();
    expect(member).toMatchObject({ id: userID, role: "user", used_bytes: 0 });
    expectSafe(member);
    useToken(adminToken);
    expect(await me()).toMatchObject({ id: adminID, role: "admin" });
    expect(await fetchDashboardMetric("accounts", { id: adminID, role: "admin" })).toBe(2);
    useToken();
    expect(await fetchSite()).toMatchObject({ register_enabled: true, gallery_enabled: true });
  });

  it("maps real 400/401/404 and non-JSON failures without clearing a valid session on 20002", async () => {
    await expectApiError(login(fixture.userEmail, `${fixture.userPassword}-wrong`), 401, 20002);
    // The real native limit is three attempts per IP/minute. Existing sessions
    // continue to work after a fourth attempt is rejected; no limit is disabled.
    await expectApiError(login(fixture.userEmail, fixture.userPassword), 429, 30003);
    await expectApiError(
      changePassword(`${fixture.userPassword}-wrong`, fixture.nextPassword),
      401,
      20002,
    );
    expect(unauthorized).toBe(0);
    expect((await me()).id).toBe(userID);
    await expectApiError(listImages({ page: 0 }), 400, 10001);
    await expectApiError(request("/api/no-such-real-api-test-route"), 404, 10001);
    await expectApiError(request("/"), 0, -1);
    useToken();
    await expectApiError(me(), 401, 20001);
    expect(unauthorized).toBe(1);
  });

  it("rejects every administrative domain for an ordinary member without invalidating their token", async () => {
    const operations = [
      () => admin.listUsers(),
      () => admin.listGroups(),
      () => admin.listStorages(),
      () => admin.listPolicies(),
      () => admin.getSettings(),
      () => admin.listAdminImages(),
      () => getCaptchaSettings(),
      () => admin.putSettings({ gallery_enabled: false }),
    ];
    for (const operation of operations) await expectApiError(operation(), 403, 20003);
    expect(unauthorized).toBe(0);
    expect((await me()).id).toBe(userID);
    expect((await fetchSite()).gallery_enabled).toBe(true);
  });

  it("returns API-token plaintext only at creation, isolates ownership, and revokes immediately", async () => {
    const issued = await createToken({ name: `api-${fixture.nonce}`, expires_at: null });
    sensitiveValues.push(issued.token);
    expect(typeof issued.token === "string" && issued.token.length > 20).toBe(true);
    expect(issued.info).toMatchObject({ kind: "api", abilities: ["*"], expires_at: null });
    const listed = await listTokens();
    expect(listed.some((item) => item.id === issued.info.id)).toBe(true);
    expectSafe(listed);
    expect(JSON.stringify(listed).includes(issued.token)).toBe(false);
    useToken(issued.token);
    expect((await me()).id).toBe(userID);
    useToken(userToken);
    expect(await revokeToken(issued.info.id)).toBeNull();
    expect((await listTokens()).some((item) => item.id === issued.info.id)).toBe(false);
    useToken(issued.token);
    await expectApiError(me(), 401, 20001);
    expect(unauthorized).toBe(1);
    useToken(adminToken);
    const foreign = await createToken({ name: `foreign-${fixture.nonce}` });
    sensitiveValues.push(foreign.token);
    useToken(userToken);
    await expectApiError(revokeToken(foreign.info.id), 403, 20003);
    await expectApiError(revokeToken(missingID), 403, 20003);
    useToken(foreign.token);
    expect((await me()).id).toBe(adminID);
    useToken(adminToken);
    await revokeToken(foreign.info.id);
  });

  it("creates and patches real albums, including cover_image_id=0 and encoded keyword filters", async () => {
    const first = await createAlbum({ name: `相册 & ${fixture.nonce}`, cover_image_id: 0 });
    albumA = first.id;
    albumB = (await createAlbum({ name: `destination-${fixture.nonce}` })).id;
    expect(first).toMatchObject({ image_count: 0, cover_image_id: 0, cover_thumb_url: "" });
    expect(await updateAlbum(albumA, { intro: "真实 HTTP 契约", cover_image_id: 0 })).toMatchObject(
      {
        intro: "真实 HTTP 契约",
        cover_image_id: 0,
      },
    );
    const found = await listAlbums({ page: 1, size: 1, keyword: "相册 &" });
    expect(found).toMatchObject({ total: 1, page: 1, size: 1 });
    expect(found.items[0]?.id).toBe(albumA);
    await expectApiError(createAlbum({ name: " " }), 400, 10001);
    useToken(adminToken);
    await expectApiError(updateAlbum(albumA, { name: "foreign change" }), 403, 20003);
  });

  it("uploads through the real frontend XHR module and continues after an invalid image", async () => {
    const files = [
      new File(["not an image"], `invalid-${fixture.nonce}.png`, { type: "image/png" }),
      ...fixture.images.map(
        (path, index) =>
          new File([readFileSync(path)], `real-${index}-${fixture.nonce}.png`, {
            type: "image/png",
          }),
      ),
    ];
    const started: number[] = [];
    const settled: number[] = [];
    const results = await uploadImages({
      files,
      policyId: policyID,
      isPublic: false,
      albumId: albumA,
      onFileStart: (index) => {
        started.push(index);
      },
      onFileSettled: (index) => {
        settled.push(index);
      },
    });
    expect(started).toEqual([0, 1, 2]);
    expect(settled).toEqual([0, 1, 2]);
    expect(results[0]).toMatchObject({ ok: false, status: 422, code: 50003 });
    expect(results.slice(1).map((result) => ({ ok: result.ok, status: result.status }))).toEqual([
      { ok: true, status: 201 },
      { ok: true, status: 201 },
    ]);
    const first = results[1];
    const second = results[2];
    if (!first?.ok || !second?.ok) throw new Error("Valid image upload failed");
    imageA = first.image;
    imageB = second.image;
    expect(imageA).toMatchObject({
      user_id: userID,
      album_id: albumA,
      width: 8,
      height: 6,
      ext: "png",
      mime: "image/png",
      is_public: false,
    });
    expectSafe(imageA);
    expect(imageA.has_thumb).toBe(true);
    expect(imageA.has_original).toBe(true);
    expect((await listImages({ size: 1 })).total).toBe(2);
    expect(await albumCount(albumA)).toBe(2);
    chargedBytes = imageA.charged_bytes + imageB.charged_bytes;
    expect((await me()).used_bytes).toBe(chargedBytes);
    expect(await fetchDashboardMetric("images", { id: userID, role: "user" })).toBe(2);
    expect(await fetchDashboardMetric("storage", { id: userID, role: "user" })).toBe(chargedBytes);
    expect((await getImageExif(imageA.id)).raw).toBeTypeOf("object");
    const preview = await fetchProtectedThumbnail(imageA.local_thumb_url);
    expect(preview.type).toBe("image/webp");
    expect(preview.size > 0).toBe(true);
  });

  it("executes versioned search and scoped suggestions through the real frontend API modules", async () => {
    const query = {
      qv: 1 as const,
      q: `extension:png is:private album:#${albumA}`,
      tz: "UTC",
      page: 1,
      size: 20,
    };
    const results = await searchImages(query);
    expect(results.total).toBe(2);
    expect(new Set(results.items.map(({ id }) => id))).toEqual(new Set([imageA.id, imageB.id]));
    expect(results.search).toMatchObject({
      appliedVersion: 1,
      canonicalQ: `format:png album:#${albumA} visibility:private`,
      tz: "UTC",
      authorizedAlbums: [{ id: String(albumA) }],
      appliedRange: { afterUtc: null, beforeUtc: null },
    });
    expectSafe(results);
    const one = await searchImages({ ...query, q: `real-0-${fixture.nonce} format:png` });
    expect(one.total).toBe(1);
    expect(one.items.map(({ id }) => id)).toEqual([imageA.id]);
    const scoped = await searchImages({ ...query, q: "" }, undefined, String(albumA));
    expect(scoped.total).toBe(2);
    const empty = await searchImages({ ...query, q: "" }, undefined, String(albumB));
    expect(empty).toMatchObject({ items: [], total: 0 });
    const suggestions = await suggestAlbums("", 1, undefined, String(albumA));
    expect(suggestions.items.map(({ id }) => id)).toEqual([String(albumA)]);
    expect(suggestions.hasMore).toBe(false);
    expectSafe(suggestions);
    const unavailable = await searchImages(
      { ...query, q: `album:#${albumB}` },
      undefined,
      String(albumA),
    ).catch((error: unknown) => error);
    expect(unavailable).toBeInstanceOf(ApiError);
    if (!(unavailable instanceof ApiError)) throw new Error("Expected scoped search error");
    expect(unavailable.status).toBe(422);
    expect(unavailable.data).toMatchObject({ diagnostics: [{ code: "ALBUM_NOT_AVAILABLE" }] });
    const invalid = await searchImages({ ...query, q: "format:" }).catch((error: unknown) => error);
    expect(invalid).toBeInstanceOf(ApiError);
    if (!(invalid instanceof ApiError)) throw new Error("Expected syntax error");
    expect(invalid.status).toBe(400);
    expect(invalid.data).toMatchObject({
      diagnostics: [{ code: "MISSING_VALUE", span: { start: 7, end: 7 } }],
    });
    useToken(adminToken);
    await expectApiError(searchImages({ ...query, q: `album:#${albumA}` }), 422, 10001);
  });

  it("moves images with mixed 207 results, clears covers, and preserves images when deleting albums", async () => {
    const moved = await batchAlbums([imageA.id, missingID], albumB);
    expect(moved.map(({ id, status, code }) => ({ id, status, code }))).toEqual([
      { id: imageA.id, status: 200, code: 0 },
      { id: missingID, status: 404, code: 10001 },
    ]);
    expect(await albumCount(albumA)).toBe(1);
    expect(await albumCount(albumB)).toBe(1);
    expect((await listImages({ album_id: 0 })).total).toBe(0);
    expect((await listImages({ album_id: albumB })).items.map(({ id }) => id)).toEqual([imageA.id]);
    const covered = await updateAlbum(albumB, { cover_image_id: imageA.id });
    expect(covered.cover_thumb_url).toBe(imageA.local_thumb_url);
    expect(await updateAlbum(albumB, { cover_image_id: 0 })).toMatchObject({
      cover_image_id: 0,
      cover_thumb_url: "",
    });
    expect(await deleteAlbum(albumA)).toBeNull();
    expect((await getImage(imageB.id)).album_id).toBe(0);
    expect((await listImages({ album_id: 0 })).items.map(({ id }) => id)).toEqual([imageB.id]);
    expect((await listImages()).total).toBe(2);
  });

  it("keeps private images out of anonymous gallery while preserving safe public direct links", async () => {
    expect((await listGallery()).total).toBe(0);
    expect((await setImageVisibility(imageA.id, true)).is_public).toBe(true);
    expect((await batchPermission([imageB.id], false))[0]).toMatchObject({
      id: imageB.id,
      code: 0,
      status: 200,
    });
    useToken();
    const gallery = await listGallery({ size: 1 });
    expect(gallery.total).toBe(1);
    expect(gallery.items[0]?.id).toBe(imageA.id);
    expectSafe(gallery);
    const privateDirect = await fetch(imageB.links.original);
    expect(privateDirect.status).toBe(200);
    expect(new Uint8Array(await privateDirect.arrayBuffer())).toEqual(
      new Uint8Array(readFileSync(fixture.images[1]!)),
    );
    await expectApiError(getImageExif(imageA.id), 401, 20001);
    expect((await fetchProtectedThumbnail(imageA.local_thumb_url)).size > 0).toBe(true);
    await expectApiError(fetchProtectedThumbnail(imageB.local_thumb_url), 401, 20001);
  });

  it("deletes and restores through real recycle APIs, reconciling bytes, album counts, and object URLs", async () => {
    expect(await deleteImage(imageA.id)).toBeNull();
    expect((await listImages()).total).toBe(1);
    expect(await albumCount(albumB)).toBe(0);
    expect((await me()).used_bytes).toBe(chargedBytes - imageA.charged_bytes);
    const trash = await listTrash();
    expect(trash.total).toBe(1);
    expect(trash.items[0]?.id).toBe(imageA.id);
    expect(Boolean(trash.items[0]?.deleted_at && trash.items[0]?.purge_at)).toBe(true);
    expect(await fetchDashboardMetric("trash", { id: userID, role: "user" })).toBe(1);
    expect((await fetch(imageA.links.original)).status).toBe(404);
    const restored = await restoreImages([imageA.id, missingID]);
    expect(restored.map(({ id, code, status }) => ({ id, code, status }))).toEqual([
      { id: imageA.id, code: 0, status: 200 },
      { id: missingID, code: 10001, status: 404 },
    ]);
    expect((await listTrash()).total).toBe(0);
    expect(await albumCount(albumB)).toBe(1);
    expect((await me()).used_bytes).toBe(chargedBytes);
    expect((await getImage(imageA.id)).deleted_at).toBeNull();
    const restoredObject = await fetch(imageA.links.original);
    expect(restoredObject.status).toBe(200);
    expect(new Uint8Array(await restoredObject.arrayBuffer())).toEqual(
      new Uint8Array(readFileSync(fixture.images[0]!)),
    );
  });

  it("purges only generated fixtures and leaves real storage/database accounting empty", async () => {
    await deleteAlbum(albumB);
    expect((await getImage(imageA.id)).album_id).toBe(0);
    const deleted = await batchDelete([imageA.id, imageB.id, missingID]);
    expect(deleted.map(({ code }) => code)).toEqual([0, 0, 10001]);
    const purged = await purgeImages([imageA.id, imageB.id]);
    expect(purged.map(({ code, status }) => ({ code, status }))).toEqual([
      { code: 0, status: 200 },
      { code: 0, status: 200 },
    ]);
    expect((await listTrash()).total).toBe(0);
    expect((await listImages()).total).toBe(0);
    expect((await listAlbums()).total).toBe(0);
    expect((await me()).used_bytes).toBe(0);
    expect((await fetch(imageA.links.original)).status).toBe(404);
    await expectApiError(fetchProtectedThumbnail(imageA.local_thumb_url), 404, 10001);
  });

  it("revokes the web session on logout and every remaining member token on password change", async () => {
    const spare = await createToken({ name: `password-change-${fixture.nonce}` });
    sensitiveValues.push(spare.token);
    expect(await logout()).toBeNull();
    await expectApiError(me(), 401, 20001);
    useToken(spare.token);
    expect((await me()).id).toBe(userID);
    expect(await changePassword(fixture.userPassword, fixture.nextPassword)).toBeNull();
    await expectApiError(me(), 401, 20001);
    useToken(userToken);
    await expectApiError(me(), 401, 20001);
    useToken(adminToken);
    expect((await me()).id).toBe(adminID);
    expect(await fetchDashboardMetric("globalImages", { id: adminID, role: "admin" })).toBe(0);
    expect(await fetchDashboardMetric("accounts", { id: adminID, role: "admin" })).toBe(2);
    setUnauthorizedHandler(null);
  });

  it("creates and edits an isolated account through the actual administrator API wrappers", async () => {
    useToken(adminToken);
    const group = (await admin.listGroups()).find(({ is_guest }) => !is_guest);
    if (!group) throw new Error("Missing fixture account group");
    const created = await admin.createUser({
      username: `created-${fixture.nonce}`,
      email: `created-${fixture.nonce}@imgnest.invalid`,
      password: fixture.nextPassword,
      display_name: "测试昵称",
      role: "user",
      status: "disabled",
      group_id: group.id,
    });
    expectSafe(created);
    expect(created).toMatchObject({
      display_name: "测试昵称",
      role: "user",
      status: "disabled",
      group_id: group.id,
    });
    const updated = await admin.patchUser(created.id, {
      username: `edited-${fixture.nonce}`,
      email: `edited-${fixture.nonce}@imgnest.invalid`,
      display_name: "修改后的昵称",
      role: "user",
      status: "enabled",
      group_id: group.id,
    });
    expectSafe(updated);
    expect(updated).toMatchObject({
      username: `edited-${fixture.nonce}`,
      email: `edited-${fixture.nonce}@imgnest.invalid`,
      display_name: "修改后的昵称",
      status: "enabled",
    });
    expect(
      (await admin.listUsers({ keyword: updated.username })).items.map(({ id }) => id),
    ).toEqual([created.id]);
    await expectApiError(admin.patchUser(created.id, { email: fixture.adminEmail }), 409, 30002);
    expect((await admin.listUsers({ keyword: updated.username })).items[0]?.email).toBe(
      updated.email,
    );
    await expectApiError(admin.patchUser(adminID, { status: "disabled" }), 403, 20003);
    expect((await me()).id).toBe(adminID);
  });
});
