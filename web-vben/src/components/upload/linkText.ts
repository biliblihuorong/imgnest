/** 上传结果的复制格式。 */
export type LinkFormat = "url" | "markdown" | "html" | "bbcode";

/** HTML 复制文本里的最小转义：文件名可能含引号或尖括号。 */
function escapeHtml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

/** 从文件名与链接构造各格式的复制文本。 */
export function buildLinkText(format: LinkFormat, name: string, link: string): string {
  switch (format) {
    case "url":
      return link;
    case "markdown":
      return `![${name}](${link})`;
    case "html":
      return `<img src="${escapeHtml(link)}" alt="${escapeHtml(name)}" />`;
    case "bbcode":
      return `[img]${link}[/img]`;
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
