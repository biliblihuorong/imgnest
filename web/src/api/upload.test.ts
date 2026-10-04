import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setUnauthorizedHandler, TOKEN_STORAGE_KEY } from "./client";
import {
  MAX_UPLOAD_FILES,
  MAX_UPLOAD_FILE_BYTES,
  MAX_UPLOAD_TOTAL_BYTES,
  preflightUpload,
  uploadImages,
  type ImageView,
} from "./upload";

/**
 * 最小 XHR 桩：记录请求细节，测试里手动触发进度 / 响应 / 网络错误。
 */
class FakeXHR {
  static instances: FakeXHR[] = [];

  status = 0;
  responseText = "";
  onprogress: ((event: { loaded: number; total: number; lengthComputable: boolean }) => void) | null =
    null;
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  readonly upload = { onprogress: null as FakeXHR["onprogress"] };
  readonly headers = new Map<string, string>();
  private sentBody: FormData | null = null;

  constructor() {
    FakeXHR.instances.push(this);
  }

  open(): void {}

  setRequestHeader(name: string, value: string): void {
    this.headers.set(name, value);
  }

  send(body: FormData): void {
    this.sentBody = body;
  }

  get sent(): FormData | null {
    return this.sentBody;
  }

  respond(status: number, body: string): void {
    this.status = status;
    this.responseText = body;
    this.onload?.();
  }

  failNetwork(): void {
    this.onerror?.();
  }
}

function makeFile(name: string, size: number): File {
  const file = new File(["x"], name, { type: "image/png" });
  Object.defineProperty(file, "size", { value: size });
  return file;
}

const imageView: ImageView = {
  id: 7,
  key: "2026/10/a.png",
  user_id: 1,
  album_id: 0,
  policy_id: 1,
  storage_id: 1,
  name: "a.png",
  ext: "png",
  mime: "image/png",
  size: 1024,
  webp_size: 512,
  charged_bytes: 1536,
  width: 100,
  height: 100,
  frames: 1,
  has_original: true,
  has_webp: true,
  has_thumb: true,
  scrubbed: false,
  is_public: false,
  md5: "m",
  sha1: "s",
  src_md5: "sm",
  links: {
    url: "https://img.example/2026/10/a.png",
    original: "https://img.example/2026/10/a.png",
    webp: "https://img.example/2026/10/a.webp",
    thumbnail_url: "https://img.example/2026/10/a_thumbs.webp",
  },
  local_thumb_url: "/t/2026/10/a.png",
  deleted_at: null,
  purge_at: null,
  created_at: "2026-10-04T00:00:00Z",
};

function envelopeJson(data: unknown): string {
  return JSON.stringify({ code: 0, message: "ok", data });
}

describe("preflightUpload", () => {
  it("空文件列表拒绝", () => {
    const result = preflightUpload([]);
    expect(result.ok).toBe(false);
    expect(result.errors[0]).toContain("请先选择");
  });

  it("文件数超过 20 拒绝", () => {
    const files = Array.from({ length: MAX_UPLOAD_FILES + 1 }, (_, i) => makeFile(`f${i}.png`, 10));
    const result = preflightUpload(files);
    expect(result.ok).toBe(false);
    expect(result.errors[0]).toContain(`单次最多上传 ${MAX_UPLOAD_FILES} 个文件`);
  });

  it("单文件超过 20MiB 拒绝", () => {
    const files = [makeFile("small.png", 10), makeFile("big.png", MAX_UPLOAD_FILE_BYTES + 1)];
    const result = preflightUpload(files);
    expect(result.ok).toBe(false);
    expect(result.errors.some((error) => error.includes("单文件上限"))).toBe(true);
    expect(result.errors.some((error) => error.includes("big.png"))).toBe(true);
    expect(result.errors.some((error) => error.includes("small.png"))).toBe(false);
  });

  it("合计超过 64MiB 拒绝", () => {
    const half = Math.floor(MAX_UPLOAD_TOTAL_BYTES / 2) + 1;
    const files = [makeFile("a.png", half), makeFile("b.png", half)];
    const result = preflightUpload(files);
    expect(result.ok).toBe(false);
    expect(result.errors.some((error) => error.includes("单次合计上限"))).toBe(true);
  });

  it("限额内全部通过", () => {
    const files = [makeFile("a.png", 1024), makeFile("b.png", MAX_UPLOAD_FILE_BYTES)];
    const result = preflightUpload(files);
    expect(result.ok).toBe(true);
    expect(result.errors).toEqual([]);
  });
});

describe("uploadImages", () => {
  beforeEach(() => {
    localStorage.clear();
    FakeXHR.instances = [];
    vi.stubGlobal("XMLHttpRequest", FakeXHR);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("单文件 201：解析 ImageView、附 Bearer 头与 multipart 字段、回调进度", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|tok");
    const file = makeFile("a.png", 100);
    const onFileProgress = vi.fn();
    const promise = uploadImages({
      files: [file],
      policyId: 3,
      isPublic: false,
      onFileProgress,
    });

    const xhr = FakeXHR.instances[0];
    expect(xhr.headers.get("Authorization")).toBe("Bearer 7|tok");
    const form = xhr.sent;
    expect(form).not.toBeNull();
    expect(form?.getAll("file").length).toBe(1);
    expect(form?.get("policy_id")).toBe("3");
    expect(form?.get("is_public")).toBe("false");

    xhr.upload.onprogress?.({ loaded: 60, total: 100, lengthComputable: true });
    expect(onFileProgress).toHaveBeenCalledWith(0, file, 60, 100);

    xhr.respond(201, envelopeJson(imageView));
    const results = await promise;
    expect(results).toEqual([{ ok: true, filename: "a.png", status: 201, image: imageView }]);
  });

  it("无 token 时不带 Authorization；is_public=true 按字面量发送", async () => {
    const promise = uploadImages({ files: [makeFile("a.png", 1)], policyId: null, isPublic: true });
    const xhr = FakeXHR.instances[0];
    expect(xhr.headers.has("Authorization")).toBe(false);
    expect(xhr.sent?.get("policy_id")).toBeNull();
    expect(xhr.sent?.get("is_public")).toBe("true");
    xhr.respond(201, envelopeJson(imageView));
    await promise;
  });

  it("外壳 code!=0：失败项携带 code/message/status（与 client 错误语义一致）", async () => {
    const promise = uploadImages({ files: [makeFile("a.png", 1)], policyId: 1, isPublic: false });
    FakeXHR.instances[0].respond(
      415,
      JSON.stringify({ code: 30007, message: "不支持的图片格式", data: null }),
    );
    const results = await promise;
    expect(results).toEqual([
      { ok: false, filename: "a.png", status: 415, code: 30007, message: "不支持的图片格式" },
    ]);
  });

  it("上传遇 20001（凭证失效）触发全局未授权回调", async () => {
    const handler = vi.fn();
    setUnauthorizedHandler(handler);
    try {
      const promise = uploadImages({
        files: [makeFile("a.png", 1)],
        policyId: 1,
        isPublic: false,
      });
      FakeXHR.instances[0].respond(
        401,
        JSON.stringify({ code: 20001, message: "未登录", data: null }),
      );
      const results = await promise;
      expect(results[0]?.ok).toBe(false);
      expect(handler).toHaveBeenCalledTimes(1);
    } finally {
      setUnauthorizedHandler(null);
    }
  });

  it("上传遇 20002（凭证内容错误）不触发全局未授权回调", async () => {
    const handler = vi.fn();
    setUnauthorizedHandler(handler);
    try {
      const promise = uploadImages({
        files: [makeFile("a.png", 1)],
        policyId: 1,
        isPublic: false,
      });
      FakeXHR.instances[0].respond(
        401,
        JSON.stringify({ code: 20002, message: "凭证错误", data: null }),
      );
      const results = await promise;
      expect(results[0]?.ok).toBe(false);
      expect(handler).not.toHaveBeenCalled();
    } finally {
      setUnauthorizedHandler(null);
    }
  });

  it("响应体非 JSON：按网络错误处理（code=-1、status=0）", async () => {
    const promise = uploadImages({ files: [makeFile("a.png", 1)], policyId: 1, isPublic: false });
    FakeXHR.instances[0].respond(200, "gateway timeout");
    const results = await promise;
    expect(results).toEqual([
      { ok: false, filename: "a.png", status: 0, code: -1, message: "网络错误" },
    ]);
  });

  it("网络错误：失败项 code=-1", async () => {
    const promise = uploadImages({ files: [makeFile("a.png", 1)], policyId: 1, isPublic: false });
    FakeXHR.instances[0].failNetwork();
    const results = await promise;
    expect(results).toEqual([
      { ok: false, filename: "a.png", status: 0, code: -1, message: "网络错误" },
    ]);
  });

  it("207 批量响应：成功项映射为 image，失败项映射为 code/message", async () => {
    const promise = uploadImages({ files: [makeFile("a.png", 1)], policyId: 1, isPublic: false });
    FakeXHR.instances[0].respond(
      207,
      envelopeJson([
        {
          filename: "a.png",
          status: 415,
          code: 30007,
          message: "不支持的图片格式",
          data: null,
        },
      ]),
    );
    const results = await promise;
    expect(results).toEqual([
      { ok: false, filename: "a.png", status: 415, code: 30007, message: "不支持的图片格式" },
    ]);
  });

  it("207 批量响应：成功项取 item.data 与 item.status", async () => {
    const promise = uploadImages({ files: [makeFile("a.png", 1)], policyId: 1, isPublic: false });
    FakeXHR.instances[0].respond(
      207,
      envelopeJson([
        { filename: "a.png", status: 201, code: 0, message: "ok", data: imageView },
      ]),
    );
    const results = await promise;
    expect(results).toEqual([{ ok: true, filename: "a.png", status: 201, image: imageView }]);
  });

  it("多文件顺序上传：逐文件回调按索引触发", async () => {
    const files = [makeFile("a.png", 1), makeFile("b.png", 2)];
    const started: number[] = [];
    const settled: number[] = [];
    const promise = uploadImages({
      files,
      policyId: 1,
      isPublic: false,
      onFileStart: (index) => started.push(index),
      onFileSettled: (index) => settled.push(index),
    });

    FakeXHR.instances[0].respond(201, envelopeJson(imageView));
    await vi.waitFor(() => expect(FakeXHR.instances.length).toBe(2));
    FakeXHR.instances[1].respond(201, envelopeJson(imageView));

    const results = await promise;
    expect(started).toEqual([0, 1]);
    expect(settled).toEqual([0, 1]);
    expect(results.length).toBe(2);
    expect(results[0]?.ok).toBe(true);
    expect(results[1]?.filename).toBe("b.png");
  });

  it("前一个文件失败不影响后续文件继续上传", async () => {
    const files = [makeFile("a.png", 1), makeFile("b.png", 2)];
    const promise = uploadImages({ files, policyId: 1, isPublic: false });
    FakeXHR.instances[0].respond(
      415,
      JSON.stringify({ code: 30007, message: "不支持的图片格式", data: null }),
    );
    await vi.waitFor(() => expect(FakeXHR.instances.length).toBe(2));
    FakeXHR.instances[1].respond(201, envelopeJson(imageView));
    const results = await promise;
    expect(results[0]?.ok).toBe(false);
    expect(results[1]?.ok).toBe(true);
  });
});
