package lsky

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/biliblihuorong/imgnest/internal/service"
)

// MaxDumpBytes bounds an uploaded SQL export.
const MaxDumpBytes = service.LskyMaxDumpBytes

// ErrNotLskyDump rejects a file that holds no Lsky Pro tables.
var ErrNotLskyDump = errors.New("not a Lsky Pro database export")

// dumpTables are the only tables kept; everything else (configs with SMTP
// passwords, sessions, jobs) is skipped while lexing and never stored.
var dumpTables = map[string]bool{"groups": true, "users": true, "personal_access_tokens": true, "albums": true, "strategies": true, "images": true}

type dumpRow map[string]*string

// ParseDump reads a plain-SQL export of a Lsky Pro PostgreSQL database:
// pg_dump output (COPY blocks or --inserts) and the multi-row INSERT files
// GUI tools such as DBX or Navicat write. Zone-less timestamps are read in
// loc. Storage strategy credentials are dropped as each row is read; only
// the public URL is kept.
func ParseDump(r io.Reader, loc *time.Location) (Snapshot, error) {
	if r == nil || loc == nil {
		return Snapshot{}, errors.New("parse lsky dump: input and time zone are required")
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxDumpBytes+1))
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse lsky dump: %w", err)
	}
	if len(data) > MaxDumpBytes {
		return Snapshot{}, fmt.Errorf("parse lsky dump: larger than %d MiB", MaxDumpBytes>>20)
	}
	tables, err := parseStatements(data)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse lsky dump: %w", err)
	}
	if tables["users"] == nil || tables["images"] == nil && tables["groups"] == nil {
		return Snapshot{}, ErrNotLskyDump
	}
	return snapshotFromRows(tables, loc)
}

// parseStatements walks the file statement by statement, keeping the rows
// of INSERT and COPY statements that target dumpTables.
func parseStatements(data []byte) (map[string][]dumpRow, error) {
	tables := map[string][]dumpRow{}
	l := &lexer{src: data}
	for {
		l.skipSpace()
		if l.eof() {
			return tables, nil
		}
		start := l.pos
		word := strings.ToUpper(l.word())
		switch word {
		case "INSERT":
			if err := l.insert(tables); err != nil {
				return nil, fmt.Errorf("line %d: %w", l.line(start), err)
			}
		case "COPY":
			if err := l.copyBlock(tables); err != nil {
				return nil, fmt.Errorf("line %d: %w", l.line(start), err)
			}
		default:
			if err := l.skipStatement(); err != nil {
				return nil, fmt.Errorf("line %d: %w", l.line(start), err)
			}
		}
	}
}

type lexer struct {
	src []byte
	pos int
}

func (l *lexer) eof() bool { return l.pos >= len(l.src) }

func (l *lexer) line(at int) int { return bytes.Count(l.src[:min(at, len(l.src))], []byte("\n")) + 1 }

func (l *lexer) peek() byte {
	if l.eof() {
		return 0
	}
	return l.src[l.pos]
}

// skipSpace skips whitespace and SQL comments.
func (l *lexer) skipSpace() {
	for !l.eof() {
		c := l.src[l.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			l.pos++
		case c == '-' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '-':
			for !l.eof() && l.src[l.pos] != '\n' {
				l.pos++
			}
		case c == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '*':
			end := bytes.Index(l.src[l.pos+2:], []byte("*/"))
			if end < 0 {
				l.pos = len(l.src)
				return
			}
			l.pos += end + 4
		default:
			return
		}
	}
}

func (l *lexer) word() string {
	start := l.pos
	for !l.eof() {
		c := l.src[l.pos]
		if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			break
		}
		l.pos++
	}
	return string(l.src[start:l.pos])
}

func (l *lexer) keyword(want string) bool {
	l.skipSpace()
	save := l.pos
	if strings.EqualFold(l.word(), want) {
		return true
	}
	l.pos = save
	return false
}

func (l *lexer) expect(c byte) error {
	l.skipSpace()
	if l.peek() != c {
		return fmt.Errorf("expected %q", c)
	}
	l.pos++
	return nil
}

// ident reads one possibly quoted identifier.
func (l *lexer) ident() (string, error) {
	l.skipSpace()
	if l.peek() != '"' {
		name := l.word()
		if name == "" {
			return "", errors.New("expected identifier")
		}
		return strings.ToLower(name), nil
	}
	l.pos++
	var b strings.Builder
	for !l.eof() {
		c := l.src[l.pos]
		l.pos++
		if c == '"' {
			if l.peek() == '"' {
				l.pos++
				b.WriteByte('"')
				continue
			}
			return b.String(), nil
		}
		b.WriteByte(c)
	}
	return "", errors.New("unterminated identifier")
}

// tableName reads [schema.]table and returns the table part.
func (l *lexer) tableName() (string, error) {
	name, err := l.ident()
	if err != nil {
		return "", err
	}
	for l.peek() == '.' {
		l.pos++
		if name, err = l.ident(); err != nil {
			return "", err
		}
	}
	return name, nil
}

func (l *lexer) columns() ([]string, error) {
	if err := l.expect('('); err != nil {
		return nil, err
	}
	var cols []string
	for {
		col, err := l.ident()
		if err != nil {
			return nil, err
		}
		cols = append(cols, col)
		l.skipSpace()
		switch l.peek() {
		case ',':
			l.pos++
		case ')':
			l.pos++
			return cols, nil
		default:
			return nil, errors.New("malformed column list")
		}
	}
}

// skipStatement advances past the next top-level semicolon, honouring
// string, identifier and dollar quoting.
func (l *lexer) skipStatement() error {
	for !l.eof() {
		c := l.src[l.pos]
		switch c {
		case ';':
			l.pos++
			return nil
		case '\'':
			if _, err := l.str(false); err != nil {
				return err
			}
		case '"':
			if _, err := l.ident(); err != nil {
				return err
			}
		case '$':
			if !l.dollar() {
				l.pos++
			}
		case '-', '/':
			before := l.pos
			l.skipSpace()
			if l.pos == before {
				l.pos++
			}
		default:
			l.pos++
		}
	}
	return nil
}

// dollar skips a $tag$...$tag$ body when one starts here.
func (l *lexer) dollar() bool {
	end := bytes.IndexByte(l.src[l.pos+1:], '$')
	if end < 0 {
		return false
	}
	tag := l.src[l.pos : l.pos+end+2]
	for _, c := range tag[1 : len(tag)-1] {
		if c != '_' && !unicode.IsLetter(rune(c)) && !unicode.IsDigit(rune(c)) {
			return false
		}
	}
	close := bytes.Index(l.src[l.pos+len(tag):], tag)
	if close < 0 {
		l.pos = len(l.src)
		return true
	}
	l.pos += len(tag) + close + len(tag)
	return true
}

func (l *lexer) insert(tables map[string][]dumpRow) error {
	if !l.keyword("INTO") {
		return l.skipStatement()
	}
	table, err := l.tableName()
	if err != nil {
		return err
	}
	if !dumpTables[table] {
		return l.skipStatement()
	}
	l.skipSpace()
	if l.peek() != '(' {
		return fmt.Errorf("INSERT into %s without a column list", table)
	}
	cols, err := l.columns()
	if err != nil {
		return err
	}
	if !l.keyword("VALUES") {
		return fmt.Errorf("INSERT into %s without VALUES", table)
	}
	for {
		values, err := l.tuple()
		if err != nil {
			return err
		}
		if len(values) != len(cols) {
			return fmt.Errorf("%s row has %d values for %d columns", table, len(values), len(cols))
		}
		row := dumpRow{}
		for i, col := range cols {
			row[col] = values[i]
		}
		tables[table] = append(tables[table], row)
		l.skipSpace()
		if l.peek() == ',' {
			l.pos++
			continue
		}
		return l.skipStatement()
	}
}

// tuple reads one parenthesised VALUES row; nil entries are SQL NULL.
func (l *lexer) tuple() ([]*string, error) {
	if err := l.expect('('); err != nil {
		return nil, err
	}
	var values []*string
	for {
		value, err := l.value()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		l.skipSpace()
		switch l.peek() {
		case ',':
			l.pos++
		case ')':
			l.pos++
			return values, nil
		default:
			return nil, errors.New("malformed VALUES row")
		}
	}
}

func (l *lexer) value() (*string, error) {
	l.skipSpace()
	var text string
	c := l.peek()
	switch {
	case c == '\'':
		s, err := l.str(false)
		if err != nil {
			return nil, err
		}
		text = s
	case (c == 'E' || c == 'e') && l.pos+1 < len(l.src) && l.src[l.pos+1] == '\'':
		l.pos++
		s, err := l.str(true)
		if err != nil {
			return nil, err
		}
		text = s
	case c == '-' || c == '+' || c == '.' || (c >= '0' && c <= '9'):
		start := l.pos
		l.pos++
		for !l.eof() && strings.IndexByte("0123456789.eE+-", l.src[l.pos]) >= 0 {
			l.pos++
		}
		text = string(l.src[start:l.pos])
	default:
		word := strings.ToUpper(l.word())
		switch word {
		case "NULL":
			l.cast()
			return nil, nil
		case "TRUE", "FALSE":
			text = strings.ToLower(word)
		case "":
			return nil, errors.New("unsupported value")
		default:
			// A function call such as now() or nextval(...): keep its text.
			start := l.pos - len(word)
			if l.peek() == '(' {
				if err := l.skipParens(); err != nil {
					return nil, err
				}
			}
			text = string(l.src[start:l.pos])
		}
	}
	l.cast()
	return &text, nil
}

// cast skips a trailing ::type, including modifiers and array brackets.
func (l *lexer) cast() {
	for {
		l.skipSpace()
		if l.pos+1 >= len(l.src) || l.src[l.pos] != ':' || l.src[l.pos+1] != ':' {
			return
		}
		l.pos += 2
		l.skipSpace()
		if l.peek() == '"' {
			_, _ = l.ident()
		} else {
			l.word()
			// Multi-word types: character varying, timestamp without time zone.
			for l.keyword("varying") || l.keyword("precision") || l.keyword("without") || l.keyword("with") || l.keyword("time") || l.keyword("zone") {
			}
		}
		for l.peek() == '.' {
			l.pos++
			l.word()
		}
		l.skipSpace()
		if l.peek() == '(' {
			_ = l.skipParens()
		}
		for l.peek() == '[' {
			for !l.eof() && l.src[l.pos] != ']' {
				l.pos++
			}
			l.pos++
		}
	}
}

func (l *lexer) skipParens() error {
	depth := 0
	for !l.eof() {
		switch l.src[l.pos] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				l.pos++
				return nil
			}
		case '\'':
			if _, err := l.str(false); err != nil {
				return err
			}
			continue
		}
		l.pos++
	}
	return errors.New("unbalanced parentheses")
}

// str reads a quoted literal; escapes enables E” backslash sequences.
func (l *lexer) str(escapes bool) (string, error) {
	l.pos++ // opening quote
	var b strings.Builder
	for !l.eof() {
		c := l.src[l.pos]
		l.pos++
		switch {
		case c == '\'':
			if l.peek() == '\'' {
				l.pos++
				b.WriteByte('\'')
				continue
			}
			return b.String(), nil
		case c == '\\' && escapes && !l.eof():
			b.WriteString(unescape(l))
		default:
			b.WriteByte(c)
		}
	}
	return "", errors.New("unterminated string")
}

// unescape decodes one backslash sequence of an E” string or COPY field;
// the backslash itself has been consumed.
func unescape(l *lexer) string {
	c := l.src[l.pos]
	l.pos++
	switch c {
	case 'n':
		return "\n"
	case 't':
		return "\t"
	case 'r':
		return "\r"
	case 'b':
		return "\b"
	case 'f':
		return "\f"
	case 'v':
		return "\v"
	case 'x':
		n := 0
		for n < 2 && !l.eof() && strings.IndexByte("0123456789abcdefABCDEF", l.src[l.pos]) >= 0 {
			l.pos++
			n++
		}
		if n == 0 {
			return "x"
		}
		v, _ := strconv.ParseUint(string(l.src[l.pos-n:l.pos]), 16, 8)
		return string([]byte{byte(v)})
	case 'u', 'U':
		width := 4
		if c == 'U' {
			width = 8
		}
		if l.pos+width <= len(l.src) {
			if v, err := strconv.ParseInt(string(l.src[l.pos:l.pos+width]), 16, 32); err == nil {
				l.pos += width
				return string(rune(v)) // out-of-range code points become U+FFFD
			}
		}
		return string(c)
	}
	if c >= '0' && c <= '7' {
		start := l.pos - 1
		for l.pos-start < 3 && !l.eof() && l.src[l.pos] >= '0' && l.src[l.pos] <= '7' {
			l.pos++
		}
		v, _ := strconv.ParseUint(string(l.src[start:l.pos]), 8, 8)
		return string([]byte{byte(v)})
	}
	return string(c)
}

// copyBlock reads "COPY table (cols) FROM stdin;" and its data lines.
func (l *lexer) copyBlock(tables map[string][]dumpRow) error {
	table, err := l.tableName()
	if err != nil {
		return err
	}
	l.skipSpace()
	var cols []string
	if l.peek() == '(' {
		if cols, err = l.columns(); err != nil {
			return err
		}
	}
	fromStdin := l.keyword("FROM") && l.keyword("stdin")
	if err := l.skipStatement(); err != nil {
		return err
	}
	if !fromStdin {
		return nil
	}
	// Data starts on the next line and ends at a line holding only "\.".
	if nl := bytes.IndexByte(l.src[l.pos:], '\n'); nl >= 0 && len(bytes.TrimSpace(l.src[l.pos:l.pos+nl])) == 0 {
		l.pos += nl + 1
	}
	scanner := bufio.NewScanner(bytes.NewReader(l.src[l.pos:]))
	scanner.Buffer(make([]byte, 0, 64<<10), MaxDumpBytes)
	consumed := 0
	keep := dumpTables[table] && cols != nil
	for scanner.Scan() {
		line := scanner.Text()
		consumed += len(scanner.Bytes()) + 1
		line = strings.TrimSuffix(line, "\r")
		if line == `\.` {
			l.pos += consumed
			return nil
		}
		if !keep {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != len(cols) {
			return fmt.Errorf("%s COPY row has %d fields for %d columns", table, len(fields), len(cols))
		}
		row := dumpRow{}
		for i, col := range cols {
			if fields[i] == `\N` {
				row[col] = nil
				continue
			}
			text := copyField(fields[i])
			row[col] = &text
		}
		tables[table] = append(tables[table], row)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return errors.New("COPY data has no terminating line")
}

func copyField(field string) string {
	if !strings.Contains(field, `\`) {
		return field
	}
	l := &lexer{src: []byte(field)}
	var b strings.Builder
	for !l.eof() {
		c := l.src[l.pos]
		l.pos++
		if c == '\\' && !l.eof() {
			b.WriteString(unescape(l))
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// snapshotFromRows converts the kept rows with the same rules Load applies
// to a live database.
func snapshotFromRows(tables map[string][]dumpRow, loc *time.Location) (Snapshot, error) {
	var snap Snapshot
	for _, row := range tables["groups"] {
		g := Group{ID: row.int("id"), Name: row.text("name"), IsDefault: row.bool("is_default"), IsGuest: row.bool("is_guest"), CreatedAt: row.stamp("created_at", loc)}
		configs := row.text("configs")
		if configs == "" {
			configs = "{}"
		}
		if err := groupConfigs(&g, configs); err != nil {
			return Snapshot{}, err
		}
		snap.Groups = append(snap.Groups, g)
	}
	for _, row := range tables["users"] {
		snap.Users = append(snap.Users, User{ID: row.int("id"), GroupID: row.int("group_id"), Name: row.text("name"), Email: row.text("email"), PasswordHash: row.text("password"), IsAdmin: row.bool("is_adminer"), Enabled: row.int("status") == 1, RegisteredIP: row.text("registered_ip"), CreatedAt: row.stamp("created_at", loc)})
	}
	for _, row := range tables["personal_access_tokens"] {
		if !strings.HasSuffix(row.text("tokenable_type"), "User") {
			continue
		}
		t := Token{ID: row.int("id"), UserID: row.int("tokenable_id"), Name: row.text("name"), Hash: row.text("token"), CreatedAt: row.stamp("created_at", loc)}
		if at := row.stamp("last_used_at", loc); !at.IsZero() {
			t.LastUsedAt = &at
		}
		snap.Tokens = append(snap.Tokens, t)
	}
	for _, row := range tables["albums"] {
		snap.Albums = append(snap.Albums, Album{ID: row.int("id"), UserID: row.int("user_id"), Name: row.text("name"), Intro: row.text("intro"), CreatedAt: row.stamp("created_at", loc)})
	}
	for _, row := range tables["strategies"] {
		// Only the public URL leaves the configs; access keys are dropped.
		var configs struct {
			URL string `json:"url"`
		}
		_ = json.Unmarshal([]byte(row.text("configs")), &configs)
		snap.Strategies = append(snap.Strategies, Strategy{ID: row.int("id"), Key: int(row.int("key")), Name: row.text("name"), URL: strings.TrimRight(configs.URL, "/")})
	}
	for _, row := range tables["images"] {
		img := Image{ID: row.int("id"), UserID: row.int("user_id"), AlbumID: row.int("album_id"), StrategyID: row.int("strategy_id"), Extension: row.text("extension"), OriginName: row.text("origin_name"), MD5: row.text("md5"), IP: row.text("uploaded_ip")}
		finishImage(&img, row.text("path"), row.text("name"), int(row.int("permission")), row.stamp("created_at", loc))
		snap.Images = append(snap.Images, img)
	}
	sortSnapshot(&snap)
	return snap, nil
}

func sortSnapshot(snap *Snapshot) {
	slices.SortStableFunc(snap.Groups, func(a, b Group) int { return cmp.Compare(a.ID, b.ID) })
	slices.SortStableFunc(snap.Users, func(a, b User) int { return cmp.Compare(a.ID, b.ID) })
	slices.SortStableFunc(snap.Tokens, func(a, b Token) int { return cmp.Compare(a.ID, b.ID) })
	slices.SortStableFunc(snap.Albums, func(a, b Album) int { return cmp.Compare(a.ID, b.ID) })
	slices.SortStableFunc(snap.Strategies, func(a, b Strategy) int { return cmp.Compare(a.ID, b.ID) })
	slices.SortStableFunc(snap.Images, func(a, b Image) int { return cmp.Compare(a.ID, b.ID) })
}

func (r dumpRow) text(col string) string {
	if v := r[col]; v != nil {
		return *v
	}
	return ""
}

func (r dumpRow) int(col string) int64 {
	text := strings.TrimSpace(r.text(col))
	if n, err := strconv.ParseInt(text, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(text, 64); err == nil {
		return int64(f)
	}
	return 0
}

func (r dumpRow) bool(col string) bool {
	switch strings.ToLower(strings.TrimSpace(r.text(col))) {
	case "t", "true", "1", "y", "yes", "on":
		return true
	}
	return false
}

// stamp parses a zone-less timestamp in loc; offsets, when present, win.
// Year-1 placeholders come back zero, as from a live database.
func (r dumpRow) stamp(col string, loc *time.Location) time.Time {
	text := strings.TrimSpace(r.text(col))
	if text == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999-07", "2006-01-02T15:04:05.999999999Z07:00"} {
		if t, err := time.Parse(layout, text); err == nil {
			if t.Year() < 1971 {
				return time.Time{}
			}
			return t.UTC()
		}
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999999", "2006-01-02T15:04:05.999999999", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, text, loc); err == nil {
			if t.Year() < 1971 {
				return time.Time{}
			}
			return t.UTC()
		}
	}
	return time.Time{}
}
