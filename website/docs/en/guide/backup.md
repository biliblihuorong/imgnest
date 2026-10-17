# Backup and restore

Admin → Settings → Backup/Export provides manual and scheduled backups; restore is command-line only, so the target database ends in a consistent state.

## What is in an archive

Each backup is a zip named `imgnest-YYYYMMDD-HHMMSS[-auto].zip`:

| File | Content |
| --- | --- |
| `database.sql` | INSERTs of every data table in foreign-key order |
| `config/imgnest.yaml` | The config file passed via `--config` at startup |
| `config/imgnest.env` | All `IMGNEST_*` environment variables |
| `manifest.json` | Format, database driver, schema version, per-table row counts |

**Archives contain password hashes, the database DSN and `security.master_key`** — treat them as secrets and download them as an administrator only. Backups are written under `backup.dir` (default `data/backup`, override with `IMGNEST_BACKUP_DIR`) with directory mode `0700` and file mode `0600`; writes go to a temporary file first and are renamed, so a failure never leaves a partial archive.

## Scheduled backups

Configured on the backup page and stored server-side:

| Setting | Range | Default |
| --- | --- | --- |
| `interval_hours` | 1–720 hours | 24 |
| `keep` | Keep the latest 1–365 | 7 |

The server checks every 5 minutes and writes an `-auto` archive when due; cleanup only removes surplus automatic archives — manual backups are never deleted automatically.

## Restoring

```bash
# 1. Stop the service, then:
imgnest backup restore --yes imgnest-20261017-093000.zip
```

- The target database must already be migrated by the **same version**; a driver or schema mismatch is refused (error 30021).
- Restore empties every data table and re-imports inside one transaction, executing only INSERT/UPDATE statements from the backup format; PostgreSQL sequences are realigned afterwards.
- Config files are **not** overwritten automatically — copy them back from the archive by hand if needed.
- `imgnest backup create` works standalone for cron, equivalent to the scheduled console backups.

## Admin API

Administrators only: `GET /api/admin/backups` (list and schedule), `POST /api/admin/backups` (run now), `GET /api/admin/backups/{name}` (download), `DELETE /api/admin/backups/{name}`, `PUT /api/admin/backups/schedule`. Only one backup task may run at a time (30020).
