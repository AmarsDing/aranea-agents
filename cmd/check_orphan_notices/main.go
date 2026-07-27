package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

type row struct {
	ID         string
	TurnID     string
	TaskID     string
	SessionID  string
	SpiritID   string
	Kind       string
	AuthorKey  string
	Seq        int64
	Content    string
	NoticeType string
	Status     string
	StartedAt  string
	Version    int64
}

func main() {
	sessionID := "d78029b9-c305-4bc1-9583-ac9f743cdc60"
	if len(os.Args) > 1 {
		sessionID = os.Args[1]
	}
	outFile, err := os.Create(`f:\aranea-agents\logs\_orphan_notices.txt`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create out:", err)
		os.Exit(1)
	}
	defer outFile.Close()
	printf := func(format string, args ...any) {
		fmt.Fprintf(outFile, format, args...)
	}

	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		fmt.Fprintln(os.Stderr, "ping:", err)
		os.Exit(1)
	}

	// 0. Kind distribution.
	printf("=== steps_v2 kind distribution (session_id=$1 OR spirit_session_id=$1) ===\n")
	kr, err := db.Query(`
SELECT kind, count(*) AS n,
       count(*) FILTER (WHERE turn_id = '') AS orphan_turn,
       count(*) FILTER (WHERE task_id = '') AS orphan_task
FROM steps_v2
WHERE session_id = $1 OR spirit_session_id = $1
GROUP BY kind ORDER BY n DESC`, sessionID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query kinds:", err)
		os.Exit(1)
	}
	for kr.Next() {
		var kind string
		var n, orphanTurn, orphanTask int
		if err := kr.Scan(&kind, &n, &orphanTurn, &orphanTask); err != nil {
			fmt.Fprintln(os.Stderr, "scan:", err)
			os.Exit(1)
		}
		printf("  kind=%-10s total=%-5d orphan_turn=%-5d orphan_task=%-5d\n", kind, n, orphanTurn, orphanTask)
	}
	kr.Close()

	// 1. Session-level orphan notices: kind='notice' AND turn_id=''.
	printf("\n=== session-level orphan notices (kind='notice' AND turn_id='') ===\n")
	rows, err := db.Query(`
SELECT id, turn_id, task_id, session_id, spirit_session_id, kind,
       author_agent_key, seq, left(content, 200), notice_type, status,
       to_char(started_at, 'YYYY-MM-DD HH24:MI:SS.MS'), version
FROM steps_v2
WHERE (session_id = $1 OR spirit_session_id = $1)
  AND kind = 'notice' AND turn_id = ''
ORDER BY started_at, id`, sessionID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query notices:", err)
		os.Exit(1)
	}
	var notices []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.TurnID, &r.TaskID, &r.SessionID, &r.SpiritID,
			&r.Kind, &r.AuthorKey, &r.Seq, &r.Content, &r.NoticeType, &r.Status,
			&r.StartedAt, &r.Version); err != nil {
			fmt.Fprintln(os.Stderr, "scan:", err)
			os.Exit(1)
		}
		notices = append(notices, r)
	}
	rows.Close()
	printf("count=%d\n", len(notices))
	for _, r := range notices {
		printf("id=%s seq=%d ver=%d status=%s notice_type=%q\n  ts=%s task=%q author=%q\n  content=%q\n",
			r.ID, r.Seq, r.Version, r.Status, r.NoticeType, r.StartedAt, r.TaskID, r.AuthorKey, r.Content)
	}

	// 2. Duplicate groups by notice_type + content.
	printf("\n=== duplicate groups (notice_type + content) ===\n")
	type key struct{ nt, content string }
	groups := map[key][]row{}
	order := []key{}
	for _, r := range notices {
		k := key{r.NoticeType, r.Content}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}
	for _, k := range order {
		g := groups[k]
		if len(g) < 2 {
			continue
		}
		printf("x%d notice_type=%q content=%q\n", len(g), k.nt, k.content)
		for _, r := range g {
			printf("    id=%s ts=%s seq=%d ver=%d status=%s task=%q\n",
				r.ID, r.StartedAt, r.Seq, r.Version, r.Status, r.TaskID)
		}
	}

	// 2.5 tasks + turns structure for this session.
	printf("\n=== tasks_v2 for session ===\n")
	tr, err := db.Query(`
SELECT id, left(coalesce(user_message,''), 40), status, seq, to_char(created_at, 'HH24:MI:SS.MS')
FROM tasks_v2 WHERE session_id = $1 ORDER BY created_at`, sessionID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query tasks:", err)
		os.Exit(1)
	}
	for tr.Next() {
		var id, title, status, ts string
		var seq int64
		if err := tr.Scan(&id, &title, &status, &seq, &ts); err != nil {
			fmt.Fprintln(os.Stderr, "scan:", err)
			os.Exit(1)
		}
		printf("  task=%s seq=%d status=%s created=%s title=%q\n", id, seq, status, ts, title)
	}
	tr.Close()

	printf("\n=== turns_v2 for session ===\n")
	ur, err := db.Query(`
SELECT id, task_id, status, agent_key, to_char(started_at, 'HH24:MI:SS.MS')
FROM turns_v2 WHERE session_id = $1 ORDER BY started_at`, sessionID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query turns:", err)
		os.Exit(1)
	}
	for ur.Next() {
		var id, taskID, status, agentKey, ts string
		if err := ur.Scan(&id, &taskID, &status, &agentKey, &ts); err != nil {
			fmt.Fprintln(os.Stderr, "scan:", err)
			os.Exit(1)
		}
		printf("  turn=%s task=%s status=%s agent=%s started=%s\n", id, taskID, status, agentKey, ts)
	}
	ur.Close()

	// 2.8 first steps of each turn (to identify synthesis turns by their input).
	printf("\n=== first reply/action step per turn (input identification) ===\n")
	fs, err := db.Query(`
SELECT s.turn_id, s.kind, left(coalesce(s.content,''), 100), s.seq
FROM steps_v2 s
WHERE (s.session_id = $1 OR s.spirit_session_id = $1)
  AND s.kind = 'reply'
  AND (s.turn_id, s.seq) IN (
    SELECT turn_id, max(seq) FROM steps_v2
    WHERE (session_id = $1 OR spirit_session_id = $1) AND kind = 'reply'
    GROUP BY turn_id)
ORDER BY s.turn_id`, sessionID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query first steps:", err)
		os.Exit(1)
	}
	for fs.Next() {
		var turnID, kind, content string
		var seq int64
		if err := fs.Scan(&turnID, &kind, &content, &seq); err != nil {
			fmt.Fprintln(os.Stderr, "scan:", err)
			os.Exit(1)
		}
		printf("  turn=%s seq=%d kind=%s content=%q\n", turnID, seq, kind, content)
	}
	fs.Close()

	// 3. Same notice_type + content but with different task_id / across tasks.
	printf("\n=== ALL notice steps (any turn) with notice_type — compact ===\n")
	all, err := db.Query(`
SELECT id, turn_id, task_id, seq, left(content, 80), notice_type, status,
       to_char(started_at, 'HH24:MI:SS.MS'), version
FROM steps_v2
WHERE (session_id = $1 OR spirit_session_id = $1) AND kind = 'notice'
ORDER BY started_at, id`, sessionID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query all notices:", err)
		os.Exit(1)
	}
	defer all.Close()
	for all.Next() {
		var id, turnID, taskID, content, nt, status, ts string
		var seq, ver int64
		if err := all.Scan(&id, &turnID, &taskID, &seq, &content, &nt, &status, &ts, &ver); err != nil {
			fmt.Fprintln(os.Stderr, "scan:", err)
			os.Exit(1)
		}
		printf("  ts=%s id=%s turn=%q task=%q seq=%d ver=%d nt=%q status=%s content=%q\n",
			ts, id, turnID, taskID, seq, ver, nt, status, content)
	}
}
