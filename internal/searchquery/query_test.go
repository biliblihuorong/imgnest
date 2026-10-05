package searchquery

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSharedQueryVectors(t *testing.T) {
	data, err := os.ReadFile("testdata/query-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var suite struct {
		Cases []struct {
			ID          string `json:"id"`
			Raw         string `json:"raw"`
			Canonical   string `json:"canonical"`
			AST         *AST   `json:"ast"`
			Diagnostics []struct {
				Code string `json:"code"`
				Span Span   `json:"span"`
			} `json:"diagnostics"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(data, &suite); err != nil {
		t.Fatal(err)
	}
	for _, v := range suite.Cases {
		t.Run(v.ID, func(t *testing.T) {
			p := Parse(v.Raw)
			if len(v.Diagnostics) > 0 {
				if p.OK || p.AST != nil || len(p.Diagnostics) != len(v.Diagnostics) {
					t.Fatalf("unexpected result %+v", p)
				}
				for i, d := range v.Diagnostics {
					if p.Diagnostics[i].Code != d.Code || p.Diagnostics[i].Span != d.Span {
						t.Fatalf("diagnostic=%+v want=%+v", p.Diagnostics[i], d)
					}
				}
				return
			}
			if !p.OK {
				t.Fatalf("parse: %+v", p.Diagnostics)
			}
			if v.AST != nil && !reflect.DeepEqual(p.AST, v.AST) {
				t.Fatalf("ast=%+v want=%+v", p.AST, v.AST)
			}
			s := Serialize(*p.AST)
			if s != v.Canonical {
				t.Fatalf("canonical=%q want=%q", s, v.Canonical)
			}
			again := Parse(s)
			if !again.OK || !reflect.DeepEqual(p.AST, again.AST) || Serialize(*again.AST) != s {
				t.Fatalf("roundtrip: %+v", again)
			}
		})
	}
}

func TestLimitsAndStrictGrammar(t *testing.T) {
	tests := []struct{ raw, code string }{
		{strings.Repeat("a", 4097), "QUERY_TOO_COMPLEX"}, {strings.Repeat("a ", 33), "QUERY_TOO_COMPLEX"},
		{strings.Repeat("a ", 17), "QUERY_TOO_COMPLEX"}, {"a b c d e f g h i", "QUERY_TOO_COMPLEX"},
		{"album:" + strings.Repeat("#1,", 10) + "#1", "QUERY_TOO_COMPLEX"}, {`album:#01`, "INVALID_ENUM"},
		{`ab"cd"`, "UNEXPECTED_CHARACTER"}, {`"ab"cd`, "UNEXPECTED_CHARACTER"}, {`camera:a,b`, "UNEXPECTED_CHARACTER"},
		{"\"a\nb\"", "CONTROL_CHARACTER"}, {"a\x00b", "CONTROL_CHARACTER"}, {`a\b`, "INVALID_ESCAPE"},
		{`is:all`, "INVALID_ENUM"}, {`order:newest`, "INVALID_ENUM"}, {`format:.jpg`, "INVALID_ENUM"},
	}
	for _, v := range tests {
		p := Parse(v.raw)
		if p.OK || p.Diagnostics[0].Code != v.code {
			t.Errorf("%q: %+v want %s", v.raw, p, v.code)
		}
	}
}

func TestDayBoundaries(t *testing.T) {
	for _, v := range []struct{ day, zone, want string }{
		{"2026-10-01", "Asia/Shanghai", "2026-09-30T16:00:00Z"},
		{"2026-03-08", "America/New_York", "2026-03-08T05:00:00Z"},
		{"2026-03-09", "America/New_York", "2026-03-09T04:00:00Z"},
		{"2026-11-01", "America/New_York", "2026-11-01T04:00:00Z"},
		{"2026-11-02", "America/New_York", "2026-11-02T05:00:00Z"},
		{"2018-11-04", "America/Sao_Paulo", "2018-11-04T03:00:00Z"},
	} {
		loc, err := Location(v.zone)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DayStart(v.day, loc)
		if err != nil || got.Format(time.RFC3339) != v.want {
			t.Errorf("%+v: %v %v", v, got, err)
		}
	}
	loc, _ := Location("Pacific/Apia")
	if _, err := DayStart("2011-12-30", loc); err == nil {
		t.Error("accepted skipped day")
	}
	for _, z := range []string{"", "Local", "Bad/Zone", "GMT+08:00", "../UTC"} {
		if _, err := Location(z); err == nil {
			t.Errorf("accepted zone %q", z)
		}
	}
}

func TestCanonicalQueryFitsProtocolLimit(t *testing.T) {
	terms := []string{}
	for i := 0; i < 8; i++ {
		n := 128
		if i == 7 {
			n = 125
		}
		terms = append(terms, strings.Repeat(string(rune(0x1f600+i)), n))
	}
	raw := strings.Join(terms, " ")
	if len(raw) != 4091 {
		t.Fatal(len(raw))
	}
	result := Parse(raw)
	if result.OK || result.Diagnostics[0].Code != "QUERY_TOO_COMPLEX" {
		t.Fatalf("accepted non-roundtrippable canonical query: %+v", result)
	}
}

func FuzzParseSerialize(f *testing.F) {
	for _, seed := range []string{"", `album:"A,B",#42 camera:"Canon EOS"`, "暑假 format:JPEG sort:newest", `"C:\\photos\\a.png"`, "😀 unknown:bad", "Cafe\u0301 café", `album:#trip`, "minsize:0MB maxsize:0.000001MB"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		parsed := Parse(raw)
		if !parsed.OK {
			if parsed.AST != nil || len(parsed.Diagnostics) < 1 || len(parsed.Diagnostics) > 5 {
				t.Fatal("invalid failure shape")
			}
			for _, d := range parsed.Diagnostics {
				if d.Span.Start < 0 || d.Span.End < d.Span.Start || d.Span.End > utf16len(raw) {
					t.Fatalf("invalid diagnostic range %+v", d)
				}
			}
			return
		}
		canonical := Serialize(*parsed.AST)
		again := Parse(canonical)
		if !again.OK || !reflect.DeepEqual(parsed.AST, again.AST) || Serialize(*again.AST) != canonical {
			t.Fatalf("roundtrip raw=%q canonical=%q result=%+v", raw, canonical, again)
		}
	})
}
