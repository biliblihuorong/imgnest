import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import { resolve } from "node:path";
import { parseArgs } from "node:util";

const { values } = parseArgs({
  options: {
    origin: { type: "string", default: "http://127.0.0.1:4174" },
    chromium: { type: "string" },
    output: { type: "string", default: "/tmp/imgnest-doc-layout" },
  },
});
// Reuse external browser tooling; the website's package/lock file stays unchanged.
const { chromium } = await import(
  process.env.IMGNEST_E2E_PLAYWRIGHT_MODULE || "playwright"
);
const output = resolve(values.output);
await mkdir(output, { recursive: true });
const browser = await chromium.launch({
  headless: true,
  executablePath: values.chromium,
  args: ["--no-sandbox"],
});
const failures = [];
const results = [];
const check = (condition, message) => {
  if (!condition) failures.push(message);
};

async function inspect(page, name) {
  const measurements = await page.evaluate(() => {
    const rect = (selector) => {
      const { x, y, width, height, bottom, right } = document
        .querySelector(selector)
        .getBoundingClientRect();
      return { x, y, width, height, bottom, right };
    };
    const card = document.querySelector(".VPDocAside");
    return {
      card: rect(".VPDocAside"),
      outline: rect(".VPDocAsideOutline"),
      title: rect(".outline-title"),
      rail: rect(".VPDocAsideOutline .content"),
      article: rect(".VPDoc > .container > .content > .content-container"),
      nav: rect(".VPNav"),
      viewport: { width: innerWidth, height: innerHeight },
      cardScrollable: card.scrollHeight > card.clientHeight,
      documentWidth: document.documentElement.scrollWidth,
    };
  });
  results.push({ name, ...measurements });
  await page.screenshot({ path: `${output}/${name}.png`, fullPage: false });
  return measurements;
}

try {
  for (const [name, width, dark, language] of [
    ["desktop-light", 1440, false, ""],
    ["desktop-dark", 1440, true, ""],
    ["desktop-english", 1280, false, "/en"],
    ["desktop-wide", 1920, false, ""],
  ]) {
    const page = await browser.newPage({
      viewport: { width, height: 980 },
      colorScheme: dark ? "dark" : "light",
    });
    page.setDefaultTimeout(10000);
    page.on("pageerror", (error) => failures.push(`${name}: ${error.message}`));
    await page.goto(`${values.origin}${language}/guide/configuration.html`, {
      waitUntil: "networkidle",
    });
    const m = await inspect(page, name);
    check(
      Math.abs(m.card.y - m.article.y) <= 2,
      `${name}: outline and article tops differ`,
    );
    check(
      m.card.bottom - m.outline.bottom <= 32,
      `${name}: outline card stretches beyond its contents`,
    );
    check(
      m.title.x - m.rail.x >= 12,
      `${name}: outline text loses its inset from the active-marker rail`,
    );
    check(m.documentWidth <= width, `${name}: page overflows horizontally`);
    const link = page.locator(".VPDocAsideOutline .outline-link").last();
    const hash = await link.getAttribute("href");
    await link.click();
    await page.waitForFunction(
      (hash) => decodeURIComponent(location.hash) === decodeURIComponent(hash),
      hash,
    );
    // The card must remain usable after the document scrolls to an outline anchor.
    await page.waitForFunction(() => window.scrollY > 0);
    const scrolled = await inspect(page, `${name}-scrolled`);
    check(
      scrolled.card.y >= scrolled.nav.bottom,
      `${name}: outline overlaps the header when scrolling`,
    );
    check(
      scrolled.card.bottom <= 980 - 16,
      `${name}: outline extends past the viewport`,
    );
    await page.close();
  }

  const short = await browser.newPage({
    viewport: { width: 1440, height: 480 },
  });
  short.setDefaultTimeout(10000);
  await short.goto(`${values.origin}/guide/deployment.html`, {
    waitUntil: "networkidle",
  });
  const m = await inspect(short, "short-screen-long-outline");
  check(
    m.card.bottom <= 480 - 16,
    "short screen: outline card does not fit the available height",
  );
  check(
    m.cardScrollable,
    "short screen: long outline has no internal scroll area",
  );
  const before = await short
    .locator(".VPDocAside")
    .evaluate((el) => el.scrollTop);
  await short.locator(".VPDocAside").hover();
  await short.mouse.wheel(0, 400);
  if (m.cardScrollable) {
    await short.waitForFunction(
      (before) => document.querySelector(".VPDocAside").scrollTop > before,
      before,
    );
  }
  await short.close();

  for (const width of [390, 1024]) {
    const page = await browser.newPage({ viewport: { width, height: 844 } });
    page.setDefaultTimeout(10000);
    await page.goto(`${values.origin}/guide/configuration.html`, {
      waitUntil: "networkidle",
    });
    const layout = await page.evaluate(() => ({
      asideHidden:
        getComputedStyle(document.querySelector(".VPDoc .aside")).display ===
        "none",
      width: document.documentElement.scrollWidth,
    }));
    check(
      layout.asideHidden,
      `${width}px: desktop outline is visible on a narrow screen`,
    );
    check(layout.width <= width, `${width}px: page overflows horizontally`);
    await page.screenshot({ path: `${output}/narrow-${width}.png` });
    await page.close();
  }
} catch (error) {
  failures.push(error.message);
  throw error;
} finally {
  await browser.close();
  await writeFile(
    `${output}/results.json`,
    JSON.stringify({ results, failures }, null, 2),
  );
}
assert.equal(failures.length, 0, failures.join("\n"));
console.log(`Documentation layout checks passed; screenshots: ${output}`);
