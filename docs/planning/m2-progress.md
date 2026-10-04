# M2 实施与验收记录

计划：[M2 image core](../superpowers/plans/2026-10-04-m2-image-core.md)。基线 M1 `2f00a6f`；分支 `feat/m2-image-core`。

用户于2026-10-04确认：私有图片仅隐藏公共列表/画廊，直链可访问；启用脱敏时失败拒绝上传；容量按唯一云端 Key 的原图/WebP/缩略图实际字节计费。本轮按已授权的并行方式执行，由主代理完成整合与验证。

## 交付范围

| 计划任务 | 实际结果 |
| --- | --- |
| 1 固定环境 | Go1.27.1/vips8.18.6 实际链接；同版本启用 BMP/Magick；官方源码校验和固定，jemalloc，共享 /go 缓存，固定 MinIO 源码构建 |
| 2 路径与本机存储 | 模板、清洗、防穿越、原子不覆盖、归属、复制和中断 staging 清理；race/fuzz通过，路径90.4% |
| 3 S3/版本清理 | 实际条件能力探测、服务端 multipart copy、归属补偿、版本/markers精确删除；真实MinIO通过 |
| 4 数据层 | 仅新增0002；双库预约/配额/碰撞/撤销/回滚/生命周期通过，0001哈希未变 |
| 5 图像处理 | 方向、透明度、动画、CMYK/BMP/TIFF/HEIC/AVIF及格式/像素限制；native race通过，84.4% |
| 6 元数据 | 完整容器归档，合成GPS/作者/序列号/MakerNote/XMP，无损清理与边界拒绝；冻结版scoped race通过，77.1% |
| 7 初始化 | AES-256-GCM、无密钥DTO/错误、init-local/init-storage/init-policy及默认组绑定 |
| 8 上传 | 短事务、唯一Key计费、SHA256回执、全部尝试Key补偿、提交不确定保护；upload.go整体81.7%，Upload函数80.7% |
| 9 原生接口 | 有界streaming multipart，单201/批量207，图片/EXIF/权限/回收站，本机/缩略图直出；OpenAPI18路径/145本地引用 |
| 10 回收站 | 原Key404后成功，恢复鉴权/配额重验，restore_cleanup，原Key/trash所有版本删除，路径生命周期锁 |
| 11 真组件联调 | 双库实际二进制+curl，双库真实HTTP/GPS隐私，真实MinIO加密配置/字节计费/回收站/版本清理 |
| 12 验证与审查 | 全量正常/race/lint/build/modverify通过；跨作者审查的P1均原始条件RED→GREEN；文档同步 |

## 验证证据

命令在 Docker Desktop Linux amd64 执行，PG和MinIO实际运行；没有用必要后端的skip代替通过。

- `docker compose -f deploy/compose.dev.yaml build dev minio`：exit0；实际Go1.27.1、vips8.18.6、pkg-config8.18.6。原锁定镜像BMP测试先失败（Magick关闭），同版本重编后通过；初次重编缺meson，补容器内meson/ninja后通过。
- `go mod tidy`、`go mod verify`：exit0，all modules verified。未升级锁定直接版本；smithy-go1.28.1是固定SDK已选定、现显式使用的依赖。
- `go test -mod=readonly ./... -count=1`：exit0，全部包通过。service27.646s、CLI8.579s、HTTP7.892s、repo3.789s。
- `go test -mod=readonly -race -coverprofile=.cache/m2-all.cover ./... -count=1`：exit0。service287.313s、HTTP81.282s、CLI23.171s。路径90.4%、图像84.4%、repo76.1%、storage75.5%、config90.7%。被仪器化包整体67.3%；子进程及从HTTP测试调用的native包未被此命令跨包仪器化，其0%不代表真组件测试跳过。
- 后续只增加认证证明/实时URL测试：`go test -race -coverprofile=.cache/m2-upload-final.cover ./internal/service -run Upload -count=1`：exit0。upload.go223/273语句=81.7%，Upload函数80.7%；没有修改生产行为。
- 固定 `golangci-lint@v2.14.0 run ./...`：exit0，0 issues。native整数转换用明确范围检查解决，无全局suppress。
- `go build -mod=readonly -trimpath -o bin/imgnest ./cmd/imgnest`：exit0，实际链接二进制。最终modverify再次通过。
- `TestActualProcessSmoke`：双库真实启动，实际curl上传，私有原/WebP/thumb直链，trash404/restore/purge、重启会话、Token吊销及正常退出全部PASS；curl认证配置走stdin，无密钥argv/日志。
- `TestRealMinIOEncryptedImageLifecycle`：隔离版本化桶、配置加密入库、实际连接检查、下载总字节等于计费、旧URL404、恢复、最终versions/markers为空且used_bytes=0，PASS。测试清理曾错误使用已取消t.Context，改为独立限时上下文后通过。
- 路径fuzz5s约83,178次执行PASS；协议测试含chunked multipart尾部总量上限、取消后native工作仍占slot、有界字段/文件、无临时源文件及错误脱敏。

## RED→GREEN 与跨作者审查

数据、路径/存储、图像/元数据由独立worker实现；主代理编排业务与CLI，路径worker后续完成HTTP。跨作者只读审查聚焦服务隐私、授权、状态与并发；以下发现已关闭。

1. 缺API/占位行为先RED，再完成路径、存储、迁移、图像/EXIF、上传及回收站。双库真实覆盖配额/路径竞争、同Key去重、溢出、事务回滚和来源Token撤销。
2. 固定MinIO忽略普通CopyObject目标条件；改为条件multipart completion，真实412冲突、不覆盖、版本删除通过。最终purge先验证全部实体归属，外部历史零删除。
3. native加载器修复缺少终止标记的文件；先失败，再严格扫描拒绝。TIFF元数据别名像素、JPEG扫描间元数据、ISOBMFF extent越出IDAT均实际RED→GREEN。
4. webp_only尺寸描述源而非保存主图：先RED，重新Probe primary后GREEN。重命名候选重验100rune/255byte限制。泄漏断言保留原Upload错误，避免被Stat错误覆盖。
5. 同owner、同长度、不同bytes垃圾箱被误认有效并删除源：实际Local回归RED，改由Driver.Copy校验内容后GREEN。源已404重试用持久SHA256流式校验。
6. 旧预览回填暂停时purge释放路径，可能覆盖新图缓存（P1）：pausedPut先RED，锁后查记录/权限并持生命周期锁至缓存写完，race GREEN。
7. 前台补偿与Sweep重复清理，可能提前释放路径误删替换对象（P1）：pausedCache.Delete先RED；登记cleanup，锁内复读当前state/operation，全IO及Finish持锁，race GREEN。锁等待超时仍有日志供重试。

## 决定与实际限制

- 原图脱敏不重编码；WebP源复用同Key。src_md5/EXIF来自源，主图尺寸/ext/size/hash来自实际保存版本；派生WebP仅留ICC、thumb无元数据。
- 本机持久对象是owned-envelope，Driver.Open返回图片bytes；缓存仍是普通WebP。备份保留整个根，不能直接静态输出物理对象文件。
- 最终删除先验证全部实体版本归属，再逐VersionID及marker删除；补偿只删本次owner与staging。没有整桶清空或外部覆盖。
- 无GetBlob。opaque classicTIFF可能在私有raw中保存明确标记的≤20MiB full-source-fallback，启用原图脱敏时拒绝；BigTIFF/不支持ISOBMFF布局拒绝。样本为合成或native生成，未声称真实相机品牌验证。
- M2单实例/Linux amd64；真实B2/COS/R2账户未配置，MinIO不替代厂商联调；CDN缓存不处理。Vue/相册CRUD为M3，蓝空v1/完整管理为M4，多架构发布与迁移保留后续关口。
- 已同步spec§8.2、versions、pipeline Skill、OpenAPI、配置示例和开发说明。M2按整合批次提交，未按12个建议中间提交拆分；未推送/合并/tag，未操作旧蓝空数据。

## 提交记录

用户于2026-10-04恢复并完成Git收尾：敏感信息核对通过（非测试代码无真实凭证，测试凭证仅deploy/compose.dev.yaml的隔离值），新增文件与文档相对链接核对通过，bin/与.cache/保持忽略。M2整合批次以`024bb04`（feat: implement M2 image core pipeline）提交，共97个文件、13103行新增，含本progress与[任务交接](m2-handoff-2026-10-04.md)快照；本节由随后的docs提交单独记录。未推送/合并/tag；测试PG/MinIO容器仍处于停止状态。
