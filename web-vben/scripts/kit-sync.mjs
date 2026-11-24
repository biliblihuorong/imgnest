#!/usr/bin/env node
/**
 * 把 admin-kit 的核心文件（kit.manifest.json）同步到一个用了本模板的项目。
 *
 * 在 admin-kit 仓库里运行（项目里同步过去的这份会拒绝运行）：
 *   node scripts/kit-sync.mjs <项目前端目录>            只检查，列出不同的文件；有差异时退出码为 1（可放进 CI）
 *   node scripts/kit-sync.mjs <项目前端目录> --diff     同时打印每个不同文件的 diff
 *   node scripts/kit-sync.mjs <项目前端目录> --write    把模板版本写过去（只新增/覆盖，从不删除项目文件）
 *
 * 项目里改过核心文件时会显示为「不同」：先把那部分需求改成走 app.hooks.ts / app.config.ts，
 * 或者把改动提回模板，再同步。核心目录（以 / 结尾）里只有项目才有的文件会单独列出，不会被删。
 * 只用 Node 自带模块，不需要先 pnpm install。
 */
import { spawnSync } from "node:child_process";
import { copyFileSync, existsSync, mkdirSync, readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const kitRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const args = process.argv.slice(2);
const target = args.find((arg) => !arg.startsWith("--"));
const write = args.includes("--write");
const showDiff = args.includes("--diff");

if (!target) {
  console.error("用法：node scripts/kit-sync.mjs <项目前端目录> [--diff | --write]");
  process.exit(2);
}
const targetRoot = resolve(target);
if (targetRoot === kitRoot) {
  console.error("目标目录就是模板本身。");
  process.exit(2);
}
if (!existsSync(join(targetRoot, "package.json"))) {
  console.error(`${targetRoot} 里没有 package.json，确认传的是项目的前端目录。`);
  process.exit(2);
}

const manifest = JSON.parse(readFileSync(join(kitRoot, "kit.manifest.json"), "utf8"));
// 本脚本也会被同步进各项目；只有在模板仓库里运行才有意义。
const kitPackage = JSON.parse(readFileSync(join(kitRoot, "package.json"), "utf8"));
if (kitPackage.name !== manifest.kit) {
  console.error(
    `这里是项目「${kitPackage.name}」，不是模板。请在 ${manifest.kit} 仓库里运行：pnpm kit:sync <本项目前端目录>`,
  );
  process.exit(2);
}
const IGNORED_DIRS = new Set(["node_modules", "dist", ".turbo", "coverage"]);

function walk(root, dir) {
  if (!existsSync(join(root, dir))) return [];
  return readdirSync(join(root, dir)).flatMap((name) => {
    if (IGNORED_DIRS.has(name)) return [];
    const path = join(dir, name);
    return statSync(join(root, path)).isDirectory() ? walk(root, path) : [path];
  });
}

const kitFiles = new Set();
const targetOnly = [];
for (const entry of manifest.files) {
  if (entry.endsWith("/")) {
    const kitSide = walk(kitRoot, entry);
    for (const file of kitSide) kitFiles.add(file);
    const known = new Set(kitSide);
    for (const file of walk(targetRoot, entry)) if (!known.has(file)) targetOnly.push(file);
  } else {
    kitFiles.add(entry);
  }
}

const same = [];
const changed = [];
const added = [];
for (const file of [...kitFiles].sort()) {
  const from = join(kitRoot, file);
  if (!existsSync(from)) {
    console.error(`清单里的 ${file} 在模板中不存在，先修 kit.manifest.json。`);
    process.exit(2);
  }
  const to = join(targetRoot, file);
  if (!existsSync(to)) added.push(file);
  else if (readFileSync(from).equals(readFileSync(to))) same.push(file);
  else changed.push(file);
}

const rel = (path) => relative(process.cwd(), path) || ".";
console.log(`模板：${rel(kitRoot)}  →  项目：${rel(targetRoot)}`);
console.log(`相同 ${same.length}，不同 ${changed.length}，项目缺少 ${added.length}`);
for (const file of changed) {
  console.log(`  不同  ${file}`);
  if (showDiff)
    spawnSync("diff", ["-u", join(targetRoot, file), join(kitRoot, file)], { stdio: "inherit" });
}
for (const file of added) console.log(`  缺少  ${file}`);
if (targetOnly.length > 0) {
  console.log(`核心目录里只有项目才有的文件（不会动它们）：`);
  for (const file of targetOnly) console.log(`  项目  ${file}`);
}

if (write) {
  for (const file of [...changed, ...added]) {
    const to = join(targetRoot, file);
    mkdirSync(dirname(to), { recursive: true });
    copyFileSync(join(kitRoot, file), to);
  }
  console.log(
    `已写入 ${changed.length + added.length} 个文件。接着在项目里跑 typecheck / test / build。`,
  );
} else if (changed.length + added.length > 0) {
  console.log("加 --diff 查看差异，加 --write 同步。");
  process.exit(1);
}
