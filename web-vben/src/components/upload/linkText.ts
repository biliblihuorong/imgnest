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
