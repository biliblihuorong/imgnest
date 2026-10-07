/**
 * 首页的全部文案。改字只改这个文件：zh 是中文首页，en 是英文首页（/en/）。
 * 两份结构必须一致，TypeScript 会在少字段时报错。
 */

export type Layout = "classic" | "marvis";
export type Mode = "light" | "dark";
export type MockLang = "zh" | "en";

/** 滚动到某一步时，界面演示切到的状态。lang: "alt" 表示切到当前页面语言之外的另一种。 */
export interface TourState {
  layout: Layout;
  mode: Mode;
  lang: "base" | "alt";
}

export interface Step {
  title: string;
  body: string;
  note?: string;
}

export interface HomeCopy {
  hero: { title: string; lead: string; primary: string; secondary: string; docs: string };
  tour: { steps: (Step & { state: TourState })[]; controls: Record<"layout" | "mode" | "lang", string> } & {
    options: Record<Layout | Mode, string>;
  };
  engine: {
    title: string;
    lead: string;
    steps: Step[];
    pipeline: { caption: string; stages: string[]; done: string };
    limits: { caption: string; working: string; waiting: string; rows: [label: string, value: string][]; foot: string };
    recovery: { caption: string; written: string; failed: string; removed: string; facts: string[] };
  };
  compat: { title: string; body: string; points: string[]; request: string; response: string; link: string };
  start: { title: string; body: string; action: string; comments: [string, string, string] };
  footer: string;
}

export const copy: Record<MockLang, HomeCopy> = {
  zh: {
    hero: {
      title: "你的图片，放在你自己的图床上",
      lead: "ImgNest 是用 Go 和 Vue 3 写的自托管图床：多用户、多存储，原图和 WebP 各存一份，兼容蓝空图床 v1 API，PicGo、uPic、Typora 改个地址就能继续用。",
      primary: "开始使用",
      secondary: "往下看界面",
      docs: "/guide/getting-started",
    },
    tour: {
      steps: [
        {
          title: "经典布局，上手不用学",
          body: "顶栏、标签页、贴边侧栏。从蓝空或者别的后台换过来，东西都在你习惯的位置。",
          state: { layout: "classic", mode: "light", lang: "base" },
        },
        {
          title: "新版布局，把地方让给图片",
          body: "侧栏浮起来，Ctrl K 直接搜图，点一张图，右侧面板就给出尺寸、大小和各种格式的链接。",
          note: "在「设置 → 外观」里随时切回经典布局，不用重新构建。",
          state: { layout: "marvis", mode: "light", lang: "base" },
        },
        {
          title: "深色模式，晚上看图不刺眼",
          body: "浅色、深色，或者跟随系统。强调色和圆角也可以自己调，两套布局共用同一份偏好。",
          state: { layout: "marvis", mode: "dark", lang: "base" },
        },
        {
          title: "中文和 English，一键切换",
          body: "界面文案全部来自语言包，没登录的访客也能切换语言、深浅色和布局。",
          state: { layout: "marvis", mode: "dark", lang: "alt" },
        },
      ],
      controls: { layout: "布局", mode: "外观", lang: "语言" },
      options: { classic: "经典", marvis: "新版", light: "浅色", dark: "深色" },
    },
    engine: {
      title: "润物细无声",
      lead: "转码、脱敏、清理都在后台做完。你能注意到的只有一件事：链接已经好了。",
      steps: [
        {
          title: "快：链接返回时就能打开",
          body: "图片处理交给 libvips。原图、WebP、缩略图在同一次上传请求里生成并写入存储，响应里的每个链接拿到就能访问。",
          note: "后台列表和预览读的是本机缩略图，不消耗云端流量。",
        },
        {
          title: "省：内存有上限，不靠运气",
          body: "同时处理的上传数、单个文件大小、像素总量、处理时长都有明确上限，超出的排队或者直接拒绝，一张超大图拖不垮整个服务。",
          note: "运行时使用 jemalloc，并关闭 libvips 的操作缓存，避免长期运行后内存越涨越高。",
        },
        {
          title: "稳：失败了不留半截文件",
          body: "任何一个对象写入失败，或者数据库事务失败，这次上传已经写进去的对象会全部删掉。删除先进回收站，默认保留 7 天。",
          note: "服务重启时，先把没做完的上传、删除、恢复处理完，再开始接收请求。",
        },
      ],
      pipeline: {
        caption: "一次上传经过的步骤",
        stages: [
          "读文件头，确认真实格式",
          "完整 EXIF 存进本地库",
          "原位抹掉 GPS，不重新编码",
          "转出 WebP",
          "生成缩略图",
          "写入存储",
          "入库，返回链接",
        ],
        done: "响应返回，原图、WebP、缩略图链接均可访问",
      },
      limits: {
        caption: "默认上限",
        working: "处理中",
        waiting: "排队",
        rows: [
          ["同时处理的上传请求", "2 个"],
          ["单个文件", "20 MiB"],
          ["整个请求", "64 MiB"],
          ["像素总量（含动图各帧）", "1 亿"],
          ["单次处理时长", "5 分钟"],
        ],
        foot: "都可以在配置文件或环境变量里调整",
      },
      recovery: {
        caption: "上传中途失败时",
        written: "已写入",
        failed: "写入失败",
        removed: "已清理",
        facts: ["回收站默认保留 7 天", "数据库变更只走版本化迁移", "全部测试在竞态检测下运行"],
      },
    },
    compat: {
      title: "蓝空的客户端，直接接过来",
      body: "`/api/v1` 的路由、字段名、类型和单位与蓝空图床保持一致，只多不少。旧客户端认识的字段都在，不认识的会被忽略。",
      points: ["PicGo、uPic、Typora 只需要改接口地址", "`links.url` 默认返回 WebP 地址", "v1 接口不返回任何 EXIF 或 GPS"],
      request: "请求",
      response: "响应",
      link: "查看接口说明",
    },
    start: {
      title: "一个镜像就能跑起来",
      body: "SQLite 和 PostgreSQL 共用同一个 Docker 镜像，支持 amd64 和 arm64，启动时自动完成数据库迁移。创建管理员时怎么传密码，文档里有完整写法。",
      action: "完整步骤",
      comments: ["启动服务（SQLite）", "创建管理员，密码从标准输入读取", "创建本机存储"],
    },
    footer: "ImgNest 是开源项目，网站内容与仓库文档同步维护。",
  },

  en: {
    hero: {
      title: "Your images, on an image host you run yourself",
      lead: "ImgNest is a self-hosted image host written in Go and Vue 3. It serves multiple users and storages, keeps both the original and a WebP copy, and speaks the Lsky Pro v1 API, so PicGo, uPic and Typora keep working after you change one URL.",
      primary: "Get started",
      secondary: "See the interface",
      docs: "/en/guide/getting-started",
    },
    tour: {
      steps: [
        {
          title: "Classic layout, nothing to relearn",
          body: "A top bar, tabs and a docked sidebar. Coming from Lsky Pro or another admin panel, everything is where you expect it.",
          state: { layout: "classic", mode: "light", lang: "base" },
        },
        {
          title: "New layout, more room for images",
          body: "The sidebar floats, Ctrl K searches your images, and selecting an image opens a side panel with its dimensions, size and links in every format.",
          note: "Switch back to the classic layout in Settings → Appearance. No rebuild needed.",
          state: { layout: "marvis", mode: "light", lang: "base" },
        },
        {
          title: "Dark mode for late evenings",
          body: "Light, dark, or follow the system. Accent colour and corner radius are adjustable too, and both layouts share the same preferences.",
          state: { layout: "marvis", mode: "dark", lang: "base" },
        },
        {
          title: "中文 and English, one click apart",
          body: "Every label comes from a language pack. Visitors who are not signed in can switch language, colour mode and layout as well.",
          state: { layout: "marvis", mode: "dark", lang: "alt" },
        },
      ],
      controls: { layout: "Layout", mode: "Appearance", lang: "Language" },
      options: { classic: "Classic", marvis: "New", light: "Light", dark: "Dark" },
    },
    engine: {
      title: "Quiet by design",
      lead: "Conversion, metadata scrubbing and cleanup all finish in the background. The one thing you notice is that the link already works.",
      steps: [
        {
          title: "Fast: links work the moment they arrive",
          body: "libvips does the image work. The original, the WebP copy and the thumbnail are produced and stored within the same upload request, so every link in the response is already reachable.",
          note: "Lists and previews in the admin read local thumbnails and use no cloud traffic.",
        },
        {
          title: "Lean: memory use has a ceiling",
          body: "Concurrent uploads, file size, total pixels and processing time each have a fixed limit. Anything beyond them waits or is rejected, so one huge image cannot take the service down.",
          note: "The runtime uses jemalloc and turns off the libvips operation cache to keep memory from creeping up over long uptimes.",
        },
        {
          title: "Steady: a failed upload leaves nothing behind",
          body: "If any object write or the database transaction fails, every object already written for that upload is removed. Deleted images go to a recycle bin, kept for 7 days by default.",
          note: "On restart, unfinished uploads, deletions and restores are completed before the server accepts requests.",
        },
      ],
      pipeline: {
        caption: "What one upload goes through",
        stages: [
          "Read the file header to find the real format",
          "Store the full EXIF in the local database",
          "Erase GPS in place, without re-encoding",
          "Encode the WebP copy",
          "Generate the thumbnail",
          "Write to storage",
          "Save the record and return links",
        ],
        done: "Response sent: original, WebP and thumbnail links all resolve",
      },
      limits: {
        caption: "Default limits",
        working: "Processing",
        waiting: "Waiting",
        rows: [
          ["Uploads processed at once", "2"],
          ["Single file", "20 MiB"],
          ["Whole request", "64 MiB"],
          ["Total pixels, animation frames included", "100 million"],
          ["Processing time per upload", "5 minutes"],
        ],
        foot: "All adjustable in the config file or through environment variables",
      },
      recovery: {
        caption: "When an upload fails midway",
        written: "Written",
        failed: "Write failed",
        removed: "Removed",
        facts: [
          "Recycle bin keeps images for 7 days by default",
          "Schema changes only through versioned migrations",
          "The whole test suite runs under the race detector",
        ],
      },
    },
    compat: {
      title: "Lsky Pro clients work as they are",
      body: "Routes, field names, types and units under `/api/v1` match Lsky Pro, with extra fields added and none removed. Old clients find every field they know and ignore the rest.",
      points: [
        "PicGo, uPic and Typora only need a new endpoint URL",
        "`links.url` returns the WebP address by default",
        "No v1 route ever returns EXIF or GPS data",
      ],
      request: "Request",
      response: "Response",
      link: "Read the API guide",
    },
    start: {
      title: "One image is all it takes",
      body: "SQLite and PostgreSQL share one Docker image, built for amd64 and arm64, and database migrations run on start. The docs show how to pass the administrator password.",
      action: "Full steps",
      comments: [
        "Start the server (SQLite)",
        "Create the administrator; the password is read from standard input",
        "Create a local storage",
      ],
    },
    footer: "ImgNest is open source. This site is maintained alongside the repository docs.",
  },
};

/** 界面演示里的文字，取自 web-vben 的语言包。 */
export const mockLabels: Record<MockLang, Record<string, string>> = {
  zh: {
    badge: "图床",
    search: "搜索图片",
    upload: "上传图片",
    images: "我的图片",
    albums: "我的相册",
    gallery: "公开画廊",
    dashboard: "数据概览",
    tokens: "账户与令牌",
    admin: "站点管理",
    users: "用户管理",
    storages: "存储管理",
    policies: "规则管理",
    count: "共 128 张",
    size: "大小",
    dimensions: "尺寸",
    webp: "WebP",
    album: "相册",
    albumName: "旅行",
    copy: "复制链接",
  },
  en: {
    badge: "Images",
    search: "Search images",
    upload: "Upload",
    images: "My images",
    albums: "My albums",
    gallery: "Public gallery",
    dashboard: "Overview",
    tokens: "Account & tokens",
    admin: "Administration",
    users: "Users",
    storages: "Storage",
    policies: "Upload policies",
    count: "128 images",
    size: "Size",
    dimensions: "Dimensions",
    webp: "WebP",
    album: "Album",
    albumName: "Travel",
    copy: "Copy link",
  },
};
