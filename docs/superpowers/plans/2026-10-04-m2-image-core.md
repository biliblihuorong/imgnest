# ImgNest M2 Image Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. 沿用用户确认的并行分工与当前环境原生 TDD/进度记录/末尾独立审查；缺失的上游工作流脚本不伪造调用。

**Goal:** 交付本机/S3 的同步图片上传、双版本/双缩略图、完整本地元数据、无损脱敏及可补偿回收站，真实 curl 和两库/MinIO/ libvips 测试通过。

**Architecture:** 使用 M1 鉴权证明与用户事务锁，pending image 行占路径并预约容量，网络 IO 在短事务外，完成/补偿通过 operation ID 幂等处理。service 只依赖 repo/storage/imaging/exif 接口；本机直出和 S3 直链的回收站都真实移除原 Key。

**Tech Stack:** Go1.27.1、libvips8.18.6/vipsgen1.3.11、imagemeta1.1.0、aws-sdk-go-v2/S3 版本完全按 versions.md；已有 Gin/GORM/koanf/Cobra，标准库 crypto/os.Root。MinIO 测试工具固定正式 tag RELEASE.2025-10-15T17-29-55Z 从源码构建，不添加产品新依赖。

**Spec:** [spec.md](../../spec.md)、[M2设计补充](../specs/2026-10-04-m2-image-core-design.md)、[versions.md](../../versions.md)。基线为 M1 `2f00a6f`。新分支建议 `feat/m2-image-core`，从该基线开始，保留未推送的 M1 历史。

**Status:** 用户于2026-10-04确认三项取舍并授权执行；沿用并行实现/末尾审查，实际进度见 docs/planning/m2-progress.md。

## Global Constraints

- 依赖不升级；libvips/vipsgen必须匹配；Docker固定版本；不在 Windows 宿主链接 libvips。
- 不修改0001；仅新增版本化 SQL；PG/SQLite均实际验证，无 AutoMigrate。
- http→service→repo/storage/imaging/exif；业务方法context首参，框架接口除外；%w包装，敏感原始错误脱敏。
- 原生 code/message/data；用户鉴权证明在持久写入前重验；不能复制第二套上传 service 给未来 v1。
- 魔数+libvips；SVG拒绝；总像素100,000,000；转换同步、CPU信号量；WebP Q80/effort4，thumb400/Q75。
- 全量 EXIF/XMP/未知元数据本地保存；云端无损改写；GPS/raw仅本人/admin；公开 DTO不含EXIF。
- Key为path.ext/path.webp/path_thumbs.webp；URL实时构造；保留命名空间/后缀；同Key唯一计费。
- 任意写/事务失败清理本次全部对象；不能覆盖或误删别人的对象；提交不确定先查状态。
- 回收站7天，源站旧URL404，恢复路径不被抢占；B2全部版本及标记真实清理。
- 首阶段不引入分布式队列、调度框架或通用CRUD代码生成；操作记录用image行，任务由现有单体执行。

## Review Focus

1. 同名与配额竞争、int64溢出：预约事务/实际对象去重，Tasks4/8覆盖。
2. 存储超时实际已落盘、事务提交确认丢失、补偿失败：不破坏别人/已提交对象，Tasks2/4/8覆盖。
3. 多个元数据块、MakerNote身份信息、循环/越界IFD、源像素被改写：完整保留与隐私正确，Task6覆盖。
4. 撤销之后完成的上传/恢复、旧操作清理覆盖新操作：证明与operation_id边界，Tasks4/8/10覆盖。
5. 删除/恢复/purge竞争、B2原Key历史版本、缺失缓存：路径/配额/404/清理正确，Tasks3/10/11覆盖。

## 文件边界与并行顺序

| 工作区 | 文件 | 负责内容 |
| --- | --- | --- |
| 父代理 | deploy、go.mod/go.sum、internal/cli、internal/http、docs、进度/Git | 固定环境、接线、API、验收 |
| 路径/存储 worker | internal/pathtpl、internal/storage | 模板、本机、安全S3与版本清理 |
| 图像/元数据 worker | internal/imaging、internal/exif、testdata | libvips、容器元数据提取与无损清理 |
| 持久化/业务 worker | internal/model、internal/repo、internal/service | 迁移契约、预约、上传/回收站编排 |

先冻结共享类型，再并行路径/驱动与图像/元数据；父代理负责所有公共依赖改动，workers不改锁文件、Git或他人目录。业务 worker先完成数据层，再利用测试接口编排；父代理最后做实际组件接线。不得把整个M2拆成互不兼容的独立脚手架。

## Shared Interfaces

新模型 Storage/Policy/Image/ImageExif/Album 的基本字段按spec§4；Image增加 State、Operation、OperationID、ChargedBytes 和对象回执，不保存URL。Exif.Raw含原始块归档与解码字段；敏感字段明确不进入普通ImageView。

- `pathtpl.Build(ctx context.Context, pathTpl, nameTpl string, vars Variables) (Result,error)`；Variables含Time、UserID、Filename、MD5、SHA1、Random io.Reader；Result含Path string、HasRandom bool。`Validate(ctx,...templates) error`、`Sanitize(ctx,string)(string,error)`。
- `storage.Driver`：`PutNew(ctx,key string, body io.ReadSeeker, opts PutOptions)(Receipt,error)`、`Open(ctx,key string)(io.ReadCloser,ObjectInfo,error)`、`Stat(ctx,key string)(ObjectInfo,error)`、`Copy(ctx,source,target string,opts CopyOptions)(Receipt,error)`、`DeleteCurrent(ctx,key string)error`、`PurgeAllVersions(ctx,key string)error`。Receipt含Key/Size/VersionID/OwnerID，ObjectInfo含Size/MIME/归属信息；opts的OwnerID不是密钥，不存GPS。
- `imaging.Processor`：`Probe(ctx,data []byte)(Info,error)`、`Process(ctx,data []byte,opts Options)(Result,error)`。Info含真实Format/Ext/MIME、视觉Width/Height、LoadedFrames、Orientation；Result含Info、WebP/Thumbnail字节。源WebP是否复用由上传编排按统一Key规则决定。
- `exif.Extractor`：`Extract(ctx,data []byte,info imaging.Info)(model.ImageExif,error)`；`Scrubber`：`Scrub(ctx,data []byte,format,mode string)([]byte,error)`。读取与清理都不调用未经验证的 GetBlob 所有权路径。
- `ImageRepository`：`ReserveUpload(ctx context.Context,req model.UploadReservation)(model.Image,error)`、`RecordObjectReceipt(ctx context.Context,key,operationID string,receipt model.ObjectReceipt)error`、`CommitUpload(ctx context.Context,key,operationID string,exif model.ImageExif,grant model.TokenGrant)(model.Image,error)`、`StartCleanup(ctx context.Context,key,operationID string)error`、`FinishCleanup(ctx context.Context,key,operationID string)error`；过期/已提交操作不允许重复变更。UploadReservation携带真实处理结果、对象清单、鉴权证明和计费金额。
- PolicyRepository提供 `UploadPolicy(ctx,userID,policyID uint64)(model.Policy,model.Storage,model.Group,error)`；StorageProvider按已有模型返回驱动，不让service依赖具体S3/GORM。
- `UploadService.Upload(ctx,subject TokenSubject,input UploadInput)(ImageView,error)`，opaque subject复用M1；ImageView仅包含普通字段、实时links和本地thumb链接，无Exif。
- ImageService提供 `List(ctx,subject,query)`、`Exif(ctx,subject,key)`、`Trash(ctx,subject,keys)`、`Restore(ctx,subject,keys)`、`Purge(ctx,subject,keys)`；repo的begin/finish转换统一operation_id。后台处理使用显式内部任务权限，不伪造用户证明。

共享值类型如下，所有金额int64、数据库ID为uint64且限制signedBIGINT、时间UTC；具体模型的spec字段按蛇形JSON/GORM列映射，敏感值不进入公共DTO：

| 类型 | 精确字段 |
| --- | --- |
| pathtpl.Variables | Time time.Time；UserID uint64；Filename/MD5/SHA1 string；Random io.Reader |
| storage.PutOptions/CopyOptions | MIME/OwnerID/CacheControl string；只允许新目标Key；Reader由调用者保持到写入结束 |
| storage.Receipt | Key/VersionID/OwnerID string；Size int64 |
| storage.ObjectInfo | Size int64；MIME/OwnerID/VersionID string |
| imaging.Info | Format/Ext/MIME string；Width/Height/LoadedFrames/Orientation int |
| imaging.Options | WebPMode string；Quality/Effort/MaxWidth/MaxHeight/ThumbSize int；Lossless/SkipIfLarger/ThumbEnabled bool |
| imaging.Result | Info imaging.Info；WebP/Thumbnail []byte |
| service.UploadInput | Data []byte；Filename/IP string；PolicyID/AlbumID uint64；IsPublic bool |
| model.UploadReservation | Image model.Image；Objects []model.ObjectReceipt（写意图时VersionID为空）；Grant model.TokenGrant；operation由服务层生成，不由HTTP客户端提供 |
| model.ObjectReceipt | Key/VersionID/OwnerID string；Size int64 |
| model.Image操作扩展 | State/Operation/OperationID string；ChargedBytes int64；ObjectManifest json.RawMessage；其余spec字段与索引保留 |

为避免repo反向引用storage实现，共享对象意图/回执的持久字段声明在model（ObjectReceipt），storage结果在service转换为该plain值，repo接口消费model.ObjectReceipt。所有消费者使用同一字段命名，变更必须通知。

## Task 1: 固定图像/测试环境与契约

**Files:** deploy/Dockerfile.dev、deploy/Dockerfile.minio-test、deploy/compose.dev.yaml、go.mod/go.sum、docs/versions.md、docs/planning/m2-progress.md；新增接口类型文件。

- [ ] 用户确认后同步推荐决定及技术补充到 spec/项目Skill；冻结类型/错误码并检查所有消费者一致；建立分支和进度基线。
- [ ] 从锁定 imagor-base-dev 复制Go1.27.1工具链，明确cgo/pkg-config/动态库路径；MinIO工具源码固定tag与提交，仅隔离测试网络。
- [ ] 写隔离libvips生命周期/加载/关闭检查，先运行确认缺环境/接口的RED；再配置启动与Close，使原生检查通过，不以manifest查询替代链接成功。
- [ ] Run `docker compose -f deploy/compose.dev.yaml build dev minio`；Expected exit0；容器 `go version`=1.27.1、`vips --version`=8.18.6，MinIO报告固定源码版本。
- [ ] 仅加入本批使用的已锁定模块，tidy/verify；记录实际间接依赖，提交 `chore: enable pinned libvips and S3 test environment`。

## Task 2: 路径模板与本机驱动

**Files:** internal/pathtpl/{template,sanitize}_test.go及对应实现；internal/storage/{driver,local}.go与测试。

- [ ] 变量表全覆盖：固定Time2026-10-04生成2026/10/04；MD5/16、SHA1、uid、UUID/rand；未知变量、rand0/65、hash0/33拒绝。

```go
result, err := pathtpl.Build(t.Context(), "{Y}/{m}/{d}", "{filename}", pathtpl.Variables{
    Time: time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC), Filename: "旅行.png",
})
if err != nil || result.Path != "2026/10/04/旅行" { t.Fatalf("path contract failed") }
```
- [ ] 清洗中文字节边界、点段/控制符/斜线/保留路径与 `_thumbs`；同一输入与模板不会产生空/逃逸路径。
- [ ] local PutNew拒绝已存在Key且不改变旧bytes；中途写失败无半文件；os.Root拒绝../与symlink逃逸；copy/delete/purge重试幂等。

```go
// 已先PutNew key="2026/a.png", body="first"；再次PutNew同Key="second"。
if !errors.Is(err, storage.ErrExists) { t.Fatal("existing key was overwritten") }
if string(readBack) != "first" { t.Fatal("conflict changed original bytes") }
```
- [ ] RED→实现→GREEN；Run `docker compose -f deploy/compose.dev.yaml run --rm dev go test -race -cover ./internal/pathtpl ./internal/storage -count=1`；pathtpl覆盖≥80，Expected exit0。
- [ ] 提交 `feat: implement safe image paths and local objects`。

## Task 3: S3驱动与真实版本清理

**Files:** internal/storage/s3.go、s3_test.go、testdata；存储预设配置类型。

- [ ] 先写协议测试，确认endpoint/path-style/ContentLength/checksum/metadata不泄密；Put条件冲突不覆盖，超时已提交通过归属回执处理。
- [ ] 实现上述Driver窄接口；Purge列出Key所有版本和delete markers，分页处理，只删精确Key，带VersionId逐一删除；不调用整桶清空。
- [ ] MinIO集成开启versioning、生成多个原Key/trash版本，put/copy/delete后验证旧直链404、purge后版本列表为空。厂商能力差异不由MinIO通过推断。
- [ ] Run `docker compose -f deploy/compose.dev.yaml up -d --wait minio` 后 `docker compose -f deploy/compose.dev.yaml run --rm dev go test ./internal/storage -run S3 -count=1 -v`；Expected实际MinIO案例PASS，无skip替代。
- [ ] 提交 `feat: add compatible S3 objects and version-aware purge`。

## Task 4: 图片迁移、规则与预约事务

**Files:** model存储/规则/图片/Exif/相册类型；两库0002_image_core.sql；repo/policy、image与生命周期测试。

- [ ] 先写M1→M2升级、fresh/twice/checksum、表约束/类型、unique跨pending/active/trash测试，0001哈希必须不变。
- [ ] 用户锁下SUM预约金额并校验used+reserved，不增加第二缓存计数；同路径/剩余配额竞争恰一成功；计费去重和int64边界。

```go
// 容量1000，已用600；两个屏障同步请求，各预约300。
if successes != 1 || quotaFailures != 1 { t.Fatal("concurrent quota reservation escaped limit") }
// src为WebP且两种逻辑版本指向同Key，100bytes+20bytes云thumb，总额120。
if image.ChargedBytes != 120 { t.Fatal("one physical WebP key charged twice") }
```
- [ ] Reserve/receipt/commit/cleanup幂等；认证撤销后commit拒绝；提交确认丢失通过operation状态确认而非删已成功对象。
- [ ] RED→实现→GREEN；Run `docker compose -f deploy/compose.dev.yaml run --rm dev go test ./internal/migrate ./internal/repo -count=1 -v`，PG/SQLite均真实PASS。
- [ ] 提交 `feat: reserve image paths and quotas transactionally`。

## Task 5: libvips同步 WebP 与缩略图

**Files:** internal/imaging/{processor,vips}.go、测试与样本。

- [ ] 先用JPEG方向6、PNG alpha、动态GIF、动态WebP、CMYK、BMP/TIFF/HEIC/AVIF、1x1、截断和超像素文件测试魔数+probe与LoadedFrames。
- [ ] Process仅缩小不放大、视觉旋转、动画帧保留；webp80/effort4/ICC-only、thumb第一帧400/Q75/no-meta；输入WebP复用及only模式例外有断言。
- [ ] 信号量/取消/资源Close检查；无操作缓存；Q/effort/keep明确设置，不能用库默认0或Q75冒充项目参数。
- [ ] RED→GREEN；Run `docker compose -f deploy/compose.dev.yaml run --rm dev go test -race -cover ./internal/imaging -count=1`；Expected exit0，覆盖≥80。
- [ ] 提交 `feat: process oriented animated WebP and local thumbnails`。

## Task 6: 完整元数据与无损敏感项清理

**Files:** internal/exif/{extract,container,tiff,scrub}.go、测试样本；项目scrub引用说明。

- [ ] 本地raw保留原EXIF/XMP/MakerNote/未知块；imagemeta补常见字段，单用Decode不能宣称全量。
- [ ] GPS/标准与MakerNote序列号/作者/XMP清理；all保方向ICC；多个APP1/EXIF、extendedXMP、PNG CRC、WebP RIFF flag与恶意IFD偏移/循环。
- [ ] 断言原压缩像素区完全相同、重新解码pixels相同、敏感bytes/字段不存在、非敏感相机参数保留；故障按用户选择执行。

```go
if !bytes.Equal(beforeCompressedPixels, afterCompressedPixels) { t.Fatal("original reencoded") }
if after.GPSLat != nil || after.GPSLng != nil { t.Fatal("location retained in cloud original") }
if after.Make != before.Make || after.Model != before.Model { t.Fatal("camera fields lost") }
```
- [ ] RED→GREEN；Run `docker compose -f deploy/compose.dev.yaml run --rm dev go test -race ./internal/exif -count=1 -v`；真实/合成样本逐类记录，不能假称未提供相机品牌样本已验证。
- [ ] 提交 `feat: archive full image metadata and scrub originals losslessly`。

## Task 7: 加密存储配置与可用规则初始化

**Files:** service/storage、policy；repo/storage；config.security；CLI存储/规则初始化；测试。

- [ ] AES-GCM随机nonce、篡改拒绝、错误/DTO不含原密钥；主密钥32bytes来自部署配置；不提交真实凭证。
- [ ] 规则保存时validate模板/参数；组策略/defaultpolicy与enabled状态一致；连接测试写/读/复制/删除，测试Key归属可确认且清理。
- [ ] 本机默认storage/policy与组绑定；S3配置由stdin/未跟踪文件提供，CLI参数不含secret。
- [ ] 双库与驱动契约 RED→GREEN；Run `docker compose -f deploy/compose.dev.yaml run --rm dev go test ./internal/service ./internal/cli -count=1 -v`。
- [ ] 提交 `feat: configure encrypted storages and upload policies`。

## Task 8: 上传编排与补偿

**Files:** internal/service/upload.go、upload_test.go；需要的窄接口与无Exif ImageView。

- [ ] 正常Both/Only/None、WebP同Key、skip larger、thumb开关、HEIF模式、实际size/md5/sha1、链接实时变更。
- [ ] 第二/第三次对象写、localthumb、Exif/最终事务、认证撤销、取消、超时已写、重复commit和cleanup失败注入；断言只清理本次对象、不误删已提交对象、路径不提前释放。

```go
// 第二对象写入故障后检查真实测试存储和数据库，不对mock本身断言。
if ownedObjectCount != 0 || activeImageCount != 0 { t.Fatal("failed upload left active artifacts") }
if existingOtherImageBytesChanged { t.Fatal("compensation removed another image") }
```
- [ ] Upload按设计短事务+存储IO+commit编排；cleanup独立限时context且包含所有尝试写入；单实例重启恢复遗留操作。
- [ ] RED→GREEN；Run `docker compose -f deploy/compose.dev.yaml run --rm dev go test -race -cover ./internal/service -run Upload -count=1 -v`；上传函数覆盖≥80，未执行的全套随后Task12跑。
- [ ] 提交 `feat: upload image variants with compensating cleanup`。

## Task 9: 原生上传、图片/EXIF与本机出图

**Files:** native/upload、image、thumb；router；OpenAPI；HTTP/实际进程测试。

- [ ] multipart真实Body总限额/文件限额/批量结果；字段后缀Content-Type欺骗；匿名拒绝、策略/相册归属、GPS/raw仅owner/admin、普通DTO没有Exif。
- [ ] 本机访问prefix只出active声明对象；内部trash/pending/不存在404；原图/WebP/云thumb链接真实可开；本地thumb受owner/admin或public规则保护、缺失懒回填。
- [ ] 迁移/初始化/login/curl上传的实际二进制流程；源URL更改后links即变，不存URL。
- [ ] RED→GREEN；Run `docker compose -f deploy/compose.dev.yaml run --rm dev go test ./internal/http/... ./internal/cli -count=1 -v`；两库/真实vips驱动，无假实现替代最终验证。
- [ ] 提交 `feat: expose native image upload and authorized metadata`。

## Task 10: 删除、恢复与彻底清理

**Files:** service/image、trash；repo/image_operation；HTTP trash；任务扫描与测试。

- [ ] 原Key真实移除后成功返回，扣费一次；恢复检查当前配额、路径唯一、撤销旧请求不生效；保留本地预览。
- [ ] Delete/restore/purge竞争，部分copy/delete失败，operation互斥与重试；trash0、7天边界、每小时扫描，清理完成后才删行/释放路径。
- [ ] S3 purge同时处理原Key遗留历史版本与trash所有版本/delete markers，不能删刚恢复active的对象。
- [ ] RED→GREEN；Run `docker compose -f deploy/compose.dev.yaml run --rm dev go test ./internal/repo ./internal/service ./internal/http/... -run 'Trash|Restore|Purge' -count=1 -v`；两库和MinIO实际案例PASS。
- [ ] 提交 `feat: recycle restore and purge image objects safely`。

## Task 11: 端到端故障与恢复关口

**Files:** 现有integration/process测试、testdata、M2操作说明。

- [ ] 真实local/MinIO上传→原/WebP/thumb可解码→localthumb→删除旧URL404→恢复同URL→purge全部版本；普通Owner不能读别人GPS/raw。
- [ ] 存储失败、数据库最终失败、context取消、重启恢复cleanup、重试幂等、并发Quota/同Path和来源Token撤销均有确定性屏障。
- [ ] 断言所有原始敏感值不进stdout/stderr/slog；MakerNotes/GetBlob路径以隔离原生检查验证替代假设。
- [ ] Run `docker compose -f deploy/compose.dev.yaml run --rm dev go test ./... -count=1`；Expected exit0，真实驱动与双库测试未skip。
- [ ] 提交 `test: exercise real image lifecycle and failure recovery`。

## Task 12: 全量验证与独立审查

- [ ] PG/MinIO准备后跑完整suite/race、锁定lint、gofmt、modverify、带真实libvips链接的build；保留每次失败名称和实际修复，不用旧快照替代。
- [ ] pathtpl/imaging/upload覆盖≥80；记录各类样本、版本/资源限制、厂商实际未联调项；外部CDN缓存不纳入源站404证明。
- [ ] 独立审查全分支；重要问题在原始输入/故障条件下RED→GREEN，完整suite再绿色。
- [ ] 同步spec/versions/Skill/OpenAPI/README，完成M2 progress、代码提交和测试容器清理；不推送/合并未授权远端。

## 当前交接

本计划已基于M1与官方锁定源码完成初次自审。用户尚需审阅三项产品取舍和本书面计划；并行方式已确定，不再重复询问执行方式。确认后从Task1连续执行至Task12，再整体汇报。
