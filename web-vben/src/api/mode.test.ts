import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { expect, it } from "vitest";

const SRC = resolve(process.cwd(), "src");

function sourceFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return sourceFiles(path);
    return /\.(ts|vue)$/.test(name) && !/\.test\.ts$/.test(name) ? [path] : [];
  });
}

// 静态 import mock.ts 会让演示账号和假数据进入生产包（见 src/api/mode.ts）。
it("never imports the mock backend statically outside tests", () => {
  const offenders = sourceFiles(SRC).filter((file) =>
    /^\s*import\b[^;]*from\s+["'](?:@\/api\/|\.\/|\.\.\/api\/)mock["']/m.test(
      readFileSync(file, "utf8"),
    ),
  );
  expect(offenders.map((file) => relative(SRC, file))).toEqual([]);
});

// client.ts 必须原地重复 MOCK_MODE 的条件，二者一旦不一致，界面提示和实际请求会对不上。
it("keeps the mock condition in client.ts identical to MOCK_MODE", () => {
  const normalize = (text: string) => text.replace(/\s+/g, " ").trim();
  const condition = (source: string, pattern: RegExp) => normalize(pattern.exec(source)?.[1] ?? "");
  const mode = condition(
    readFileSync(join(SRC, "api/mode.ts"), "utf8"),
    /MOCK_MODE: boolean =([\s\S]*?);/,
  );
  const client = condition(
    readFileSync(join(SRC, "api/client.ts"), "utf8"),
    /if \(\s*(import\.meta\.env\.DEV[\s\S]*?)\) \{\s*try \{\s*const \{ mockRequest \}/,
  );
  expect(mode).not.toBe("");
  expect(client).toBe(mode);
});
