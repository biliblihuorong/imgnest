package repo

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type searchTimings struct {
	logger.Interface
	countDurations   []time.Duration
	countSQL, rowSQL string
}

func (s *searchTimings) Trace(_ context.Context, begin time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	if strings.Contains(strings.ToLower(sql), "select count(") {
		s.countDurations = append(s.countDurations, time.Since(begin))
		s.countSQL = sql
	} else if strings.HasPrefix(strings.ToLower(sql), "select") {
		s.rowSQL = sql
	}
}
func percentile(values []time.Duration, p int) time.Duration {
	v := append([]time.Duration{}, values...)
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	return v[(len(v)-1)*p/100]
}

// TestSearchPerformance is deliberately opt-in: it builds disposable synthetic
// 10k/100k libraries, verifies totals, reports p50/p95 and COUNT time, and emits
// real database query plans. No production credentials or data are required.
func TestSearchPerformance(t *testing.T) {
	if os.Getenv("IMGNEST_BENCH_SEARCH") != "1" {
		t.Skip("set IMGNEST_BENCH_SEARCH=1 for 10k/100k timing and query-plan verification")
	}
	t.Logf("runtime=%s %s/%s CPUs=%d; warm synthetic data, 21 measured iterations, page size 20", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
	for _, count := range []int{10000, 100000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
				own := newImageFixture(t, db, "perf-owner")
				other := newImageFixture(t, db, "perf-other")
				albums, _ := NewAlbumRepository(t.Context(), db)
				a, err := albums.Create(t.Context(), model.Album{UserID: own.user.ID, Name: "large album"})
				if err != nil {
					t.Fatal(err)
				}
				base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
				after := base.AddDate(0, 0, 14)
				total, formatDate, albumTotal := int64(0), int64(0), int64(0)
				batch := make([]model.Image, 0, 500)
				for i := 0; i < count; i++ {
					owner := own
					if i%10 == 9 {
						owner = other
					} else {
						total++
					}
					name := "shared-name.jpg"
					if i == 0 {
						name = "rare-needle.jpg"
					}
					mime := "image/jpeg"
					if i%2 == 0 {
						mime = "image/png"
					}
					if i%5 == 4 {
						mime = "application/octet-stream"
					}
					at := base.AddDate(0, 0, i%28)
					if owner.user.ID == own.user.ID && i%3 == 0 {
						albumTotal++
					}
					if owner.user.ID == own.user.ID && mime == "image/png" && !at.Before(after) {
						formatDate++
					}
					key := fmt.Sprintf("perf-%d", i)
					search := name
					batch = append(batch, model.Image{UserID: owner.user.ID, PolicyID: owner.policy.ID, StorageID: owner.storage.ID, Key: key, Path: key, OriginName: name, FilenameSearch: &search, MIME: mime, Ext: "jpg", State: model.ImageStateActive, Size: int64(i%10000 + 1), Width: 1, Height: 1, Frames: 1, ObjectManifest: []model.ObjectReceipt{}, CreatedAt: at})
					if len(batch) == cap(batch) {
						if err := db.CreateInBatches(batch, 500).Error; err != nil {
							t.Fatal(err)
						}
						batch = batch[:0]
					}
				}
				if len(batch) > 0 {
					if err := db.CreateInBatches(batch, 500).Error; err != nil {
						t.Fatal(err)
					}
				}
				if err := db.Model(&model.Image{}).Where("user_id = ? AND CAST(SUBSTR(key, 6) AS BIGINT) % 3 = 0", own.user.ID).Update("album_id", a.ID).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Exec("ANALYZE").Error; err != nil {
					t.Fatal(err)
				}
				tests := []struct {
					name   string
					filter model.ImageSearchFilter
					total  int64
				}{{"empty", model.ImageSearchFilter{Sort: "newest"}, total}, {"format-date", model.ImageSearchFilter{Formats: []string{"png"}, AfterUTC: &after, Sort: "newest"}, formatDate}, {"album", model.ImageSearchFilter{AlbumIDs: []uint64{a.ID}, Sort: "newest"}, albumTotal}, {"rare", model.ImageSearchFilter{Terms: []string{"rare-needle"}, Sort: "newest"}, 1}, {"no-hit", model.ImageSearchFilter{Terms: []string{"never-present"}, Sort: "newest"}, 0}}
				for _, v := range tests {
					t.Run(v.name, func(t *testing.T) {
						timings := &searchTimings{Interface: logger.Default.LogMode(logger.Silent)}
						r := &ImageRepository{db: db.Session(&gorm.Session{Logger: timings})}
						durations := []time.Duration{}
						for i := 0; i < 22; i++ {
							start := time.Now()
							items, total, err := r.List(t.Context(), model.ImageListFilter{UserID: own.user.ID, Search: &v.filter}, 1, 20)
							if err != nil || total != v.total || len(items) != min(20, int(v.total)) {
								t.Fatalf("total=%d want=%d rows=%d err=%v", total, v.total, len(items), err)
							}
							if i > 0 {
								durations = append(durations, time.Since(start))
							}
						}
						t.Logf("rows=%d matched=%d p50=%s p95=%s COUNT-p50=%s COUNT-p95=%s", count, v.total, percentile(durations, 50), percentile(durations, 95), percentile(timings.countDurations[1:], 50), percentile(timings.countDurations[1:], 95))
						for kind, sql := range map[string]string{"count": timings.countSQL, "rows": timings.rowSQL} {
							prefix := "EXPLAIN QUERY PLAN "
							if db.Name() == "postgres" {
								prefix = "EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) "
							}
							rows, err := db.Raw(prefix + sql).Rows()
							if err != nil {
								t.Fatal(err)
							}
							cols, err := rows.Columns()
							if err != nil {
								_ = rows.Close()
								t.Fatal(err)
							}
							plan := []string{}
							for rows.Next() {
								values := make([]any, len(cols))
								targets := make([]any, len(cols))
								for i := range values {
									targets[i] = &values[i]
								}
								if err := rows.Scan(targets...); err != nil {
									_ = rows.Close()
									t.Fatal(err)
								}
								for i, v := range values {
									if b, ok := v.([]byte); ok {
										values[i] = string(b)
									}
								}
								plan = append(plan, fmt.Sprint(values...))
							}
							if err := rows.Close(); err != nil {
								t.Fatal(err)
							}
							t.Logf("%s plan:\n%s", kind, strings.Join(plan, "\n"))
						}
					})
				}
			})
		})
	}
}
