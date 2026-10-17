# 搜索语法

个人图片页和相册详情页共用一套可编辑的查询语法。普通词是文件名原文的子串,多个词、多个条件之间是"与";只有 `format:` 和 `album:` 的逗号列表内部是"或"。没有布尔表达式、通配符和正则——`%`、`_`、`*`、`AND`、`OR`、`NOT` 都是普通文本。

## 示例

```
暑假 海边 format:jpg,png album:"暑假照片" after:2026-10-01
"家庭 合照" camera:"Canon EOS" minsize:1MB maxsize:20MB
album:#42,#57 sort:oldest
visibility:private album:unfiled before:2026-11-01
```

## 字段

| 字段 | 可接受的值 |
| --- | --- |
| `format` | jpg, png, gif, webp, avif, bmp, tiff, svg, heic, heif, unknown(逗号分隔多选) |
| `album` | 精确相册名、`#` 加十进制 ID、或未加引号的 `unfiled`(未分组) |
| `camera` | 一个文本值,匹配归一化的相机 / 镜头型号 |
| `minsize` / `maxsize` | 含边界的 MB 数,0–1024000,最多六位小数 |
| `after` / `before` | `YYYY-MM-DD`;`after` 含当天零点,`before` 不含 |
| `visibility` | all, public, private |
| `sort` | newest, oldest, size-desc, size-asc, name-asc, name-desc |

细节:

- `1MB` 恰好是 1,000,000 字节;`0.000001MB` 是一个字节,`0` 是有效边界。
- `format` 描述上传时探测出的**真实格式**,不是文件名后缀,也不会扩大上传权限。
- 日期指上传时间(`created_at`),不是 EXIF 拍摄时间;时区取浏览器设置并随链接传递,夏令时切换日不假设 24 小时。
- 引号包住含空格或逗号的值,引号内只认 `\"` 和 `\\` 两个转义。`album:#42` 是 ID,`album:"#42"` 是名字;`album:unfiled` 是未分组,`album:"unfiled"` 是字面名。
- 重复的 `format`/`album` 会合并;重复的单值字段即使值相同也是错误;未知字段、写错的一部分条件都会明确报错,不会静默忽略。

## 兼容别名

为了照顾从蓝空习惯迁移过来的写法,少量别名会被规范化:

| 别名 | 规范化为 |
| --- | --- |
| `extension:jpg,png` | `format:jpg,png` |
| `is:public` / `is:private` | `visibility:public` / `visibility:private` |
| `order:earliest` | `sort:oldest` |
| `order:utmost` / `order:least` | `sort:size-desc` / `sort:size-asc` |
| `order:created_at` / `order:created_at-asc` | `sort:newest` / `sort:oldest` |

这是固定的几个别名,不是完整的蓝空语法兼容。`name:` 不是文件名限定词,`$旅行` 是普通文本。

## 接口与限制

查询走 `GET /api/images?qv=1&q=...&tz=Asia%2FShanghai&page=1&size=20`(相册详情页是 `/api/albums/{id}/images` 加同样参数)。`q` 必须存在但可以为空;`page` 1–10000,`size` 只能是 20 / 50 / 100。`qv=1` 与旧的关键字参数协议不能混用。

- 原始与规范化后的查询都不得超过 4096 UTF-8 字节,超出的规范化结果会返回 `QUERY_TOO_COMPLEX` 而不是截断执行。
- 原始查询最多 32 个词,普通文本词最多 16 个(去重后 8 个),每个文本值最多 200 个码点。
- 相册详情页的相册边界独立校验:清空或修改搜索词都不会越出这个相册,查询不能放大范围。
- 成功响应在 `data.search` 里带回规范化查询与应用的相册,前端据此校验过滤确实生效;重命名相册不会让旧链接失效(规范化引用用的是相册 ID)。
