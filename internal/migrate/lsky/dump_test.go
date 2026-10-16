package lsky

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// dbxDump mimics the DBX/Navicat style: quoted identifiers, multi-row
// INSERTs, every value quoted, bare booleans and casts in defaults.
const dbxDump = `-- Database export: LsyPro
CREATE TABLE "public"."configs" (
  "name" character varying(32) NOT NULL,
  "value" text,
  CONSTRAINT "configs_name_unique" UNIQUE (name)
);
INSERT INTO "public"."configs" ("name", "value") VALUES
('mail', '{"password":"smtp-secret; still a string"}');

CREATE TABLE "public"."groups" ("id" bigint, "intro" character varying(255) NOT NULL DEFAULT ''::character varying);
INSERT INTO "public"."groups" ("id", "name", "is_default", "is_guest", "configs", "created_at", "updated_at") VALUES
('1', '默认组', TRUE, TRUE, '{"maximum_file_size":5120,"limit_per_minute":20,"accepted_file_suffixes":["jpg","png"]}', '2024-04-12 14:44:55', NULL),
('3', 'it''s 博客', FALSE, FALSE, '{"maximum_file_size":"51200.5"}', '0001-01-01 00:00:00', NULL);

INSERT INTO "public"."users" ("id", "group_id", "name", "email", "password", "remember_token", "is_adminer", "status", "registered_ip", "created_at") VALUES
('1', '3', 'Shell', 'shell@example.com', '$2y$10$hash', 'remember-secret', TRUE, '1', '127.0.0.1', '2024-04-12 14:44:55'),
('2', NULL, 'off', 'off@example.com', '$2y$10$hash2', NULL, FALSE, '0', '', NULL);

INSERT INTO "public"."personal_access_tokens" ("id", "tokenable_type", "tokenable_id", "name", "token", "abilities", "last_used_at", "created_at") VALUES
('7', 'App\Models\User', '1', 'picgo', 'abc123', '["*"]', '2025-01-01 08:00:00', '2024-05-01 00:00:00'),
('8', 'App\Models\Robot', '1', 'bot', 'zzz', NULL, NULL, NULL);

INSERT INTO "public"."strategies" ("id", "key", "name", "intro", "configs", "created_at") VALUES
('9', '10', 'B2', '', '{"url":"https://img.example.com/Bucket/","access_key_id":"AKIA-SECRET","secret_access_key":"SECRET-KEY"}', NULL);

INSERT INTO "public"."albums" ("id", "user_id", "name", "intro", "created_at") VALUES ('4', '1', 'blog', E'line\none \'q\'', '2024-06-01 10:00:00');

INSERT INTO "public"."images" ("id", "user_id", "album_id", "group_id", "strategy_id", "key", "path", "name", "origin_name", "alias_name", "size", "mimetype", "extension", "md5", "sha1", "width", "height", "permission", "is_unhealthy", "uploaded_ip", "created_at", "updated_at") VALUES
('2', '1', '4', '3', '9', 'k', '2025/07/25', '688328dadcaae.PNG', 'a (1).png', '', '12.5', 'image/png', 'PNG', 'aa', 'bb', '10', '10', '1', FALSE, '1.2.3.4', '2025-07-25 14:49:00', NULL),
('1', NULL, NULL, NULL, '9', 'k', '123/01/27', '63d342d7133e6.png', 'x;y.png', '', '1', 'image/png', 'png', 'cc', 'dd', '1', '1', '0', FALSE, '', '0001-01-01 11:19:51', NULL);
SELECT setval('"public"."images_id_seq"', 296, true);
CREATE FUNCTION f() RETURNS trigger AS $body$ BEGIN RETURN 'a;b'; END; $body$ LANGUAGE plpgsql;
`

func TestParseDumpDBXStyle(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	snap, err := ParseDump(strings.NewReader(dbxDump), loc)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Groups) != 2 || snap.Groups[0].Name != "默认组" || !snap.Groups[0].IsGuest || snap.Groups[0].MaxFileKB != 5120 || snap.Groups[0].PerMinute != 20 {
		t.Fatalf("groups = %+v", snap.Groups)
	}
	if snap.Groups[1].Name != "it's 博客" || snap.Groups[1].MaxFileKB != 51200.5 || !snap.Groups[1].CreatedAt.IsZero() {
		t.Fatalf("second group = %+v", snap.Groups[1])
	}
	if !snap.Groups[0].CreatedAt.Equal(time.Date(2024, 4, 12, 6, 44, 55, 0, time.UTC)) {
		t.Fatalf("zone-less stamp = %v", snap.Groups[0].CreatedAt)
	}
	if len(snap.Users) != 2 || !snap.Users[0].IsAdmin || !snap.Users[0].Enabled || snap.Users[0].GroupID != 3 || snap.Users[1].Enabled || snap.Users[1].GroupID != 0 {
		t.Fatalf("users = %+v", snap.Users)
	}
	if len(snap.Tokens) != 1 || snap.Tokens[0].Hash != "abc123" || snap.Tokens[0].LastUsedAt == nil {
		t.Fatalf("tokens = %+v", snap.Tokens)
	}
	if len(snap.Strategies) != 1 || snap.Strategies[0].URL != "https://img.example.com/Bucket" || snap.Strategies[0].Key != 10 {
		t.Fatalf("strategies = %+v", snap.Strategies)
	}
	if snap.Albums[0].Intro != "line\none 'q'" {
		t.Fatalf("E-string = %q", snap.Albums[0].Intro)
	}
	if len(snap.Images) != 2 || snap.Images[0].ID != 1 || snap.Images[0].UserID != 0 || snap.Images[0].Pathname != "123/01/27/63d342d7133e6.png" {
		t.Fatalf("images = %+v", snap.Images)
	}
	if !snap.Images[0].CreatedAt.Equal(uniqidTime("63d342d7133e6.png")) {
		t.Fatalf("placeholder stamp fallback = %v", snap.Images[0].CreatedAt)
	}
	second := snap.Images[1]
	if second.Pathname != "2025/07/25/688328dadcaae.PNG" || second.Extension != "png" || !second.IsPublic || second.AlbumID != 4 || second.OriginName != "a (1).png" {
		t.Fatalf("image = %+v", second)
	}
	// Credentials of other tables and strategies never reach the snapshot.
	for _, secret := range []string{"SECRET", "smtp-secret", "remember-secret"} {
		if strings.Contains(strings.Join([]string{snap.Strategies[0].Name, snap.Strategies[0].URL}, " "), secret) {
			t.Fatalf("snapshot carries %q", secret)
		}
	}
}

// pgDump mimics plain pg_dump output with COPY blocks.
const pgDump = `--
-- PostgreSQL database dump
--
SET statement_timeout = 0;
SELECT pg_catalog.set_config('search_path', '', false);
CREATE TABLE public.users (id bigint NOT NULL, name character varying(255));
COPY public.groups (id, name, is_default, is_guest, configs, created_at, updated_at) FROM stdin;
1	默认组	t	t	{"maximum_file_size":5120}	2024-04-12 14:44:55	\N
\.

COPY public.users (id, group_id, name, email, password, is_adminer, status, registered_ip, created_at) FROM stdin;
1	1	Tab\tName	a@example.com	$2y$10$hash	f	1	\N	2024-04-12 14:44:55+00
\.

COPY public.failed_jobs (id, payload) FROM stdin;
1	ignored\ttext
\.

COPY public.images (id, user_id, album_id, strategy_id, path, name, extension, origin_name, md5, permission, uploaded_ip, created_at) FROM stdin;
5	1	\N	9	2025/01/01	a.jpg	jpg	a.jpg	ff	1	::1	2025-01-01 00:00:00.123456
\.
`

func TestParseDumpPgDumpCopy(t *testing.T) {
	snap, err := ParseDump(strings.NewReader(pgDump), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Groups) != 1 || !snap.Groups[0].IsDefault {
		t.Fatalf("groups = %+v", snap.Groups)
	}
	if len(snap.Users) != 1 || snap.Users[0].Name != "Tab\tName" || snap.Users[0].IsAdmin || snap.Users[0].RegisteredIP != "" {
		t.Fatalf("users = %+v", snap.Users)
	}
	if !snap.Users[0].CreatedAt.Equal(time.Date(2024, 4, 12, 14, 44, 55, 0, time.UTC)) {
		t.Fatalf("offset stamp = %v", snap.Users[0].CreatedAt)
	}
	if len(snap.Images) != 1 || snap.Images[0].Pathname != "2025/01/01/a.jpg" || snap.Images[0].AlbumID != 0 || snap.Images[0].IP != "::1" {
		t.Fatalf("images = %+v", snap.Images)
	}
}

func TestParseDumpRejectsOtherFiles(t *testing.T) {
	for _, body := range []string{"", "hello world", "CREATE TABLE t (id int);", "\x89PNG\r\n"} {
		if _, err := ParseDump(strings.NewReader(body), time.UTC); !errors.Is(err, ErrNotLskyDump) {
			t.Fatalf("%q: err = %v, want ErrNotLskyDump", body, err)
		}
	}
	if _, err := ParseDump(strings.NewReader(`INSERT INTO "users" ("id") VALUES ('1', '2');`), time.UTC); err == nil {
		t.Fatal("a row with more values than columns must fail")
	}
	if _, err := ParseDump(strings.NewReader("COPY public.users (id) FROM stdin;\n1\n"), time.UTC); err == nil {
		t.Fatal("an unterminated COPY block must fail")
	}
}
