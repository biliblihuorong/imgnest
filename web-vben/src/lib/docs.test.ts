import { describe, expect, it } from "vitest";
import { DOC_TOPICS, DOCS_BASE, docUrl, type DocTopicName } from "./docs";

// 文档源文件随仓库一起走：改了文件名或标题而忘了改登记表时，这里会失败。
const pages = import.meta.glob("../../../website/docs/{guide,en/guide}/*.md", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

function source(dir: "guide" | "en/guide", page: string): string | undefined {
  return pages[`../../../website/docs/${dir}/${page}.md`];
}

/** GitHub 与 VitePress 对这些标题生成的锚点一致：小写、空格变连字符、去掉其余标点。 */
function slugs(markdown: string): Set<string> {
  const result = new Set<string>();
  for (const line of markdown.split("\n")) {
    const match = /^#{2,4}\s+(.+?)\s*$/.exec(line);
    if (!match) continue;
    result.add(
      match[1]!
        .toLowerCase()
        .replace(/[^\p{L}\p{N}\s_-]/gu, "")
        .trim()
        .replace(/\s+/g, "-"),
    );
  }
  return result;
}

describe("docs registry", () => {
  const topics = Object.keys(DOC_TOPICS) as DocTopicName[];

  it.each(topics)("%s 的中英文页面与锚点都存在", (topic) => {
    const entry = DOC_TOPICS[topic];
    for (const [dir, lang] of [
      ["guide", "zh"],
      ["en/guide", "en"],
    ] as const) {
      const markdown = source(dir, entry.page);
      expect(markdown, `${dir}/${entry.page}.md`).toBeTypeOf("string");
      if ("anchor" in entry) expect(slugs(markdown!)).toContain(entry.anchor[lang]);
    }
  });

  it("按界面语言选择文档目录并编码锚点", () => {
    expect(docUrl("pathTemplate", "zh-CN")).toBe(
      `${DOCS_BASE}/guide/storage-and-policies.md#${encodeURIComponent("路径模板")}`,
    );
    expect(docUrl("pathTemplate", "en-US")).toBe(
      `${DOCS_BASE}/en/guide/storage-and-policies.md#path-templates`,
    );
    expect(docUrl("backup", "fr")).toBe(`${DOCS_BASE}/en/guide/backup.md`);
  });
});
