import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, it } from "vitest";
import { readAppBrand } from "./app-config-html";

it("reads the brand from this project's app.config.ts", () => {
  const source = readFileSync(resolve(process.cwd(), "src/app.config.ts"), "utf8");
  const brand = readAppBrand(source);
  expect(brand.name).not.toBe("");
  expect(brand.storagePrefix).toMatch(/^[\w.-]+$/);
  expect(existsSync(resolve(process.cwd(), `.${brand.logo}`))).toBe(true);
});

it("follows a swapped logo file", () => {
  const source = `import logo from "@/assets/brand/acme.png";
export const appConfig = {
  name: "Acme 后台",
  storagePrefix: "acme",
};`;
  expect(readAppBrand(source)).toEqual({
    name: "Acme 后台",
    storagePrefix: "acme",
    logo: "/src/assets/brand/acme.png",
  });
});

it("ignores names outside the appConfig object", () => {
  const source = `import logo from "@/assets/logo.svg";
interface MenuItem {
  name: string;
}
export const appConfig = {
  name: "Real",
  storagePrefix: "real",
};
export const menus = [
  { name: "common.nav.dashboard" },
];`;
  expect(readAppBrand(source).name).toBe("Real");
});

it("fails loudly when a field cannot be found", () => {
  expect(() => readAppBrand(`export const appConfig = { name: "X" };`)).toThrow(
    /storagePrefix.*import logo/,
  );
});

it("rejects a logo outside the project", () => {
  expect(() =>
    readAppBrand(`import logo from "some-package/logo.svg";
export const appConfig = {
  name: "X",
  storagePrefix: "x",
};`),
  ).toThrow(/logo/);
});
