# 使用手册

按你现在要完成的事情查找。第一次部署从「快速开始」读起；已经有账号，直接看「上传与图片管理」。

::: tip 手册与版本
手册跟随 `main` 更新，固定版本镜像不会自动获得之后的新功能。若界面与说明不同，请先在「站点管理 → 站点设置 → 关于」核对版本；`edge` 跟随 `main`，正式部署请按需要选择版本。
:::

## 第一次使用

1. [了解 ImgNest](./introduction.md)：能力、当前版本和限制。
2. [快速开始](./getting-started.md)：启动服务、创建管理员和存储、上传第一张图。
3. [上传与图片管理](./upload-and-images.md)：上传、复制链接、找图与批量操作。

## 按任务查找

| 我想…… | 界面入口 | 操作说明 |
| --- | --- | --- |
| 上传图片、复制链接 | 上传图片 `/upload` | [上传与图片管理](./upload-and-images.md) |
| 整理相册、移动图片 | 我的相册 `/albums`；我的图片勾选后移动 | [相册管理](./albums-and-gallery.md) |
| 按文件名、格式、日期找图 | 我的图片 `/images` → 顶部搜索框 | [搜索语法](./search.md) |
| 让一个地址随机显示相册图片 | 我的相册 → 点封面或名称 → 随机图片链接 | [随机图片链接](./random-links.md) |
| 查看 EXIF、删除或恢复图片 | 我的图片 → 图片详情；顶部「回收站」标签 | [隐私与回收站](./privacy-and-trash.md) |
| 接入 PicGo / Typora / uPic | 账户 `/account/tokens` → 创建 Token | [蓝空 API 与 PicGo](./lsky-api.md) |
| 写上传脚本或给博客配响应式图片 | 脚本调用接口；管理员先在规则中启用衍生图 | [原生 API v2](./api-v2.md) |
| 配置用户组、容量和上传权限 | 站点管理 → 用户组 / 用户管理 | [用户与用户组](./users-and-groups.md) |
| 增加存储、设置路径与转码 | 站点管理 → 存储管理 / 规则管理 | [存储与规则](./storage-and-policies.md) |
| 决定访客看到什么、能否匿名上传 | 站点管理 → 站点设置 → 访客与画廊 | [访客与公开展示](./public-access.md) |
| 配置注册、验证码、邮件和找回密码 | 站点设置 → 注册与登录 / 邮件 | [登录保护](./sign-in-protection.md) |
| 部署、升级和配置启动参数 | 服务器终端、Compose 文件或配置文件 | [部署](./deployment.md) |
| 从蓝空迁移 | 站点设置 → 备份/导出 → 页面底部蓝空迁移 | [从蓝空迁移](./lsky-migration.md) |
| 备份数据库与配置 | 站点设置 → 备份/导出 → 立即备份 | [备份与还原](./backup.md) |
| 修改密码、查用量与登录活动 | 账户 → 资料与安全 `/account/settings`；数据概览 `/dashboard` | [账户与用量](./account-and-overview.md) |
| 切换布局、语言和深浅色 | 新版布局侧栏底部齿轮 → 设置 → 外观 | [界面与主题](./frontends.md) |

导航收起时先展开侧栏；手机上先点「打开导航」。下文的站内地址要加在你自己的站点域名后，例如 `https://你的站点/albums`，不是文档站地址。「站点管理」仅管理员可见。响应式图片用法见[响应式图片 API](./responsive-images.md)，启动参数见[配置](./configuration.md)。

## 文档怎么分工

- 本手册面向使用者与站点管理员：功能的操作步骤只在对应专题里说明，相关页面用链接连接。
- [OpenAPI](https://github.com/biliblihuorong/imgnest/blob/main/docs/openapi.yaml) 是接口字段与错误码的详细契约。
- 仓库的 [`docs/`](https://github.com/biliblihuorong/imgnest/tree/main/docs) 面向开发者，包含规范、开发与部署说明；`planning/`、`superpowers/`、`verification/` 是开发过程记录，不是使用顺序。
