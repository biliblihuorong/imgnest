// Package lsky reads a Lsky Pro (蓝空图床) database and maps its accounts
// into ImgNest's own schema; images are re-ingested by the caller through
// the regular upload pipeline so their records and objects are exactly what
// an ImgNest upload produces.
package lsky

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

// Group is one Lsky role group with the limits ImgNest can carry over.
type Group struct {
	ID             int64
	Name           string
	IsDefault      bool
	IsGuest        bool
	MaxFileKB      float64
	PerMinute      int
	AcceptedSuffix []string
	PathRule       string
	FileRule       string
	CreatedAt      time.Time
}

// User is one Lsky account. PasswordHash is a bcrypt digest; it is copied,
// never logged or printed.
type User struct {
	ID           int64
	GroupID      int64
	Name         string
	Email        string
	PasswordHash string
	IsAdmin      bool
	Enabled      bool
	RegisteredIP string
	CreatedAt    time.Time
}

// Token is one Lsky (Laravel Sanctum) personal access token. Hash is the
// SHA-256 hex digest of the secret after "id|", the same scheme ImgNest uses.
type Token struct {
	ID         int64
	UserID     int64
	Name       string
	Hash       string
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

// Album is one Lsky album.
type Album struct {
	ID        int64
	UserID    int64
	Name      string
	Intro     string
	CreatedAt time.Time
}

// Strategy is one Lsky storage strategy; only its public URL is read, never
// its credentials.
type Strategy struct {
	ID   int64
	Key  int
	Name string
	URL  string
}

// Image is one Lsky image row. Pathname is the object key under the
// strategy URL ("{path}/{name}"), the link blogs already embed.
type Image struct {
	ID         int64
	UserID     int64
	AlbumID    int64
	StrategyID int64
	Pathname   string
	Extension  string
	OriginName string
	MD5        string
	IsPublic   bool
	IP         string
	CreatedAt  time.Time
}

// Snapshot is everything the import reads from the source database.
type Snapshot struct {
	Groups     []Group
	Users      []User
	Tokens     []Token
	Albums     []Album
	Strategies []Strategy
	Images     []Image
}

// Load reads a Lsky Pro PostgreSQL database. Timestamps without a zone are
// interpreted in loc (Lsky stores its application time zone).
func Load(ctx context.Context, db *sql.DB, loc *time.Location) (Snapshot, error) {
	if db == nil || loc == nil {
		return Snapshot{}, errors.New("load lsky: database and time zone are required")
	}
	var snap Snapshot
	steps := []func(context.Context, *sql.DB, *time.Location, *Snapshot) error{loadGroups, loadUsers, loadTokens, loadAlbums, loadStrategies, loadImages}
	for _, step := range steps {
		if err := step(ctx, db, loc, &snap); err != nil {
			return Snapshot{}, fmt.Errorf("load lsky: %w", err)
		}
	}
	return snap, nil
}

func query(ctx context.Context, db *sql.DB, statement string, scan func(*sql.Rows) error) error {
	rows, err := db.QueryContext(ctx, statement)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// stamp converts a zone-less Lsky timestamp. The DBX exporter and old Lsky
// rows can carry year-1 placeholders; those come back zero.
func stamp(value sql.NullTime, loc *time.Location) time.Time {
	if !value.Valid || value.Time.Year() < 1971 {
		return time.Time{}
	}
	t := value.Time
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), loc).UTC()
}

func loadGroups(ctx context.Context, db *sql.DB, loc *time.Location, snap *Snapshot) error {
	return query(ctx, db, `SELECT id, name, is_default, is_guest, configs::text, created_at FROM groups ORDER BY id`, func(rows *sql.Rows) error {
		var g Group
		var configs string
		var created sql.NullTime
		if err := rows.Scan(&g.ID, &g.Name, &g.IsDefault, &g.IsGuest, &configs, &created); err != nil {
			return fmt.Errorf("groups: %w", err)
		}
		var parsed struct {
			MaxFileSize    json.Number `json:"maximum_file_size"`
			LimitPerMinute json.Number `json:"limit_per_minute"`
			AcceptedSuffix []string    `json:"accepted_file_suffixes"`
			PathNamingRule string      `json:"path_naming_rule"`
			FileNamingRule string      `json:"file_naming_rule"`
		}
		if err := json.Unmarshal([]byte(configs), &parsed); err != nil {
			return fmt.Errorf("group %d configs: %w", g.ID, err)
		}
		g.MaxFileKB, _ = parsed.MaxFileSize.Float64()
		perMinute, _ := parsed.LimitPerMinute.Int64()
		g.PerMinute = int(min(max(perMinute, 0), 1<<31-1))
		g.AcceptedSuffix, g.PathRule, g.FileRule = parsed.AcceptedSuffix, parsed.PathNamingRule, parsed.FileNamingRule
		g.CreatedAt = stamp(created, loc)
		snap.Groups = append(snap.Groups, g)
		return nil
	})
}

func loadUsers(ctx context.Context, db *sql.DB, loc *time.Location, snap *Snapshot) error {
	return query(ctx, db, `SELECT id, COALESCE(group_id, 0), name, email, password, is_adminer, status, registered_ip, created_at FROM users ORDER BY id`, func(rows *sql.Rows) error {
		var u User
		var status int
		var created sql.NullTime
		if err := rows.Scan(&u.ID, &u.GroupID, &u.Name, &u.Email, &u.PasswordHash, &u.IsAdmin, &status, &u.RegisteredIP, &created); err != nil {
			return fmt.Errorf("users: %w", err)
		}
		u.Enabled = status == 1
		u.CreatedAt = stamp(created, loc)
		snap.Users = append(snap.Users, u)
		return nil
	})
}

func loadTokens(ctx context.Context, db *sql.DB, loc *time.Location, snap *Snapshot) error {
	return query(ctx, db, `SELECT id, tokenable_id, name, token, last_used_at, created_at FROM personal_access_tokens WHERE tokenable_type LIKE '%User' ORDER BY id`, func(rows *sql.Rows) error {
		var t Token
		var used, created sql.NullTime
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Hash, &used, &created); err != nil {
			return fmt.Errorf("tokens: %w", err)
		}
		if at := stamp(used, loc); !at.IsZero() {
			t.LastUsedAt = &at
		}
		t.CreatedAt = stamp(created, loc)
		snap.Tokens = append(snap.Tokens, t)
		return nil
	})
}

func loadAlbums(ctx context.Context, db *sql.DB, loc *time.Location, snap *Snapshot) error {
	return query(ctx, db, `SELECT id, user_id, name, intro, created_at FROM albums ORDER BY id`, func(rows *sql.Rows) error {
		var a Album
		var created sql.NullTime
		if err := rows.Scan(&a.ID, &a.UserID, &a.Name, &a.Intro, &created); err != nil {
			return fmt.Errorf("albums: %w", err)
		}
		a.CreatedAt = stamp(created, loc)
		snap.Albums = append(snap.Albums, a)
		return nil
	})
}

func loadStrategies(ctx context.Context, db *sql.DB, _ *time.Location, snap *Snapshot) error {
	// Only the public URL leaves the database; access keys stay behind.
	return query(ctx, db, `SELECT id, key, name, COALESCE(configs::json->>'url', '') FROM strategies ORDER BY id`, func(rows *sql.Rows) error {
		var s Strategy
		if err := rows.Scan(&s.ID, &s.Key, &s.Name, &s.URL); err != nil {
			return fmt.Errorf("strategies: %w", err)
		}
		s.URL = strings.TrimRight(s.URL, "/")
		snap.Strategies = append(snap.Strategies, s)
		return nil
	})
}

func loadImages(ctx context.Context, db *sql.DB, loc *time.Location, snap *Snapshot) error {
	return query(ctx, db, `SELECT id, COALESCE(user_id, 0), COALESCE(album_id, 0), COALESCE(strategy_id, 0), path, name, extension, origin_name, md5, permission, uploaded_ip, created_at FROM images ORDER BY id`, func(rows *sql.Rows) error {
		var img Image
		var dir, name string
		var permission int
		var created sql.NullTime
		if err := rows.Scan(&img.ID, &img.UserID, &img.AlbumID, &img.StrategyID, &dir, &name, &img.Extension, &img.OriginName, &img.MD5, &permission, &img.IP, &created); err != nil {
			return fmt.Errorf("images: %w", err)
		}
		img.Pathname = strings.Trim(path.Join(dir, name), "/")
		img.Extension = strings.ToLower(img.Extension)
		img.IsPublic = permission == 1
		img.CreatedAt = stamp(created, loc)
		if img.CreatedAt.IsZero() {
			img.CreatedAt = uniqidTime(name)
		}
		snap.Images = append(snap.Images, img)
		return nil
	})
}
