/**
 * 上传结果的复制格式。文件名与链接都是外部输入：拼进 markdown / html /
 * bbcode 前必须先转义，防止 `]`、`(`、`<`、空格等结构字符把复制文本
 * 撕裂或注入出额外标记；url 格式原样返回，不做任何加工。
 */
export type LinkFormat = "url" | "markdown" | "html" | "bbcode";

/** HTML 复制文本里的最小转义：文件名可能含引号或尖括号。 */
function escapeHtml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

/** markdown alt 转义：`\`、`[`、`]`、`(`、`)` 会破坏 `![alt](<url>)` 结构，前置反斜杠转义。 */
function escapeMarkdownAlt(value: string): string {
  return value.replace(/[\\[\]()]/g, "\\$&");
}

/** markdown 链接转义（尖括号目标形式）：`<`、`>` 会提前闭合，空格不被允许，统一百分号编码。 */
function escapeMarkdownUrl(value: string): string {
  return value.replace(/</g, "%3C").replace(/>/g, "%3E").replace(/ /g, "%20");
}

/** bbcode 链接转义：`[`、`]` 会伪造 [img]/[/img] 标签，百分号编码后仍是合法 URL。 */
function escapeBbcodeUrl(value: string): string {
  return value.replace(/\[/g, "%5B").replace(/\]/g, "%5D");
}

/** 从文件名与链接构造各格式的复制文本（各格式的转义规则见上方 escape 函数）。 */
export function buildLinkText(format: LinkFormat, name: string, link: string): string {
  switch (format) {
    case "url":
      return link;
    case "markdown":
      return `![${escapeMarkdownAlt(name)}](<${escapeMarkdownUrl(link)}>)`;
    case "html":
      return `<img src="${escapeHtml(link)}" alt="${escapeHtml(name)}" />`;
    case "bbcode":
      return `[img]${escapeBbcodeUrl(link)}[/img]`;
  }
}

/** 链接指向的版本：原图 / WebP / 缩略图。 */
export type LinkVersion = "original" | "webp" | "thumbnail";

/**
 * 按版本取一个上传结果的实际链接；选中版本缺失时回退到首选可用链接
 * （links.url 永远有值，跟随规则的 link_prefer）。
 */
export function resolveImageLink(
  image: { links: { url: string; original: string; webp: string; thumbnail_url: string } },
  version: LinkVersion,
): string {
  switch (version) {
    case "original":
      return image.links.original || image.links.url;
    case "webp":
      return image.links.webp || image.links.url;
    case "thumbnail":
      return image.links.thumbnail_url || image.links.url;
  }
}
