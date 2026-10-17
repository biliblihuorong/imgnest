# 备份与还原

后台「系统设置 → 备份/导出」提供手动与定时备份;还原只走命令行,保证目标库处于一致状态。

## 归档里有什么

每个备份是一个 zip(`imgnest-YYYYMMDD-HHMMSS[-auto].zip`):

| 文件 | 内容 |
| --- | --- |
| `database.sql` | 全部数据表的 INSERT,按外键顺序导出 |
| `config/imgnest.yaml` | 启动时 `--config` 指定的配置文件 |
| `config/imgnest.env` | 所有 `IMGNEST_*` 环境变量 |
| `manifest.json` | 格式、数据库驱动、schema 版本、各表行数 |

**归档包含密码哈希、数据库 DSN 和 `security.master_key`**,只供管理员下载,务必按密钥保管。备份写在 `backup.dir`(默认 `data/backup`,可用 `IMGNEST_BACKUP_DIR` 覆盖),目录 `0700`、文件 `0600`,写盘先落临时文件再改名,失败不会留半截文件。

## 定时备份

在备份页设置,存在服务端:

| 项 | 范围 | 默认 |
| --- | --- | --- |
| `interval_hours` | 1–720 小时 | 24 |
| `keep` | 保留最近 1–365 份 | 7 |

服务每 5 分钟检查一次,到期生成带 `-auto` 后缀的归档;清理只删多出来的自动归档,手动备份永不自动删除。

## 还原

```bash
# 1. 停止服务,然后:
imgnest backup restore --yes imgnest-20261017-093000.zip
```

- 目标库必须已经用**同一版本**迁移过;驱动或 schema 版本不匹配会拒绝(错误码 30021)。
- 还原在一个事务里清空全部数据表再导入,只执行备份格式里的 INSERT/UPDATE;PostgreSQL 的自增序列随后自动对齐。
- 配置文件**不会**被自动覆盖,需要时从归档里手工拷回。
- `imgnest backup create` 也可以单独放进 cron,和管理台的定时备份等价。

## 管理接口

管理员专用:`GET /api/admin/backups`(列表与定时设置)、`POST /api/admin/backups`(立即备份)、`GET /api/admin/backups/{name}`(下载)、`DELETE /api/admin/backups/{name}`、`PUT /api/admin/backups/schedule`。同一时间只允许一个备份任务进行中(30020)。
