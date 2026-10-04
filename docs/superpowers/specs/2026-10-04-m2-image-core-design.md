# M2 图片、存储与回收站设计补充

依据 [spec.md](../../spec.md) §4–7、[versions.md](../../versions.md) 及项目 image-pipeline Skill。本文为已确认的M2补充，采纳值同步到 spec 与 Skill；spec 仍为唯一事实来源。

## 目标与边界

完成本机/S3 存储、路径模板、同步 WebP、双缩略图、本地完整元数据、原图无损脱敏、上传补偿和基本回收站。登录用户能通过原生 API 上传、列图、看本人 EXIF、删除、恢复和彻底删除；curl 得到实际可访问的原图/WebP/缩略图地址。继续使用 M1 的不透明鉴权证明，持久写入不能仅凭旧 middleware 的 userID 授权。

Vue 页面、蓝空 v1 路由、游客上传、公开画廊、完整相册界面、旧蓝空迁移、异步历史补处理和正式发布链不属于本批。相册表及上传归属校验在 M2 先具备。

## 用户已确认的产品取舍

| 问题 | 推荐并写入本方案的值 | 另一选择的影响 |
| --- | --- | --- |
| 私有图 | 只隐藏公共列表/画廊，持有直链仍可访问；与 S3/CDN 直接出图架构一致 | 若直链也要鉴权，应改为私有桶签名 URL 或应用代理，需另定访问与缓存契约 |
| 脱敏失败 | 开启 gps/all 时拒绝本次上传；none 明确不脱敏 | 原样保存会让默认隐私规则可能失效，必须由用户明确接受 |
| 容量 | 计入实际存储对象总字节：原图、WebP、云端缩略图；同 Key 只计一次，本地缓存不计 | 仅计主版本需明确衍生图不计入用户配额；并发预约采用同一口径 |

用户已于2026-10-04逐项确认推荐值。

## 可直接按规范收敛的技术选择

- 新迁移 0002 创建 storages、policies、group_policies、albums、images、image_exif，并新增 groups.default_policy_id。0001 原样保留。
- policies 补齐 webp_effort、strip_meta、skip_if_larger、heif_mode、on_conflict。M2 衍生 WebP 强制移除 EXIF/XMP/GPS、只保留 ICC；缩略图移除所有元数据。
- 默认目录 `{Y}/{m}/{d}`、名字 `{uniqid}`、WebP both/Q80/effort4、缩略图400/Q75、scrub=gps、link_prefer=webp、heif=webp_only。源 WebP 不重复转换，同一物理 Key 不重复写入/计费/删除；webp_only 不因产物变大而丢弃唯一版本。
- 默认上传文件上限20 MiB、请求总上限64 MiB、像素总上限100,000,000（包括动图帧）、同步请求最长5分钟；组可进一步限制。原生批量上传每文件独立补偿，已成功文件不因另一文件失败回滚。
- 所有源格式以魔数+libvips 探测为准，SVG 默认拒绝。AVIF/HEIC 还须按 ftyp 品牌区分，不能仅依据 vips loader-family enum。
- 路径每段≤100 runes、完整无后缀 path≤255 UTF-8 bytes；清洗控制字符、危险分隔/查询字符和点段，保留中文。超限拒绝，不静默截断。保留命名空间 `_trash`、`.trash` 不能由用户模板生成。
- `_thumbs` 为保留文件名后缀：随机文件名重渲染最多5次，确定性文件名追加 `-1`；普通冲突遵循 rename/reject。随机目录不能解决确定性文件名的保留后缀问题。
- 全部 URL 实时由 storage.base_url 和对象 Key 构造，逐路径段正确转义；URL 不入库。

## 配额、路径和操作记录

使用 pending 图片行兼任预约，不另建 reservations 表。images 新增 state（pending/active/trash）、operation（空/upload/cleanup/trash/restore/purge）、operation_id、charged_bytes、对象清单/回执及错误分类；保留 `(storage_id,path)` 对所有状态唯一。

M2 首选在用户锁内汇总该用户 pending/restore 预约金额，避免给 users 增加第二个容量计数器。容量判断为 used + reserved + 本次金额，计算检查 int64 溢出；capacity=0 表示不限。若以后预约数量导致查询成本明显上升，再增加可重建的 reserved_bytes 计数。

1. 上传体限额/组规则预检→真实格式/像素→完整元数据本地提取→生成脱敏原图、WebP、缩略图→得出实际去重 Key 和字节数。
2. 短事务锁用户、复查不透明鉴权证明/组规则/配额、插入 pending 并占住路径。数据库决定冲突；事务不包住存储 IO。
3. 逐对象记录写入意图与回执，写云端和本地缩略图。存储以创建新对象语义工作，禁止盲目覆盖。
4. 短事务复查鉴权、预约所有权与规则，写 EXIF、pending→active、used_bytes 增加唯一对象总额。operation_id 使重复完成无二次扣费。
5. 任一步失败，先确认事务是否已经提交；提交结果不确定时查状态/幂等重试，不能直接删掉已成功记录的对象。确需补偿时清理本次拥有的全部对象，包括超时后可能已成功的写入。
6. 清理失败保留 pending 行、清单和路径预约，后台重试；全部确认删除后才释放路径。M2 明确单实例，启动时恢复前一进程遗留操作；不能只因 TTL 到期删除预约、允许旧写入者继续落对象。

服务层只消费接口。repo 负责事务、用户锁、唯一约束和状态变更；storage 负责对象语义；imaging/exif 负责字节处理。

## 存储驱动

本机用 os.Root 限定目录，临时文件写完再原子安装，拒绝路径穿越和 symlink 逃逸；创建新 Key 不能覆盖已存在文件。回收站文件保存在本机 `.trash`；缩略图缓存独立在 data/thumbs。

S3 SDK 使用锁定版本、显式 endpoint/region/path-style，request checksum 和 response checksum 都为 WhenRequired。配置密钥 AES-256-GCM 加密保存，主密钥来自未跟踪部署配置/环境，错误和响应脱敏。

Driver 区分 PutNew、Open/Stat、Copy、DeleteCurrent 与 PurgeAllVersions。新对象需要归属标记/版本回执，失败补偿不得删别人的对象。SDK 的 IfNoneMatch 字段确实存在；B2/COS/R2 实际能力必须在连接测试中探测，不能用 Head→Put 冒充原子防覆盖；不支持时明确报错或选择经审阅的可保证方案，不能静默覆盖。

MinIO 开启版本化验证 put/copy/delete/purge；仅用隔离测试桶/网络。真实 B2/COS/R2 账户联调需要显式测试配置，未配置时记录未验证，不以假服务声称厂商通过。

## 完整元数据与无损脱敏

imagemeta 用于常见字段解码，原始 EXIF/XMP/未知标签与 MakerNote 还须从 JPEG APP1、PNG eXIf/iTXt、WebP RIFF、TIFF/ISOBMFF 元数据范围提取并存本地 raw（原始块以 base64 编码保留）。解析失败不默默丢失已存在的元数据。

gps 默认移除 GPS、作者/所有人、标准序列号、全部 XMP；保守移除/清零 MakerNote 不透明载荷，因为其中也可能含厂家序列号/所有人。常见相机/镜头/曝光/时间/方向和 ICC 保留。all 只保留方向与 ICC。改写只触及元数据容器，绝不重新编码压缩像素；PNG 重算 CRC，WebP 更新 RIFF 长度/标志，处理多个同类块、extended XMP、恶意偏移和循环 IFD。

HEIC/AVIF 首版不做原位改写，按 heif_mode 只存 WebP（默认）/明确保留原样/拒绝。即便只存 WebP，本地仍保留输入的完整元数据。

脱敏后重新探测、再解析确认敏感项消失，并在样本上证明压缩像素数据未变化；失败行为按用户选择执行。GPS/raw 仅所有者和管理员能读取；公开图片 DTO、缩略图、未来画廊/v1 均不返 EXIF。

## 删除、恢复与物理清理

删除：短事务将 active→trash，记录操作、deleted_at/purge_at（默认7天）并只扣一次 used；事务外复制到 `_trash/` 并移除原 Key。对 S3 直链，成功响应必须等原 Key 不可访问。中途失败保留可恢复的操作记录，重试不重复扣费，本地缩略图保留。

恢复：在用户锁/图片操作锁内重新预约容量，复制回原 Key，再一次性转 active/增加 used；失败只清理本次恢复对象，保留回收站副本。预约仍占住路径，删除/恢复/purge 相互排斥。

Purge：确认全部原 Key、trash Key 的版本/删除标记及本地缓存已清理，再删 EXIF/image 行。B2 未带 versionId 的 Delete 只加删除标记，若逻辑移入回收站留下原 Key 历史版本，彻底删除必须也清它们。每小时扫描到期项，错误分类日志与幂等重试；trash_days=0 直接物理清理。

M2 不清理 CDN 已缓存内容，不把源站404等同于 CDN 全网即时失效。

## 原生交付与部署

- POST /api/upload：重复 file 字段或 files[]；policy_id、album_id 可选，is_public 默认 false。返回逐文件结果；单文件成功201，批量207且每项包含成功数据/错误码。两者统一使用 UploadService。
- GET /api/images、GET /api/images/{key}/exif、GET /t/{key}.webp、DELETE /api/images/{key}、GET /api/trash、POST /api/trash/restore、POST /api/trash/purge。
- 本地对象直出按已配置本机存储访问前缀查 active 图片记录，只输出其声明对象；不存在/回收站/内部路径返回404。WebP 被 skip 时按规范做本机302或链接回退。
- CLI 提供受主机管理员控制的存储/规则初始化，配置从 stdin/未跟踪文件读取，不将云密钥放命令行。默认本机规则与组绑定；完整管理 UI 仍在 M4。
- Docker.dev 使用 imagor-base vips8.18.6-r14-dev + Go1.27.1，进程启动明确设置缓存=0、项目Q80/effort4、VIPS并发/应用信号量，正确 Close 图像。

## 已核验的构建和 API 证据

- 两架构 libvips 开发镜像 manifest 可取，尚未实际构建/链接 M2。显式 Startup 必须早于任何隐式加载/HasOperation；Startup/Shutdown 不能反复用于同一进程。空 buffer 先拒绝，信号量覆盖实际编码与 Close，取消不能提前释放 C 仍持有的输入。
- 锁定 SDK PutObjectInput 有 ContentLength/IfNoneMatch/Metadata；厂商支持不由字段存在推断。[锁定源码](https://raw.githubusercontent.com/aws/aws-sdk-go-v2/service/s3/v1.114.0/service/s3/api_op_PutObject.go)
- B2 Delete 无 versionId 加标记，有 versionId 才物理删除。[官方说明](https://www.backblaze.com/apidocs/s3-delete-object)
- MinIO 社区仓库已归档，最新正式安全 tag 为 RELEASE.2025-10-15T17-29-55Z，提交 `9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a`；官方要求容器从源码构建。本项目据此固定测试工具，而非猜测不存在的镜像 tag。[发布说明](https://github.com/minio/minio/releases/tag/RELEASE.2025-10-15T17-29-55Z)
- vipsgen1.3.11 的 GetBlob 路径将借用的 libvips blob 指针送进会 g_free 的 helper；由源码推断可能悬空/重复释放，尚未原生复现。在隔离所有权测试前禁用这条读取路径，优先直接解析上传容器。GetICCProfile 使用复制路径。锁定版本不擅自升级。[vipsgen包装](https://raw.githubusercontent.com/cshum/vipsgen/v1.3.11/vips/vips.go)、[释放helper](https://raw.githubusercontent.com/cshum/vipsgen/v1.3.11/vips/util.go)、[libvips header.c](https://raw.githubusercontent.com/libvips/libvips/v8.18.6/libvips/iofuncs/header.c)
- imagemeta.Decode 不自动保留完整 XMP；loader-family Format 不能充分区分 AVIF/HEIC/BMP；加载帧数不能盲用文档总 Pages；WebP keep=0 是未设置、默认 Q75，必须明确项目 Q80、effort4 和 KeepIcc/KeepNone。[imagemeta入口](https://raw.githubusercontent.com/evanoberholster/imagemeta/v1.1.0/imagemeta.go)、[vipsgen](https://github.com/cshum/vipsgen/tree/v1.3.11/vips)

设计核对阶段未写业务代码；用户确认后进入实施，实际命令与结果记录在 M2 progress。
