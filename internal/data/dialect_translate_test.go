package data

import "testing"

func TestTranslateSQLiteDDLToPostgres(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			name: "autoincrement to bigserial",
			in:   "CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT)",
			want: "CREATE TABLE t (id BIGSERIAL PRIMARY KEY, name TEXT)",
		},
		{
			name: "blob to bytea",
			in:   "CREATE TABLE t (id INTEGER, data BLOB)",
			want: "CREATE TABLE t (id INTEGER, data BYTEA)",
		},
		{
			name: "strftime to to_char",
			in:   "CREATE TABLE t (id INTEGER, ts TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')))",
			want: "CREATE TABLE t (id INTEGER, ts TEXT DEFAULT (to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"')))",
		},
		{
			name: "datetime now to now",
			in:   "INSERT INTO t (ts) VALUES (datetime('now'))",
			want: "INSERT INTO t (ts) VALUES (now())",
		},
		{
			name: "date now to current_date",
			in:   "INSERT INTO t (d) VALUES (date('now'))",
			want: "INSERT INTO t (d) VALUES (current_date)",
		},
		{
			name: "time now to current_time",
			in:   "INSERT INTO t (tm) VALUES (time('now'))",
			want: "INSERT INTO t (tm) VALUES (current_time)",
		},
		{
			name: "case insensitive autoincrement",
			in:   "CREATE TABLE t (id integer primary key autoincrement)",
			want: "CREATE TABLE t (id BIGSERIAL PRIMARY KEY)",
		},
		{
			name: "no translation needed",
			in:   "CREATE INDEX idx_t_id ON t(id)",
			want: "CREATE INDEX idx_t_id ON t(id)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := translateSQLiteDDLToPostgres(c.in)
			if got != c.want {
				t.Errorf("translateSQLiteDDLToPostgres(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestTranslateSQLiteStatementToPostgres(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			name: "insert or ignore to on conflict do nothing",
			in:   "INSERT OR IGNORE INTO t (id, name) VALUES (1, 'a')",
			want: "INSERT INTO t (id, name) VALUES (1, 'a') ON CONFLICT DO NOTHING",
		},
		{
			name: "insert or ignore with leading comment",
			in:   "-- comment\nINSERT OR IGNORE INTO t (id) VALUES (1)",
			want: "INSERT INTO t (id) VALUES (1) ON CONFLICT DO NOTHING",
		},
		{
			name: "case insensitive insert or ignore",
			in:   "insert or ignore into t (id) values (1)",
			want: "INSERT INTO t (id) values (1) ON CONFLICT DO NOTHING",
		},
		{
			name: "regular insert unchanged",
			in:   "INSERT INTO t (id) VALUES (1)",
			want: "INSERT INTO t (id) VALUES (1)",
		},
		{
			name: "already has on conflict",
			in:   "INSERT OR IGNORE INTO t (id) VALUES (1) ON CONFLICT DO NOTHING",
			want: "INSERT INTO t (id) VALUES (1) ON CONFLICT DO NOTHING",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := translateSQLiteStatementToPostgres(c.in)
			if got != c.want {
				t.Errorf("translateSQLiteStatementToPostgres(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestStripLeadingSQLComments(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"SELECT 1", "SELECT 1"},
		{"-- comment\nSELECT 1", "SELECT 1"},
		{"-- comment\n-- another\nSELECT 1", "SELECT 1"},
		{"  -- comment\nSELECT 1", "SELECT 1"},
		{"-- comment", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := stripLeadingSQLComments(c.in)
		if got != c.want {
			t.Errorf("stripLeadingSQLComments(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestReplaceCaseInsensitive(t *testing.T) {
	cases := []struct {
		s, old, new, want string
	}{
		{"Hello World", "world", "Earth", "Hello Earth"},
		{"AAA", "a", "b", "bbb"},
		{"abc", "", "x", "abc"}, // empty old returns s unchanged
		{"", "a", "b", ""},
		{"no match", "xyz", "abc", "no match"},
	}
	for _, c := range cases {
		got := replaceCaseInsensitive(c.s, c.old, c.new)
		if got != c.want {
			t.Errorf("replaceCaseInsensitive(%q, %q, %q) = %q, want %q", c.s, c.old, c.new, got, c.want)
		}
	}
}
