/** 上传结果的复制格式。 */
export type LinkFormat = "url" | "markdown" | "html" | "bbcode";

/** 从文件名与链接构造各格式的复制文本。 */
export function buildLinkText(format: LinkFormat, name: string, link: string): string {
  switch (format) {
    case "url":
      return link;
    case "markdown":
      return `![${name}](${link})`;
    case "html":
      return `<img src="${link}" alt="${name}" />`;
    case "bbcode":
      return `[img]${link}[/img]`;
  }
}
