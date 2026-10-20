/**
 * 界面里「?」帮助链接指向的文档，统一在这里登记。
 *
 * 新增一个帮助点：在 DOC_TOPICS 里加一项（页面 slug 对应 website/docs/guide/ 下的
 * 文件名，锚点按中英文标题分别写），然后在界面里放 `<DocHelp topic="..." />`。
 * 文档改了标题或文件名时只需改这里；docs.test.ts 会检查每个页面和锚点都还存在。
 */

/** 文档没有单独部署，直接链到 GitHub 上渲染好的 Markdown。 */
export const DOCS_BASE = "https://github.com/biliblihuorong/imgnest/blob/main/website/docs";

export interface DocTopic {
  /** website/docs/guide/{page}.md */
  page: string;
  /** 页内标题的锚点（不含 #）；中英文标题不同，分开写。 */
  anchor?: { zh: string; en: string };
}

export const DOC_TOPICS = {
  storage: { page: "storage-and-policies", anchor: { zh: "存储", en: "storage" } },
  pathTemplate: {
    page: "storage-and-policies",
    anchor: { zh: "路径模板", en: "path-templates" },
  },
  responsiveVariants: {
    page: "storage-and-policies",
    anchor: { zh: "响应式图片版本", en: "responsive-image-versions" },
  },
  responsiveApi: { page: "responsive-images" },
  exifScrub: { page: "privacy-and-trash", anchor: { zh: "清理档位", en: "scrubbing-levels" } },
  randomLinks: { page: "random-links" },
  search: { page: "search" },
  guestUpload: {
    page: "public-access",
    anchor: { zh: "公开首页与游客上传", en: "public-home-page-and-guest-uploads" },
  },
  gallery: { page: "public-access", anchor: { zh: "公开画廊", en: "the-public-gallery" } },
  apiTokens: { page: "lsky-api", anchor: { zh: "获取-token", en: "get-a-token" } },
  backup: { page: "backup" },
  lskyMigration: { page: "lsky-migration" },
  signInProtection: { page: "sign-in-protection" },
  mail: {
    page: "sign-in-protection",
    anchor: { zh: "smtp-与邮箱验证码", en: "smtp-and-email-codes" },
  },
} as const satisfies Record<string, DocTopic>;

export type DocTopicName = keyof typeof DOC_TOPICS;

/** 按界面语言给出某个帮助点的文档地址；非中文界面一律用英文文档。 */
export function docUrl(topic: DocTopicName, locale: string): string {
  const entry: DocTopic = DOC_TOPICS[topic];
  const zh = locale.toLowerCase().startsWith("zh");
  const dir = zh ? "guide" : "en/guide";
  const anchor = entry.anchor
    ? `#${encodeURIComponent(zh ? entry.anchor.zh : entry.anchor.en)}`
    : "";
  return `${DOCS_BASE}/${dir}/${entry.page}.md${anchor}`;
}
