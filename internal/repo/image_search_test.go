package repo

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/searchquery"
	"gorm.io/gorm"
)

func TestUnifiedSearchWholeDatasetAndScope(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "search-owner")
		foreign := newImageFixture(t, db, "search-foreign")
		for i := 0; i < 45; i++ {
			input := f.request(fmt.Sprintf("query-%d", i), fmt.Sprintf("query/%d", i))
			input.Image.OriginName = "ordinary.png"
			if i == 0 {
				input.Image.OriginName = "Cafe\u0301 100%_match.JPG"
			}
			im, err := f.images.ReserveUpload(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			if err = db.Model(&model.Image{}).Where("id = ?", im.ID).Updates(map[string]any{"state": model.ImageStateActive, "created_at": time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}).Error; err != nil {
				t.Fatal(err)
			}
		}
		_ = reserveAndCommit(t, foreign, "foreign", "foreign")
		got, total, err := f.images.List(t.Context(), model.ImageListFilter{UserID: f.user.ID, Search: &model.ImageSearchFilter{Terms: []string{"café", "100%_"}, Sort: "newest"}}, 1, 20)
		if err != nil || total != 1 || len(got) != 1 || got[0].OriginName != "Cafe\u0301 100%_match.JPG" {
			t.Fatalf("full-dataset literal search: total=%d images=%+v err=%v", total, got, err)
		}
		got, total, err = f.images.List(t.Context(), model.ImageListFilter{UserID: f.user.ID, Search: &model.ImageSearchFilter{Sort: "newest"}}, 2, 20)
		if err != nil || total != 45 || len(got) != 20 {
			t.Fatalf("stable page: total=%d len=%d err=%v", total, len(got), err)
		}
		for _, im := range got {
			if im.UserID != f.user.ID {
				t.Fatal("foreign row leaked")
			}
		}
	})
}

func TestUnifiedSearchFixedDataset(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "fixed-owner")
		foreign := newImageFixture(t, db, "fixed-foreign")
		albums, _ := NewAlbumRepository(t.Context(), db)
		for _, v := range []struct {
			id   uint64
			name string
		}{{42, "暑假照片"}, {57, "工作"}, {58, "A,B"}, {70, "同名"}, {71, "同名"}} {
			if _, err := albums.Create(t.Context(), model.Album{ID: v.id, UserID: f.user.ID, Name: v.name}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := albums.Create(t.Context(), model.Album{ID: 99, UserID: foreign.user.ID, Name: "Private Album"}); err != nil {
			t.Fatal(err)
		}
		rows := []struct {
			id         uint64
			name, mime string
			size       int64
			album      uint64
			date       string
			public     bool
		}{{101, "暑假海边.JPG", "image/jpeg", 1000000, 42, "2026-10-01T00:00:00Z", false}, {102, "暑假海边.png", "image/png", 2000000, 42, "2026-10-02T12:00:00Z", true}, {103, "暑假山里.jpg", "image/webp", 500000, 57, "2026-09-30T23:59:59Z", false}, {104, "100%_完成.png", "image/png", 1000001, 58, "2026-10-01T00:00:00Z", false}, {105, "家庭 合照.jpeg", "image/jpeg", 3000000, 0, "2026-10-03T00:00:00Z", true}, {106, "暑假海边.png", "image/png", 1000000, 99, "2026-10-04T00:00:00Z", false}, {107, "unknown.jpg", "application/octet-stream", 1, 0, "2026-10-01T00:00:00Z", false}}
		for _, v := range rows {
			at, _ := time.Parse(time.RFC3339, v.date)
			filename := searchquery.Normalize(v.name)
			owner := f
			if v.id == 106 {
				owner = foreign
			}
			image := model.Image{ID: v.id, UserID: owner.user.ID, AlbumID: v.album, PolicyID: owner.policy.ID, StorageID: owner.storage.ID, Key: fmt.Sprint(v.id), Path: fmt.Sprint(v.id), Width: 1, Height: 1, Frames: 1, ObjectManifest: []model.ObjectReceipt{}, OriginName: v.name, FilenameSearch: &filename, MIME: v.mime, Ext: "jpg", State: model.ImageStateActive, Size: v.size, CreatedAt: at, IsPublic: v.public}
			if err := db.Create(&image).Error; err != nil {
				t.Fatal(err)
			}
		}
		camera, lens := "canon eos r6", "rf24-105"
		if err := db.Create(&model.ImageExif{ImageID: 101, Make: "Canon", Model: "EOS R6", Lens: "RF24-105", Raw: []byte(`{}`), CameraSearch: &camera, LensSearch: &lens}).Error; err != nil {
			t.Fatal(err)
		}
		tests := []struct {
			raw string
			ids []uint64
		}{{"", []uint64{105, 102, 107, 104, 101, 103}}, {"暑假 海边", []uint64{102, 101}}, {`"家庭 合照"`, []uint64{105}}, {"format:JPEG,png format:jpg", []uint64{105, 102, 104, 101}}, {".jpg format:webp", []uint64{103}}, {"album:#42,#57 sort:oldest", []uint64{103, 101, 102}}, {"minsize:1MB maxsize:1MB", []uint64{101}}, {`camera:"Canon EOS"`, []uint64{101}}, {"100%_", []uint64{104}}, {"visibility:public", []uint64{105, 102}}, {"format:unknown", []uint64{107}}, {"sort:oldest", []uint64{103, 101, 104, 107, 102, 105}}, {"minsize:0.000001MB maxsize:0.000001MB", []uint64{107}}, {"album:unfiled", []uint64{105, 107}}, {"minsize:0MB maxsize:0MB", []uint64{}}, {"after:2026-10-01 before:2026-10-02", []uint64{107, 104, 101}}}
		for _, v := range tests {
			p := searchquery.Parse(v.raw)
			if !p.OK {
				t.Fatal(p.Diagnostics)
			}
			q := model.ImageSearchFilterFromAST(*p.AST)
			for _, a := range p.AST.Filters.Albums {
				switch a.Kind {
				case "id":
					var id uint64
					if _, err := fmt.Sscan(a.Value, &id); err != nil {
						t.Fatal(err)
					}
					q.AlbumIDs = append(q.AlbumIDs, id)
				case "unfiled":
					q.IncludeUnfiled = true
				}
			}
			if p.AST.Filters.After != nil {
				x, _ := searchquery.DayStart(*p.AST.Filters.After, time.UTC)
				q.AfterUTC = &x
			}
			if p.AST.Filters.Before != nil {
				x, _ := searchquery.DayStart(*p.AST.Filters.Before, time.UTC)
				q.BeforeUTC = &x
			}
			got, total, err := f.images.List(t.Context(), model.ImageListFilter{UserID: f.user.ID, Search: &q}, 1, 20)
			ids := []uint64{}
			for _, im := range got {
				ids = append(ids, im.ID)
			}
			if err != nil || total != int64(len(v.ids)) || !reflect.DeepEqual(ids, v.ids) {
				t.Errorf("%q ids=%v total=%d err=%v want=%v", v.raw, ids, total, err, v.ids)
			}
		}
	})
}

func TestSearchLiteralUnicodeCameraAndAlbumResolution(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "unicode-owner")
		other := newImageFixture(t, db, "unicode-other")
		input := f.request("unicode", "unicode")
		input.Image.OriginName = `CAFÉ café 100%_[x]* O'Reilly <script>.JPG`
		image, err := f.images.ReserveUpload(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		if err = db.Model(&model.Image{}).Where("id = ?", image.ID).Update("state", model.ImageStateActive).Error; err != nil {
			t.Fatal(err)
		}
		foreign := reserveAndCommit(t, other, "unicode-foreign", "unicode-foreign")
		camera, lens := "canon eos r6", "rf24-105"
		if err = db.Create(&model.ImageExif{ImageID: image.ID, CameraSearch: &camera, LensSearch: &lens, Raw: []byte(`{}`)}).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.Model(&model.ImageExif{}).Where("image_id = ?", foreign.ID).Updates(map[string]any{"camera_search": camera, "lens_search": lens}).Error; err != nil {
			t.Fatal(err)
		}
		for _, v := range []struct {
			term  string
			count int64
		}{{"cafÉ", 1}, {"café", 1}, {"100%_[x]*", 1}, {"100___", 0}, {"O'Reilly", 1}, {"<script>", 1}, {"' OR 1=1 --", 0}} {
			got, total, err := f.images.List(t.Context(), model.ImageListFilter{UserID: f.user.ID, Search: &model.ImageSearchFilter{Terms: []string{searchquery.Normalize(v.term)}, Sort: "newest"}}, 1, 20)
			if err != nil || total != v.count || int64(len(got)) != v.count {
				t.Errorf("term=%q total=%d err=%v", v.term, total, err)
			}
		}
		for _, needle := range []string{"canon eos", "rf24-105"} {
			got, total, err := f.images.List(t.Context(), model.ImageListFilter{UserID: f.user.ID, Search: &model.ImageSearchFilter{Camera: &needle, Sort: "newest"}}, 1, 20)
			if err != nil || total != 1 || got[0].ID != image.ID {
				t.Errorf("camera owner fence total=%d err=%v", total, err)
			}
		}
		albums, _ := NewAlbumRepository(t.Context(), db)
		a, err := albums.Create(t.Context(), model.Album{UserID: f.user.ID, Name: "Cafe\u0301"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = albums.Create(t.Context(), model.Album{UserID: f.user.ID, Name: "Café"})
		if err != nil {
			t.Fatal(err)
		}
		nfc := "Café"
		rows, err := albums.SearchAlbums(t.Context(), f.user.ID, nil, &nfc, nil)
		if err != nil || len(rows) != 2 {
			t.Fatalf("NFC exact=%v err=%v", rows, err)
		}
		rows, err = albums.SearchAlbums(t.Context(), f.user.ID, nil, &nfc, &a.ID)
		if err != nil || len(rows) != 1 || rows[0].ID != a.ID {
			t.Fatalf("scope before limit/ambiguity=%v err=%v", rows, err)
		}
		lower := "café"
		rows, err = albums.SearchAlbums(t.Context(), f.user.ID, nil, &lower, nil)
		if err != nil || len(rows) != 0 {
			t.Fatalf("album names folded case=%v err=%v", rows, err)
		}
		if _, err := albums.Update(t.Context(), f.user.ID, a.ID, map[string]any{"name": "Changed E\u0301"}); err != nil {
			t.Fatal(err)
		}
		keyword := searchquery.Normalize("CHANGED É")
		rows, more, err := albums.SuggestAlbums(t.Context(), f.user.ID, keyword, 1, 20, nil)
		if err != nil || more || len(rows) != 1 || rows[0].ID != a.ID {
			t.Fatalf("rename suggestion=%v more=%t err=%v", rows, more, err)
		}
	})
}
