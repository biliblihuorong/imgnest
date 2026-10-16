package lsky

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/searchquery"
	"gorm.io/gorm"
)

// Options configures the account import.
type Options struct {
	// PolicyID is the ImgNest upload rule every imported group is bound to
	// and whose storage receives the re-ingested images.
	PolicyID uint64
	// Adopt takes the originals over in place, so a WebP-only rule cannot
	// break their links.
	Adopt bool
	Now   func() time.Time
}

// Mapping links Lsky IDs to the ImgNest rows that now represent them.
type Mapping struct {
	Groups map[int64]uint64
	Users  map[int64]uint64
	Albums map[int64]uint64
}

// Report summarises an account import. Notes never contain password
// hashes, token digests or storage credentials.
type Report struct {
	GroupsCreated, GroupsReused int
	UsersCreated, UsersReused   int
	TokensCreated, TokensKept   int
	AlbumsCreated, AlbumsReused int
	Notes                       []string
}

func (r *Report) note(format string, args ...any) {
	r.Notes = append(r.Notes, fmt.Sprintf(format, args...))
}

// unusablePassword marks an imported account whose Lsky hash is not bcrypt;
// it can never verify, so an administrator has to reset the password.
const unusablePassword = "!lsky-unusable-password" // #nosec G101 -- a marker no bcrypt check can match, not a credential.

// supportedExts are the source formats ImgNest accepts (repo.checkUploadGroup).
var supportedExts = map[string]bool{"jpg": true, "png": true, "gif": true, "webp": true, "bmp": true, "tiff": true, "heic": true, "avif": true}

// ImportAccounts maps Lsky groups, users, tokens and albums into ImgNest in
// one transaction. It is idempotent: groups match by name (an existing group
// keeps its limits and only gains the import rule), users by email,
// tokens by digest and albums by owner and name, so a rerun reuses what an
// earlier run created and never overwrites existing ImgNest accounts.
func ImportAccounts(ctx context.Context, db *gorm.DB, snap Snapshot, opts Options) (Mapping, Report, error) {
	mapping := Mapping{Groups: map[int64]uint64{}, Users: map[int64]uint64{}, Albums: map[int64]uint64{}}
	var report Report
	if db == nil || opts.PolicyID == 0 || opts.PolicyID > math.MaxInt64 {
		return mapping, report, errors.New("import lsky accounts: database and policy are required")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var policy model.Policy
		if err := tx.First(&policy, "id = ?", opts.PolicyID).Error; err != nil {
			return fmt.Errorf("find policy %d: %w", opts.PolicyID, err)
		}
		if policy.WebPMode == "webp_only" && !opts.Adopt {
			return errors.New("policy stores only WebP, which would break every non-WebP historic link; pick a rule with webp_mode both or none")
		}
		defaultGroup, err := defaultGroupID(tx)
		if err != nil {
			return err
		}
		now := opts.Now().UTC()
		for _, group := range snap.Groups {
			id, created, err := importGroup(tx, group, opts.PolicyID, now)
			if err != nil {
				return fmt.Errorf("group %d: %w", group.ID, err)
			}
			mapping.Groups[group.ID] = id
			if created {
				report.GroupsCreated++
			} else {
				report.GroupsReused++
			}
		}
		for _, user := range snap.Users {
			groupID, ok := mapping.Groups[user.GroupID]
			if !ok {
				groupID = defaultGroup
			}
			id, created, err := importUser(tx, user, groupID, opts.PolicyID, now, &report)
			if err != nil {
				return fmt.Errorf("user %d: %w", user.ID, err)
			}
			mapping.Users[user.ID] = id
			if created {
				report.UsersCreated++
			} else {
				report.UsersReused++
			}
		}
		for _, token := range snap.Tokens {
			owner, ok := mapping.Users[token.UserID]
			if !ok {
				report.note("Token %d 的用户 %d 不存在，跳过", token.ID, token.UserID)
				continue
			}
			created, err := importToken(tx, token, owner, now, &report)
			if err != nil {
				return fmt.Errorf("token %d: %w", token.ID, err)
			}
			if created {
				report.TokensCreated++
			} else {
				report.TokensKept++
			}
		}
		for _, album := range snap.Albums {
			owner, ok := mapping.Users[album.UserID]
			if !ok {
				report.note("相册 %d 的用户 %d 不存在，跳过", album.ID, album.UserID)
				continue
			}
			id, created, err := importAlbum(tx, album, owner, now)
			if err != nil {
				return fmt.Errorf("album %d: %w", album.ID, err)
			}
			mapping.Albums[album.ID] = id
			if created {
				report.AlbumsCreated++
			} else {
				report.AlbumsReused++
			}
		}
		return resetSequences(tx)
	})
	if err != nil {
		return Mapping{Groups: map[int64]uint64{}, Users: map[int64]uint64{}, Albums: map[int64]uint64{}}, Report{}, fmt.Errorf("import lsky accounts: %w", err)
	}
	return mapping, report, nil
}

func defaultGroupID(tx *gorm.DB) (uint64, error) {
	var setting model.Setting
	if err := tx.First(&setting, "key = ?", "default_group_id").Error; err != nil {
		return 0, fmt.Errorf("read default group: %w", err)
	}
	var id uint64
	if err := json.Unmarshal(setting.Value, &id); err != nil || id == 0 {
		return 0, errors.New("read default group: invalid setting")
	}
	return id, nil
}

func importGroup(tx *gorm.DB, source Group, policyID uint64, now time.Time) (uint64, bool, error) {
	name := strings.TrimSpace(source.Name)
	if name == "" {
		name = "lsky-group-" + strconv.FormatInt(source.ID, 10)
	}
	var existing model.Group
	err := tx.Where("name = ?", name).Order("id").First(&existing).Error
	if err == nil {
		return existing.ID, false, bindPolicy(tx, existing.ID, policyID, now)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, err
	}
	allowed := []string{}
	seen := map[string]bool{}
	for _, suffix := range source.AcceptedSuffix {
		ext := model.CanonicalExt(strings.ToLower(strings.TrimSpace(suffix)))
		if supportedExts[ext] && !seen[ext] {
			seen[ext] = true
			allowed = append(allowed, ext)
		}
	}
	maxBytes := int64(0)
	if source.MaxFileKB > 0 && source.MaxFileKB < math.MaxInt64/1024 {
		maxBytes = int64(math.Round(source.MaxFileKB * 1024))
	}
	created := source.CreatedAt
	if created.IsZero() {
		created = now
	}
	group := model.Group{Name: name, MaxFileBytes: maxBytes, AllowedExts: allowed, UploadPerMin: source.PerMinute, DefaultPolicyID: policyID, CreatedAt: created, UpdatedAt: now}
	if err := tx.Create(&group).Error; err != nil {
		return 0, false, err
	}
	return group.ID, true, bindPolicy(tx, group.ID, policyID, now)
}

func bindPolicy(tx *gorm.DB, groupID, policyID uint64, now time.Time) error {
	var count int64
	if err := tx.Table("group_policies").Where("group_id = ? AND policy_id = ?", groupID, policyID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return tx.Table("group_policies").Create(map[string]any{"group_id": groupID, "policy_id": policyID, "created_at": now, "updated_at": now}).Error
}

func importUser(tx *gorm.DB, source User, groupID, policyID uint64, now time.Time, report *Report) (uint64, bool, error) {
	email := strings.ToLower(strings.TrimSpace(source.Email))
	if email == "" {
		return 0, false, errors.New("empty email")
	}
	var existing model.User
	err := tx.Where("email = ?", email).First(&existing).Error
	if err == nil {
		report.note("蓝空用户 %d「%s」沿用已有的同邮箱账号 %d（不改其密码、角色与组）", source.ID, source.Name, existing.ID)
		// The account keeps its own group, which must still be allowed to
		// use the import rule or none of its images could be re-ingested.
		return existing.ID, false, bindPolicy(tx, existing.GroupID, policyID, now)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, err
	}
	username, err := freeUsername(tx, source, email)
	if err != nil {
		return 0, false, err
	}
	display := ""
	if username != strings.TrimSpace(source.Name) {
		display = strings.TrimSpace(source.Name)
		if utf8.RuneCountInString(display) > 64 {
			display = string([]rune(display)[:64])
		}
	}
	hash := source.PasswordHash
	if !isBcrypt(hash) {
		hash = unusablePassword
		report.note("蓝空用户 %d「%s」的密码不是 bcrypt 哈希，已导入为不可登录，请用 reset-password 重设", source.ID, source.Name)
	}
	role, status := model.UserRoleUser, model.UserStatusEnabled
	if source.IsAdmin {
		role = model.UserRoleAdmin
	}
	if !source.Enabled {
		status = model.UserStatusDisabled
		report.note("蓝空用户 %d「%s」已停用，其图片不会导入", source.ID, source.Name)
	}
	created := source.CreatedAt
	if created.IsZero() {
		created = now
	}
	user := model.User{Username: username, DisplayName: display, Email: email, PasswordHash: hash, Role: role, Status: status, GroupID: groupID, RegisteredIP: source.RegisteredIP, CreatedAt: created, UpdatedAt: now}
	if err := tx.Create(&user).Error; err != nil {
		return 0, false, err
	}
	return user.ID, true, nil
}

// freeUsername keeps the Lsky name when it is a valid, unused ImgNest
// username (3–64 runes), else falls back to the email's local part, then to
// a numbered name.
func freeUsername(tx *gorm.DB, source User, email string) (string, error) {
	local, _, _ := strings.Cut(email, "@")
	candidates := []string{strings.TrimSpace(source.Name), local, "lsky-" + local, "lsky-user-" + strconv.FormatInt(source.ID, 10)}
	for _, name := range candidates {
		runes := utf8.RuneCountInString(name)
		if !utf8.ValidString(name) || runes < 3 || runes > 64 {
			continue
		}
		var count int64
		if err := tx.Model(&model.User{}).Where("username = ?", name).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return name, nil
		}
	}
	return "", fmt.Errorf("no free username for lsky user %d", source.ID)
}

func isBcrypt(hash string) bool {
	if len(hash) != 60 {
		return false
	}
	return strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$")
}

func importToken(tx *gorm.DB, source Token, owner uint64, now time.Time, report *Report) (bool, error) {
	digest := strings.ToLower(strings.TrimSpace(source.Hash))
	if len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
		report.note("Token %d 的摘要格式不对，跳过", source.ID)
		return false, nil
	}
	var count int64
	if err := tx.Model(&model.Token{}).Where("token_hash = ?", digest).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	created := source.CreatedAt
	if created.IsZero() {
		created = now
	}
	token := model.Token{UserID: owner, Name: tokenName(source), TokenHash: digest, Kind: model.TokenKindAPI, Abilities: []string{"*"}, LastUsedAt: source.LastUsedAt, CreatedAt: created, UpdatedAt: now}
	// Sanctum tokens read "id|secret": keeping the id keeps PicGo and other
	// clients working without a new token.
	var wanted uint64
	if source.ID > 0 {
		wanted = uint64(source.ID) // #nosec G115 -- checked positive above.
		var taken int64
		if err := tx.Model(&model.Token{}).Where("id = ?", wanted).Count(&taken).Error; err != nil {
			return false, err
		}
		if taken == 0 {
			token.ID = wanted
		}
	}
	if err := tx.Create(&token).Error; err != nil {
		return false, err
	}
	if token.ID != wanted {
		report.note("Token %d 的新编号是 %d：把客户端里 Token 开头的「%d|」改成「%d|」即可继续使用", source.ID, token.ID, source.ID, token.ID)
	}
	return true, nil
}

func tokenName(source Token) string {
	name := strings.TrimSpace(source.Name)
	if name == "" {
		name = "lsky"
	}
	return "蓝空导入 " + name
}

func importAlbum(tx *gorm.DB, source Album, owner uint64, now time.Time) (uint64, bool, error) {
	name := strings.TrimSpace(source.Name)
	if name == "" {
		name = "lsky-album-" + strconv.FormatInt(source.ID, 10)
	}
	var existing model.Album
	err := tx.Select("id").Where("user_id = ? AND name = ?", owner, name).First(&existing).Error
	if err == nil {
		return existing.ID, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, err
	}
	created := source.CreatedAt
	if created.IsZero() {
		created = now
	}
	nfc, folded := searchquery.NFC(name), searchquery.Normalize(name)
	album := model.Album{UserID: owner, Name: name, Intro: source.Intro, NameNFC: &nfc, NameSearch: &folded, CreatedAt: created, UpdatedAt: now}
	if err := tx.Create(&album).Error; err != nil {
		return 0, false, err
	}
	return album.ID, true, nil
}

// resetSequences moves PostgreSQL identity counters past explicitly kept
// token IDs; SQLite derives the next rowid from the table itself.
func resetSequences(tx *gorm.DB) error {
	if tx.Name() != "postgres" {
		return nil
	}
	return tx.Exec(`SELECT setval(pg_get_serial_sequence('tokens', 'id'), GREATEST((SELECT COALESCE(MAX(id), 0) FROM tokens), 1))`).Error
}

// uniqidTime recovers the upload second from a PHP uniqid() file name
// (8 hex digits of Unix seconds, 5 of microseconds), used when the source
// row has no usable timestamp.
func uniqidTime(name string) time.Time {
	if len(name) < 13 {
		return time.Time{}
	}
	seconds, err := strconv.ParseInt(name[:8], 16, 64)
	if err != nil || strings.Trim(name[8:13], "0123456789abcdef") != "" {
		return time.Time{}
	}
	at := time.Unix(seconds, 0).UTC()
	if at.Year() < 2015 || at.Year() > 2100 {
		return time.Time{}
	}
	return at
}
