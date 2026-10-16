package lsky

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/biliblihuorong/imgnest/internal/storage"
)

func jobDump(content []byte) string {
	return fmt.Sprintf(`INSERT INTO "public"."groups" ("id", "name", "is_default", "is_guest", "configs", "created_at") VALUES ('1', 'g', TRUE, FALSE, '{}', NULL);
INSERT INTO "public"."users" ("id", "group_id", "name", "email", "password", "is_adminer", "status", "registered_ip", "created_at") VALUES ('1', '1', 'shell', 'shell@example.com', '%s', TRUE, '1', '', NULL);
INSERT INTO "public"."strategies" ("id", "key", "name", "configs") VALUES ('9', '10', 'B2', '{"url":"https://img.example.com/b"}'), ('2', '1', 'local', '{"url":"http://localhost/uploads"}');
INSERT INTO "public"."images" ("id", "user_id", "album_id", "strategy_id", "path", "name", "extension", "origin_name", "md5", "permission", "uploaded_ip", "created_at") VALUES
('1', '1', NULL, '9', '1/2025', 'a.png', 'png', 'a.png', '%s', '1', '', '2025-01-01 00:00:00'),
('2', '1', NULL, '9', '1/2025', 'missing.png', 'png', 'm.png', '', '1', '', '2025-01-01 00:00:00'),
('3', '1', NULL, '2', '1/2025', 'local.png', 'png', 'l.png', '', '1', '', '2025-01-01 00:00:00');
`, bcryptY, sum(content))
}

func waitIdle(t *testing.T, runner *Runner) service.LskyImportStatus {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := runner.Status(t.Context())
		if !status.Running {
			return status
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("import did not finish")
	return service.LskyImportStatus{}
}

type fakeObject struct {
	data    []byte
	owner   string
	version string
}

func TestRunnerAdoptsUploadedDump(t *testing.T) {
	db, policyID := target(t)
	ctx := t.Context()
	content := []byte("png-bytes")
	objects := map[string]fakeObject{"1/2025/a.png": {data: content, version: "v1"}}
	var ingested []Item
	runner, err := NewRunner(ctx, RunnerDeps{
		DB: db,
		Ingest: func(_ context.Context, policy uint64, item Item) error {
			if policy != policyID {
				return errors.New("wrong rule")
			}
			ingested = append(ingested, item)
			return nil
		},
		Adoption: func(context.Context, uint64) (service.ObjectOpener, error) {
			return func(_ context.Context, key string) (io.ReadCloser, storage.ObjectInfo, error) {
				object, ok := objects[key]
				if !ok {
					return nil, storage.ObjectInfo{}, storage.ErrNotFound
				}
				return io.NopCloser(bytes.NewReader(object.data)), storage.ObjectInfo{Size: int64(len(object.data)), OwnerID: object.owner, VersionID: object.version}, nil
			}, nil
		},
		Now: func() time.Time { return time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Start(ctx, service.LskyImportOptions{PolicyID: policyID, Mode: service.LskyModeAdopt}); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("start without a dump = %v", err)
	}
	if _, err := runner.LoadDump(ctx, strings.NewReader("not sql at all"), "Asia/Shanghai"); !errors.Is(err, service.ErrNotLskyDump) {
		t.Fatalf("garbage upload = %v", err)
	}
	status, err := runner.LoadDump(ctx, strings.NewReader(jobDump(content)), "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Loaded || status.Summary.Users != 1 || status.Summary.Images != 3 || len(status.Summary.Strategies) != 2 || status.Summary.Strategies[1].Images != 2 {
		t.Fatalf("summary = %+v", status.Summary)
	}
	if _, err := runner.Start(ctx, service.LskyImportOptions{PolicyID: policyID, Mode: "move"}); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("unknown mode = %v", err)
	}
	if _, err := runner.Start(ctx, service.LskyImportOptions{PolicyID: policyID, Mode: service.LskyModeAdopt, StrategyID: 9}); err != nil {
		t.Fatal(err)
	}
	status = waitIdle(t, runner)
	if status.Phase != service.LskyPhaseDone || status.Running || status.Total != 2 || status.Imported != 1 || status.Failed != 1 || status.Processed != 2 {
		t.Fatalf("status = %+v", status)
	}
	if len(ingested) != 1 || ingested[0].Version != "v1" || ingested[0].Path != "1/2025/a" || !strings.Contains(status.Failures[0], "没有这个文件") {
		t.Fatalf("ingested %+v, failures %v", ingested, status.Failures)
	}
	if len(status.Accounts) == 0 || status.FinishedAt == nil {
		t.Fatalf("accounts report missing: %+v", status)
	}
	if status, err = runner.Discard(ctx); err != nil || status.Loaded {
		t.Fatalf("discard = %+v, %v", status, err)
	}
}

func TestStorageFetcherRefusesOwnedAndUnversionedObjects(t *testing.T) {
	cases := map[string]fakeObject{"owned": {data: []byte("x"), owner: "img", version: "v"}, "unversioned": {data: []byte("x"), version: "null"}}
	for name, object := range cases {
		fetch := StorageFetcher{Open: func(context.Context, string) (io.ReadCloser, storage.ObjectInfo, error) {
			return io.NopCloser(bytes.NewReader(object.data)), storage.ObjectInfo{OwnerID: object.owner, VersionID: object.version}, nil
		}}
		if _, err := fetch.Fetch(t.Context(), Image{Pathname: "a.png"}, Strategy{}); err == nil {
			t.Fatalf("%s object was fetched for adoption", name)
		}
	}
}
