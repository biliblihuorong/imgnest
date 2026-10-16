package cli

import (
	"crypto/md5" // #nosec G501 -- mirrors Lsky's content checksum column in the fixture.
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// lskySource creates an isolated schema holding the Lsky Pro tables the
// import reads and returns a DSN pinned to it.
func lskySource(t *testing.T, statements ...string) string {
	t.Helper()
	dsn := os.Getenv("IMGNEST_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("IMGNEST_TEST_POSTGRES_DSN is not set")
	}
	admin, err := repo.Open(t.Context(), config.Database{Driver: "postgres", DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	adminDB, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}
	schema := "lsky_" + hex.EncodeToString(raw[:])
	if _, err := adminDB.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := adminDB.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error(err)
		}
		_ = adminDB.Close()
	})
	ddl := []string{
		`CREATE TABLE groups (id bigserial PRIMARY KEY, name varchar(64) NOT NULL, is_default boolean NOT NULL DEFAULT false, is_guest boolean NOT NULL DEFAULT false, configs json NOT NULL, created_at timestamp(0), updated_at timestamp(0))`,
		`CREATE TABLE users (id bigserial PRIMARY KEY, group_id bigint, name varchar(255) NOT NULL, email varchar(255) NOT NULL, password varchar(255) NOT NULL, is_adminer boolean NOT NULL DEFAULT false, status smallint NOT NULL DEFAULT 1, registered_ip varchar(255) NOT NULL DEFAULT '', created_at timestamp(0))`,
		`CREATE TABLE personal_access_tokens (id bigserial PRIMARY KEY, tokenable_type varchar(255) NOT NULL, tokenable_id bigint NOT NULL, name varchar(255) NOT NULL, token varchar(64) NOT NULL, last_used_at timestamp(0), created_at timestamp(0))`,
		`CREATE TABLE albums (id bigserial PRIMARY KEY, user_id bigint NOT NULL, name varchar(64) NOT NULL, intro varchar(255) NOT NULL DEFAULT '', created_at timestamp(0))`,
		`CREATE TABLE strategies (id bigserial PRIMARY KEY, key smallint NOT NULL, name varchar(64) NOT NULL, configs json NOT NULL)`,
		`CREATE TABLE images (id bigserial PRIMARY KEY, user_id bigint, album_id bigint, group_id bigint, strategy_id bigint, key varchar(255) NOT NULL, path varchar(255) NOT NULL, name varchar(255) NOT NULL, origin_name varchar(255) NOT NULL DEFAULT '', size numeric(8,2) NOT NULL DEFAULT 0, mimetype varchar(32) NOT NULL, extension varchar(32) NOT NULL, md5 varchar(32) NOT NULL, permission smallint NOT NULL DEFAULT 0, uploaded_ip varchar(255) NOT NULL DEFAULT '', created_at timestamp(0))`,
	}
	for _, statement := range append(ddl, statements...) {
		if _, err := adminDB.ExecContext(t.Context(), "SET search_path TO "+schema+"; "+statement); err != nil {
			t.Fatalf("lsky fixture: %v", err)
		}
	}
	return dsn + " search_path=" + schema
}

func TestImportLskyCLI(t *testing.T) {
	ctx := t.Context()
	sourceDir := t.TempDir()
	put := func(pathname string, data []byte) string {
		t.Helper()
		full := filepath.Join(sourceDir, filepath.FromSlash(pathname))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o600); err != nil {
			t.Fatal(err)
		}
		digest := md5.Sum(data) // #nosec G401 -- fixture checksum.
		return hex.EncodeToString(digest[:])
	}
	wide := encodedJPEG(t, 1700, 850, 1)
	wideMD5 := put("1/2024/04/12/661947a7e1b2c.jpg", wide)
	fakeMD5 := put("1/2024/04/13/661947a7e1b2d.png", encodedJPEG(t, 64, 64, 1))
	oldMD5 := put("1/23/01/27/63d342d7133e6.jpg", encodedJPEG(t, 300, 200, 1))
	icoMD5 := put("1/2024/04/14/icon.ico", []byte("not-an-image"))
	hash, err := bcrypt.GenerateFromPassword([]byte("lsky-password-123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	lskyHash := "$2y$" + string(hash[4:])
	secret := strings.Repeat("Ab3", 13) + "Z"
	tokenDigest := sha256.Sum256([]byte(secret))
	dsn := lskySource(t,
		`INSERT INTO groups (id, name, is_default, configs) VALUES (3, 'root博客组', false, '{"maximum_file_size":51390,"limit_per_minute":20000,"accepted_file_suffixes":["jpeg","jpg","png","ico"]}')`,
		`INSERT INTO users (id, group_id, name, email, password, is_adminer, registered_ip, created_at) VALUES (1, 3, '某科学的贝壳', 'shell@example.com', '`+lskyHash+`', true, '192.0.2.1', '2024-04-12 22:44:55')`,
		`INSERT INTO personal_access_tokens (id, tokenable_type, tokenable_id, name, token, created_at) VALUES (7, 'App\Models\User', 1, 'picgo', '`+hex.EncodeToString(tokenDigest[:])+`', '2024-04-12 22:44:55')`,
		`INSERT INTO albums (id, user_id, name) VALUES (4, 1, '博客')`,
		`INSERT INTO strategies (id, key, name, configs) VALUES (9, 10, 'backblaze', '{"url":"https://img.example.invalid/old-bucket","access_key":"never-read","secret_key":"never-read"}')`,
		`INSERT INTO images (id, user_id, album_id, strategy_id, key, path, name, origin_name, mimetype, extension, md5, permission, uploaded_ip, created_at) VALUES
			(1, 1, 4, 9, 'AAAAAA', '1/2024/04/12', '661947a7e1b2c.jpg', 'wide.jpg', 'image/jpeg', 'jpg', '`+wideMD5+`', 1, '192.0.2.9', '2024-04-12 22:44:55'),
			(2, 1, NULL, 9, 'BBBBBB', '1/2024/04/13', '661947a7e1b2d.png', 'fake.png', 'image/png', 'png', '`+fakeMD5+`', 0, '', '2024-04-13 10:00:00'),
			(3, 1, NULL, 9, 'CCCCCC', '1/23/01/27', '63d342d7133e6.jpg', 'old.jpg', 'image/jpeg', 'jpg', '`+oldMD5+`', 0, '', '0001-01-01 11:19:51'),
			(4, 1, NULL, 9, 'DDDDDD', '1/2024/04/14', 'icon.ico', 'icon.ico', 'image/x-icon', 'ico', '`+icoMD5+`', 0, '', '2024-04-14 10:00:00')`,
	)
	t.Setenv(lskySourceEnv, dsn)
	path := commandConfig(t)
	if _, err := command(ctx, t, path, "", "migrate"); err != nil {
		t.Fatal(err)
	}
	if _, err := command(ctx, t, path, "", "init-local", "--root", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	output, err := command(ctx, t, path, `{"responsive_variants":true}`, "init-policy", "--storage-id", "1", "--name", "blog", "--stdin", "--default=false")
	if err != nil {
		t.Fatal(err)
	}
	var policy struct{ ID uint64 }
	if json.Unmarshal([]byte(output), &policy) != nil || policy.ID == 0 {
		t.Fatal("policy setup output invalid")
	}
	output, err = command(ctx, t, path, "", "import-lsky", "--policy-id", fmt.Sprint(policy.ID), "--source-dir", sourceDir)
	if err == nil || !strings.Contains(output, "本次导入 2") || !strings.Contains(output, "旧链接会失效") || !strings.Contains(output, "不支持 .ico") {
		t.Fatalf("first run: err=%v\n%s", err, output)
	}
	if strings.Contains(output, "never-read") || strings.Contains(output, lskyHash) {
		t.Fatal("import output leaked a credential")
	}
	err = withDatabase(ctx, path, func(db *gorm.DB, _ config.Config) error {
		var images []model.Image
		if err := db.Order("path").Find(&images, "state = ?", model.ImageStateActive).Error; err != nil {
			return err
		}
		if len(images) != 2 {
			t.Fatalf("active images = %d", len(images))
		}
		blog, old := images[0], images[1]
		if blog.Path != "1/2024/04/12/661947a7e1b2c" || blog.Ext != "jpg" || !blog.HasOriginal || !blog.HasWebP || !blog.HasThumb || len(blog.Variants) != 3 || !blog.IsPublic || blog.AlbumID == 0 || blog.OriginName != "wide.jpg" || blog.IP != "192.0.2.9" {
			t.Fatalf("blog image = %+v", blog)
		}
		if !blog.CreatedAt.Equal(time.Date(2024, 4, 12, 14, 44, 55, 0, time.UTC)) {
			t.Fatalf("created_at = %v, want the Shanghai timestamp in UTC", blog.CreatedAt)
		}
		if old.Path != "1/23/01/27/63d342d7133e6" || !old.CreatedAt.Equal(time.Unix(0x63d342d7, 0)) {
			t.Fatalf("year-1 row must fall back to its uniqid time: %+v", old)
		}
		var exif int64
		if err := db.Table("image_exif").Count(&exif).Error; err != nil || exif != 2 {
			t.Fatalf("exif rows = %d, %v", exif, err)
		}
		var owner model.User
		if err := db.First(&owner, "email = ?", "shell@example.com").Error; err != nil {
			return err
		}
		if owner.UsedBytes != blog.ChargedBytes+old.ChargedBytes || owner.Role != model.UserRoleAdmin {
			t.Fatalf("owner = %+v", owner)
		}
		users, tokens, err := newServices(ctx, db)
		if err != nil {
			return err
		}
		if _, err := users.VerifyCredentials(ctx, "shell@example.com", "lsky-password-123"); err != nil {
			t.Fatalf("the Lsky bcrypt password must keep working: %v", err)
		}
		identity, err := tokens.Authenticate(ctx, "7|"+secret)
		if err != nil || identity.User.ID != owner.ID {
			t.Fatalf("the Lsky token must keep working: %+v %v", identity, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err = command(ctx, t, path, "", "import-lsky", "--policy-id", fmt.Sprint(policy.ID), "--source-dir", sourceDir)
	if err == nil || !strings.Contains(output, "本次导入 0，之前已导入 2") {
		t.Fatalf("rerun must only retry the failed image: err=%v\n%s", err, output)
	}
}

func TestLskyImageErrorNamesProcessingStep(t *testing.T) {
	err := lskyImageError(fmt.Errorf("publish: %w", &service.ProcessingError{Step: service.ProcessingMetadata}))
	if err == nil || !strings.Contains(err.Error(), "元数据") {
		t.Fatalf("err = %v", err)
	}
	if !errors.Is(&service.ProcessingError{Step: service.ProcessingPixels}, service.ErrProcessing) {
		t.Fatal("ProcessingError does not match ErrProcessing")
	}
	if err := lskyImageError(service.ErrProcessing); err == nil || !strings.Contains(err.Error(), "图片处理失败") {
		t.Fatalf("err = %v", err)
	}
}
