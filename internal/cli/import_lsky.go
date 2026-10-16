package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	pathpkg "path"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/migrate"
	"github.com/biliblihuorong/imgnest/internal/migrate/lsky"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/spf13/cobra"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// lskySourceEnv names the source DSN variable, so its password stays out of
// shell history and process listings.
const lskySourceEnv = "IMGNEST_LSKY_DSN"

func importLskyCommand(path *string) *cobra.Command {
	var policyID uint64
	var sourceURL, sourceDir, zone, dump string
	var limit int
	var dryRun, skipImages, adopt bool
	cmd := &cobra.Command{
		Use:   "import-lsky",
		Short: "Import a Lsky Pro PostgreSQL database: accounts, tokens, albums, and every image re-processed through the upload pipeline",
		Long: "Reads the Lsky Pro database named by $" + lskySourceEnv + " (a PostgreSQL DSN) or a plain SQL export (--dump), maps\n" +
			"groups, users (bcrypt passwords kept), API tokens (ids kept when free, so PicGo keeps working) and albums, then\n" +
			"imports each image under the same path with the --policy-id rule: WebP, thumbnails, responsive variants and EXIF\n" +
			"are produced exactly as for a new upload. By default the original is downloaded from its old public link (or\n" +
			"--source-dir) and stored again; with --adopt the rule's storage is the old versioned S3 bucket and the original is\n" +
			"taken over in place, never rewritten. Reruns skip what is already imported. The web console's Lsky migration\n" +
			"page does the same while the server runs.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dsn := os.Getenv(lskySourceEnv)
			if dsn == "" && dump == "" {
				return fmt.Errorf("set %s to the Lsky PostgreSQL DSN, or pass --dump", lskySourceEnv)
			}
			if policyID == 0 && !dryRun {
				return errors.New("--policy-id is required")
			}
			if adopt && (sourceURL != "" || sourceDir != "") {
				return errors.New("--adopt reads the originals from the rule's storage; drop --source-url and --source-dir")
			}
			loc, err := time.LoadLocation(zone)
			if err != nil {
				return fmt.Errorf("--timezone: %w", err)
			}
			var snap lsky.Snapshot
			if dump != "" {
				snap, err = loadLskyDump(dump, loc)
			} else {
				snap, err = loadLsky(cmd.Context(), dsn, loc)
			}
			if err != nil {
				return err
			}
			out := &lineWriter{w: cmd.OutOrStdout()}
			out.printf("蓝空数据：%d 个组，%d 个用户，%d 个 Token，%d 个相册，%d 个存储策略，%d 张图片\n",
				len(snap.Groups), len(snap.Users), len(snap.Tokens), len(snap.Albums), len(snap.Strategies), len(snap.Images))
			if dryRun {
				return out.err
			}
			err = withDatabase(cmd.Context(), *path, func(db *gorm.DB, cfg config.Config) error {
				return runLskyImport(cmd.Context(), out, db, cfg, snap, lskyRun{policyID: policyID, sourceURL: sourceURL, sourceDir: sourceDir, limit: limit, skipImages: skipImages, adopt: adopt})
			})
			return errors.Join(err, out.err)
		},
	}
	cmd.Flags().Uint64Var(&policyID, "policy-id", 0, "ImgNest upload rule whose storage receives the images (bound to every imported group)")
	cmd.Flags().StringVar(&sourceURL, "source-url", "", "public base URL of the old images (default: each Lsky strategy's URL)")
	cmd.Flags().StringVar(&sourceDir, "source-dir", "", "read old images from this local directory instead of downloading")
	cmd.Flags().StringVar(&zone, "timezone", "Asia/Shanghai", "time zone Lsky stored its timestamps in")
	cmd.Flags().IntVar(&limit, "limit", 0, "import at most this many images in this run (0 = all)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "only read and count the Lsky data")
	cmd.Flags().BoolVar(&skipImages, "skip-images", false, "import accounts, tokens and albums only")
	cmd.Flags().BoolVar(&adopt, "adopt", false, "take the originals over where they are: the rule's storage must be the old S3 bucket (versioned); originals are never rewritten, only WebP, thumbnails and variants are added")
	cmd.Flags().StringVar(&dump, "dump", "", "read the Lsky data from this plain-SQL export instead of $"+lskySourceEnv)
	return cmd
}

func loadLsky(ctx context.Context, dsn string, loc *time.Location) (lsky.Snapshot, error) {
	source, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		// The driver error can echo the DSN; never print it.
		return lsky.Snapshot{}, errors.New("open Lsky database: connection failed")
	}
	sqlDB, err := source.DB()
	if err != nil {
		return lsky.Snapshot{}, errors.New("open Lsky database: connection failed")
	}
	defer func() { _ = sqlDB.Close() }()
	return lsky.Load(ctx, sqlDB, loc)
}

type lskyRun struct {
	policyID             uint64
	sourceURL, sourceDir string
	limit                int
	skipImages, adopt    bool
}

func loadLskyDump(name string, loc *time.Location) (lsky.Snapshot, error) {
	file, err := os.Open(name) // #nosec G304 -- the operator names the export to read.
	if err != nil {
		return lsky.Snapshot{}, fmt.Errorf("open --dump: %w", err)
	}
	defer func() { _ = file.Close() }()
	return lsky.ParseDump(file, loc)
}

func runLskyImport(ctx context.Context, out *lineWriter, db *gorm.DB, cfg config.Config, snap lsky.Snapshot, run lskyRun) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("access database: %w", err)
	}
	if err := migrate.Check(ctx, sqlDB, cfg.Database.Driver); err != nil {
		return err
	}
	var policy model.Policy
	if err := db.WithContext(ctx).First(&policy, "id = ?", run.policyID).Error; err != nil {
		return fmt.Errorf("find policy %d: %w", run.policyID, err)
	}
	mapping, accounts, err := lsky.ImportAccounts(ctx, db, snap, lsky.Options{PolicyID: run.policyID, Adopt: run.adopt})
	if err != nil {
		return err
	}
	out.printf("组：新建 %d，合并 %d；用户：新建 %d，合并 %d；Token：新建 %d，已存在 %d；相册：新建 %d，合并 %d\n",
		accounts.GroupsCreated, accounts.GroupsReused, accounts.UsersCreated, accounts.UsersReused,
		accounts.TokensCreated, accounts.TokensKept, accounts.AlbumsCreated, accounts.AlbumsReused)
	for _, note := range accounts.Notes {
		out.printf("  注意：%s\n", note)
	}
	if run.skipImages {
		return nil
	}
	maxBytes := int64(cfg.Server.MaxUploadMB) << 20
	var fetch lsky.Fetcher = lsky.HTTPFetcher{Client: &http.Client{Timeout: 2 * time.Minute}, BaseURL: run.sourceURL, MaxBytes: maxBytes}
	if run.sourceDir != "" {
		fetch = lsky.DirFetcher{Root: run.sourceDir, MaxBytes: maxBytes}
	}
	images, _, closeImages, err := newImageServices(ctx, db, cfg, nil, imageHooks{})
	if err != nil {
		return err
	}
	if run.adopt {
		open, err := images.AdoptionReader(ctx, run.policyID)
		if err != nil {
			return errors.Join(fmt.Errorf("--adopt needs a rule on a versioned S3 storage: %w", err), closeImages())
		}
		fetch = lsky.StorageFetcher{Open: open, MaxBytes: maxBytes}
	}
	ingest := func(ctx context.Context, item lsky.Item) error { return lskyIngest(images)(ctx, run.policyID, item) }
	report, err := lsky.ImportImages(ctx, db, snap, mapping, fetch, ingest, lsky.ImageOptions{
		StorageID: policy.StorageID, Limit: run.limit, Progress: func(line string) { out.printf("%s\n", line) },
	})
	err = errors.Join(err, closeImages())
	out.printf("图片：本次导入 %d，之前已导入 %d，跳过 %d，失败 %d\n", report.Imported, report.AlreadyThere, len(report.Skipped), len(report.Failed))
	for _, line := range report.Skipped {
		out.printf("  跳过 %s\n", line)
	}
	for _, line := range report.Failed {
		out.printf("  失败 %s\n", line)
	}
	if err == nil && len(report.Failed) > 0 {
		err = fmt.Errorf("%d images failed; fix the cause and rerun to retry only those", len(report.Failed))
	}
	return err
}

// lskyIngest stores one Lsky image through the regular import pipeline,
// adopting it in place when the item carries a bucket version.
func lskyIngest(images *service.ImageService) func(context.Context, uint64, lsky.Item) error {
	return func(ctx context.Context, policyID uint64, item lsky.Item) error {
		filename := item.Source.OriginName
		if filename == "" {
			filename = pathpkg.Base(item.Source.Pathname)
		}
		_, err := images.ImportImage(ctx, service.ImportInput{
			Data: item.Data, Filename: filename, IP: item.Source.IP, Path: item.Path, Ext: item.Ext,
			UserID: item.UserID, PolicyID: policyID, AlbumID: item.AlbumID, IsPublic: item.Source.IsPublic, CreatedAt: item.Source.CreatedAt,
			AdoptVersion: item.Version,
		})
		return lskyImageError(err)
	}
}

// lineWriter prints the import report and keeps the first write error.
type lineWriter struct {
	w   io.Writer
	err error
}

func (l *lineWriter) printf(format string, args ...any) {
	if l.err == nil {
		_, l.err = fmt.Fprintf(l.w, format, args...)
	}
}

// lskyImageError turns pipeline sentinels into the operator-facing reason.
func lskyImageError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, service.ErrImportLinkChanged):
		return errors.New("文件实际格式与扩展名不符，保存后旧链接会失效")
	case errors.Is(err, service.ErrPathConflict):
		return errors.New("目标存储里已有同路径的对象，或原图在读取后被改动")
	case errors.Is(err, service.ErrNotFound):
		return errors.New("存储桶里没有这个文件")
	case errors.Is(err, service.ErrUnsupportedFormat):
		return errors.New("格式不受支持或不在所属组允许的格式里")
	case errors.Is(err, service.ErrQuotaExceeded):
		return errors.New("所属组容量不足")
	case errors.Is(err, service.ErrProcessing):
		return errors.New("图片处理失败（损坏、超像素上限或无法无损脱敏）")
	case errors.Is(err, service.ErrStorage):
		return errors.New("写入目标存储失败")
	case errors.Is(err, service.ErrForbidden), errors.Is(err, service.ErrUnauthenticated):
		return errors.New("所属用户已停用或不能使用该规则")
	case errors.Is(err, service.ErrInvalidInput):
		return errors.New("路径或文件不合法（可能超过上传大小上限）")
	}
	return fmt.Errorf("导入失败：%w", err)
}
