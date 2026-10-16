package lsky

import (
	"context"
	"crypto/md5" // #nosec G501 -- compares Lsky's stored content checksum, not a credential.
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/storage"
	"gorm.io/gorm"
)

// Fetcher reads the stored bytes of one Lsky image.
type Fetcher interface {
	Fetch(ctx context.Context, image Image, strategy Strategy) (Fetched, error)
}

// Fetched is one original read from the old storage. Version is set only by
// StorageFetcher: the bucket version the image adopts in place.
type Fetched struct {
	Data    []byte
	Version string
}

// HTTPFetcher downloads originals from their public links: BaseURL when set,
// else the strategy's own URL, followed by the escaped pathname.
type HTTPFetcher struct {
	Client   *http.Client
	BaseURL  string
	MaxBytes int64
}

// Fetch implements Fetcher.
func (f HTTPFetcher) Fetch(ctx context.Context, image Image, strategy Strategy) (Fetched, error) {
	data, err := f.fetch(ctx, image, strategy)
	return Fetched{Data: data}, err
}

func (f HTTPFetcher) fetch(ctx context.Context, image Image, strategy Strategy) ([]byte, error) {
	base := strings.TrimRight(f.BaseURL, "/")
	if base == "" {
		base = strategy.URL
	}
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return nil, fmt.Errorf("strategy %d has no usable public URL", strategy.ID)
	}
	segments := strings.Split(image.Pathname, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/"+strings.Join(segments, "/"), nil)
	if err != nil {
		return nil, err
	}
	client := f.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: HTTP %d", response.StatusCode)
	}
	return readCapped(response.Body, f.MaxBytes)
}

// DirFetcher reads originals from a local copy of the storage root (for a
// Lsky local strategy, or a bucket synced to disk).
type DirFetcher struct {
	Root     string
	MaxBytes int64
}

// Fetch implements Fetcher.
func (f DirFetcher) Fetch(ctx context.Context, image Image, _ Strategy) (Fetched, error) {
	data, err := f.fetch(ctx, image)
	return Fetched{Data: data}, err
}

func (f DirFetcher) fetch(ctx context.Context, image Image) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(f.Root)
	if err != nil {
		return nil, fmt.Errorf("open source directory: %w", err)
	}
	defer func() { _ = root.Close() }()
	file, err := root.Open(image.Pathname)
	if err != nil {
		return nil, fmt.Errorf("open source file: %w", err)
	}
	defer func() { _ = file.Close() }()
	return readCapped(file, f.MaxBytes)
}

// StorageFetcher reads originals straight from the ImgNest storage that
// already holds them (the Lsky bucket being adopted in place). Each object
// must be one ImgNest never wrote, in a bucket that versions it, so the
// image can pin the exact version it takes over.
type StorageFetcher struct {
	Open     func(context.Context, string) (io.ReadCloser, storage.ObjectInfo, error)
	MaxBytes int64
}

// Fetch implements Fetcher.
func (f StorageFetcher) Fetch(ctx context.Context, image Image, _ Strategy) (Fetched, error) {
	if f.Open == nil {
		return Fetched{}, errors.New("no storage to read from")
	}
	body, info, err := f.Open(ctx, image.Pathname)
	if errors.Is(err, storage.ErrNotFound) {
		return Fetched{}, errors.New("存储桶里没有这个文件")
	}
	if err != nil {
		return Fetched{}, errors.New("读取存储桶失败")
	}
	defer func() { _ = body.Close() }()
	if info.OwnerID != "" {
		return Fetched{}, errors.New("这个文件已经由 ImgNest 管理")
	}
	if !model.AdoptableVersion(info.VersionID) {
		return Fetched{}, errors.New("存储桶没有开启版本控制，无法原地接管")
	}
	data, err := readCapped(body, f.MaxBytes)
	if err != nil {
		return Fetched{}, err
	}
	return Fetched{Data: data, Version: info.VersionID}, nil
}

func readCapped(body io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = 100 << 20
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("文件超过 ImgNest 单文件上限 %d MiB", limit>>20)
	}
	return data, nil
}

// Item is one image handed to the ingest function, with its owner and
// album already mapped and, unless adopted in place, its bytes verified
// against Lsky's MD5.
type Item struct {
	Source          Image
	UserID, AlbumID uint64
	Path, Ext       string
	Data            []byte
	// Version is the bucket version to adopt in place; empty means the
	// original is written anew.
	Version string
}

// Ingest stores one item in ImgNest (the CLI runs service.ImportImage).
type Ingest func(context.Context, Item) error

// ImageOptions configures the image pass.
type ImageOptions struct {
	// StorageID is the target rule's storage, used to skip images already
	// imported at the same path.
	StorageID uint64
	// Limit stops after this many images were attempted (0 = all).
	Limit int
	// StrategyID, when set, imports only the images of that Lsky strategy.
	StrategyID int64
	// Progress, when set, receives one line per image.
	Progress func(string)
	// Report, when set, receives the running totals after each image.
	Report func(ImageReport)
}

// ImageReport counts the image pass outcome; Failed and Skipped carry one
// human-readable line per image.
type ImageReport struct {
	Imported, AlreadyThere int
	Skipped, Failed        []string
}

// ImportImages fetches every Lsky image that is not yet in ImgNest and
// passes it to ingest. Failures are recorded and the pass continues, so a
// rerun only retries what is still missing.
func ImportImages(ctx context.Context, db *gorm.DB, snap Snapshot, mapping Mapping, fetch Fetcher, ingest Ingest, opts ImageOptions) (ImageReport, error) {
	var report ImageReport
	if db == nil || fetch == nil || ingest == nil || opts.StorageID == 0 {
		return report, errors.New("import lsky images: database, fetcher, ingest and storage are required")
	}
	progress := opts.Progress
	if progress == nil {
		progress = func(string) {}
	}
	strategies := map[int64]Strategy{}
	for _, strategy := range snap.Strategies {
		strategies[strategy.ID] = strategy
	}
	disabled := map[int64]bool{}
	for _, user := range snap.Users {
		disabled[user.ID] = !user.Enabled
	}
	attempted := 0
	for _, image := range snap.Images {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if opts.StrategyID != 0 && image.StrategyID != opts.StrategyID {
			continue
		}
		label := fmt.Sprintf("#%d %s", image.ID, image.Pathname)
		dir, ext := splitPathname(image.Pathname)
		switch {
		case dir == "" || ext == "":
			report.Skipped = append(report.Skipped, label+"：路径没有扩展名")
			continue
		case ext != strings.ToLower(ext):
			report.Skipped = append(report.Skipped, label+"：大写扩展名会被规范成小写，旧链接会失效")
			continue
		case !supportedExts[model.CanonicalExt(ext)]:
			report.Skipped = append(report.Skipped, label+"：ImgNest 不支持 ."+ext+"，文件仍只在旧存储里")
			continue
		case model.CanonicalExt(ext) != ext:
			report.Skipped = append(report.Skipped, label+"：扩展名 ."+ext+" 会被规范成 ."+model.CanonicalExt(ext)+"，旧链接会失效")
			continue
		}
		owner, ok := mapping.Users[image.UserID]
		if !ok || image.UserID == 0 {
			report.Skipped = append(report.Skipped, label+"：游客或已删除用户的图片")
			continue
		}
		if disabled[image.UserID] {
			report.Skipped = append(report.Skipped, label+"：所属用户已停用")
			continue
		}
		var count int64
		if err := db.WithContext(ctx).Model(&model.Image{}).Where("storage_id = ? AND path = ?", opts.StorageID, dir).Count(&count).Error; err != nil {
			return report, fmt.Errorf("import lsky images: %w", err)
		}
		if count > 0 {
			report.AlreadyThere++
			continue
		}
		if opts.Limit > 0 && attempted >= opts.Limit {
			break
		}
		attempted++
		fetched, err := fetch.Fetch(ctx, image, strategies[image.StrategyID])
		// An adopted original is the pinned bucket version itself, so it is
		// taken as it is: Lsky recorded the MD5 of the upload, which differs
		// when it converted the file before storing it.
		if err == nil && image.MD5 != "" && fetched.Version == "" {
			digest := md5.Sum(fetched.Data) // #nosec G401 -- integrity check against Lsky's own column.
			if !strings.EqualFold(hex.EncodeToString(digest[:]), image.MD5) {
				err = errors.New("文件内容与蓝空记录的 MD5 不一致")
			}
		}
		if err == nil {
			err = ingest(ctx, Item{Source: image, UserID: owner, AlbumID: mapping.Albums[image.AlbumID], Path: dir, Ext: ext, Data: fetched.Data, Version: fetched.Version})
		}
		if err != nil {
			if ctx.Err() != nil {
				return report, ctx.Err()
			}
			report.Failed = append(report.Failed, label+"："+err.Error())
			progress("FAIL " + label + ": " + err.Error())
		} else {
			report.Imported++
			progress("OK   " + label)
		}
		if opts.Report != nil {
			opts.Report(report)
		}
	}
	return report, nil
}

// splitPathname turns "1/2025/07/25/abc.webp" into ("1/2025/07/25/abc", "webp").
func splitPathname(pathname string) (string, string) {
	ext := path.Ext(pathname)
	if ext == "" || ext == "." {
		return "", ""
	}
	return strings.TrimSuffix(pathname, ext), ext[1:]
}
