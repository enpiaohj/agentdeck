package sessions

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"

	_ "github.com/enpiaohj/agentdeck/internal/sqlitedb"
)

type nativeIndex struct {
	Path         string
	Key          string
	Missing      bool
	Hash         string
	Lines        []string
	removed      bool
	expectedHash string
	afterHash    string
}
type nativeTable struct {
	Name    string
	Where   string
	Columns []string
	Inserts []string
}
type nativeDatabase struct {
	Path           string
	Schema         string
	Tables         []nativeTable
	removed        bool
	expectedTables []nativeTable
}

func (i *nativeIndex) expected() string {
	if i.expectedHash != "" {
		return i.expectedHash
	}
	return i.Hash
}
func (d *nativeDatabase) expected() []nativeTable {
	if d.expectedTables != nil {
		return d.expectedTables
	}
	return d.Tables
}

type nativeSQL interface {
	Query(string, ...any) (*sql.Rows, error)
}

var nativeTableColumns = map[string]string{
	"threads":                  "id rollout_path created_at updated_at source model_provider cwd title sandbox_policy approval_mode tokens_used has_user_event archived archived_at git_sha git_branch git_origin_url cli_version first_user_message agent_nickname agent_role memory_mode model reasoning_effort agent_path created_at_ms updated_at_ms thread_source preview recency_at recency_at_ms history_mode name is_pinned thread_section_id section_position section_entered_at_ms project_id originator daybreak_enabled creator_user_id creator_account_id",
	"thread_dynamic_tools":     "thread_id position name description input_schema defer_loading namespace",
	"thread_attachments":       "id thread_id attachment_type identity_key payload created_at",
	"guardian_review_feedback": "id thread_id record",
	"thread_read_receipts":     "thread_id first_unread_turn revision last_published_turn",
	"thread_spawn_edges":       "parent_thread_id child_thread_id status",
}
var nativeUnrelatedTables = map[string]bool{"_sqlx_migrations": true, "backfill_state": true, "remote_control_enrollments": true, "external_agent_config_imports": true, "thread_sections": true, "rollout_migration_state": true, "rollout_migration_skipped_rollouts": true, "projects": true, "project_roots": true, "project_idempotency_keys": true}

func nativeOpenDB(path string, write bool) (*sql.DB, error) {
	if err := nativeSafePath(path); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	if len(u.Path) >= 2 && u.Path[1] == ':' {
		u.Path = "/" + u.Path
	}
	q := u.Query()
	if write {
		q.Set("mode", "rw")
	} else {
		q.Set("mode", "ro")
	}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err == nil {
		db.SetMaxOpenConns(1)
		err = db.Ping()
	}
	if err != nil && db != nil {
		db.Close()
	}
	return db, err
}

func nativeQuoteID(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// Maintenance triggers write derived columns back onto their own row. They fire
// on INSERT or UPDATE only, so a DELETE of the targeted threads never runs them;
// accepting them is safe as long as they cannot write outside the known tables.
// Go's regexp has no backreferences, so the header is captured in groups and the
// event kind is inspected in nativeMaintenanceTarget.
var nativeTriggerHead = regexp.MustCompile(`(?is)^\s*CREATE\s+TRIGGER\s+(?:IF\s+NOT\s+EXISTS\s+)?"?(\w+)"?\s+(?:BEFORE|AFTER|INSTEAD\s+OF)\s+([A-Za-z]+(?:\s+OF\s+[A-Za-z_,\s]+?)?)\s+ON\s+"?(\w+)"?\s+WHEN\b(.*)$`)
var nativeTriggerWrite = regexp.MustCompile(`(?is)\b(?:INSERT\s+(?:OR\s+\w+\s+)?INTO|UPDATE(?:\s+OR\s+\w+)?|DELETE\s+FROM)\s+"?(\w+)"?(?:\s+SET\b)?`)

// Reads the mutation event out of a trigger header so the guard can tell a
// row-local maintenance trigger from one that reacts to DELETE. The body must be
// a single BEGIN … END block and must not mention DELETE anywhere.
func nativeMaintenanceTarget(ddl string) (event, table string, ok bool) {
	m := nativeTriggerHead.FindStringSubmatch(ddl)
	if m == nil {
		return "", "", false
	}
	fields := strings.Fields(strings.ToUpper(m[2]))
	if len(fields) == 0 || (fields[0] != "INSERT" && fields[0] != "UPDATE") {
		return "", "", false
	}
	upper := strings.ToUpper(ddl)
	if strings.Contains(upper, "DELETE") {
		return "", "", false
	}
	if strings.Count(upper, "BEGIN") != 1 || strings.Count(upper, "END") != 1 {
		return "", "", false
	}
	return fields[0], m[3], true
}

func nativeKnownTable(name string) bool {
	return nativeTableColumns[name] != "" || nativeUnrelatedTables[name]
}

// nativeTriggerWrites lists the tables a trigger body writes to, so a maintenance
// trigger cannot be used to smuggle rows into a table the guard never inspects.
// Only the BEGIN … END body is scanned: the header's `UPDATE OF col` is not a write.
func nativeTriggerWrites(ddl string) []string {
	upper := strings.ToUpper(ddl)
	if idx := strings.Index(upper, "BEGIN"); idx >= 0 {
		ddl = ddl[idx:]
	}
	var out []string
	for _, m := range nativeTriggerWrite.FindAllStringSubmatch(ddl, -1) {
		if name := m[1]; name != "" && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// The known schemas are explicit. New tables, columns, triggers or unknown
// references fail closed before anything is removed, including file sources.
func nativeDBSnapshot(q nativeSQL, path, id string) (*nativeDatabase, error) {
	rows, err := q.Query("SELECT type,name,tbl_name,coalesce(sql,'') FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY type,name")
	if err != nil {
		return nil, err
	}
	var schema []string
	var tables []string
	for rows.Next() {
		var kind, name, table, ddl string
		if err = rows.Scan(&kind, &name, &table, &ddl); err != nil {
			rows.Close()
			return nil, err
		}
		if kind == "trigger" {
			if _, table, ok := nativeMaintenanceTarget(ddl); !ok || !nativeKnownTable(table) {
				rows.Close()
				return nil, fmt.Errorf("Codex 状态库含未知 trigger: %s", name)
			}
			// A maintenance trigger may only write back to known tables; one that
			// reaches an unknown table could move rows the guard cannot verify.
			for _, write := range nativeTriggerWrites(ddl) {
				if !nativeKnownTable(write) {
					rows.Close()
					return nil, fmt.Errorf("Codex 状态库 trigger 写入未知表: %s → %s", name, write)
				}
			}
		}
		if kind == "table" {
			if nativeTableColumns[name] == "" && !nativeUnrelatedTables[name] {
				rows.Close()
				return nil, fmt.Errorf("Codex 状态库结构暂不支持: %s", name)
			}
			tables = append(tables, name)
		}
		schema = append(schema, kind+"|"+name+"|"+table+"|"+ddl)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	d := &nativeDatabase{Path: path, Schema: strings.Join(schema, "\n")}
	// Restore the parent thread before its children, and delete in reverse.
	sort.Slice(tables, func(i, j int) bool {
		if tables[i] == "threads" {
			return true
		}
		if tables[j] == "threads" {
			return false
		}
		return tables[i] < tables[j]
	})
	for _, name := range tables {
		// Even unrelated known tables must not acquire a new relationship that
		// would make the targeted deletion cascade outside the supported set.
		fk, err := q.Query("PRAGMA foreign_key_list(" + nativeQuoteID(name) + ")")
		if err != nil {
			return nil, err
		}
		for fk.Next() {
			var seq, part int
			var target, from, to, update, del, match string
			if err = fk.Scan(&seq, &part, &target, &from, &to, &update, &del, &match); err != nil {
				fk.Close()
				return nil, err
			}
			if _, sensitive := nativeTableColumns[target]; sensitive && (name == "threads" || nativeTableColumns[name] == "" || target != "threads" || from != "thread_id" || to != "id" || del != "CASCADE") {
				fk.Close()
				return nil, fmt.Errorf("Codex 状态库含未知关联: %s.%s", name, from)
			}
		}
		err = fk.Err()
		fk.Close()
		if err != nil {
			return nil, err
		}
		allowed := nativeTableColumns[name]
		if allowed == "" {
			continue
		}
		cols, err := q.Query("PRAGMA table_info(" + nativeQuoteID(name) + ")")
		if err != nil {
			return nil, err
		}
		t := nativeTable{Name: name}
		has := map[string]bool{}
		for cols.Next() {
			var cid, notnull, pk int
			var col, typ string
			var def any
			if err = cols.Scan(&cid, &col, &typ, &notnull, &def, &pk); err != nil {
				cols.Close()
				return nil, err
			}
			if !strings.Contains(" "+allowed+" ", " "+col+" ") {
				cols.Close()
				return nil, fmt.Errorf("Codex 状态库含未知列: %s.%s", name, col)
			}
			t.Columns = append(t.Columns, col)
			has[col] = true
		}
		err = cols.Err()
		cols.Close()
		if err != nil {
			return nil, err
		}
		switch name {
		case "threads":
			if !has["id"] || !has["rollout_path"] {
				return nil, errors.New("Codex threads 结构不支持")
			}
			t.Where = `"id" = ?`
		case "thread_spawn_edges":
			if !has["parent_thread_id"] || !has["child_thread_id"] {
				return nil, errors.New("Codex spawn edges 结构不支持")
			}
			t.Where = `"parent_thread_id" = ? OR "child_thread_id" = ?`
		default:
			if !has["thread_id"] {
				return nil, errors.New("Codex thread 关联结构不支持")
			}
			t.Where = `"thread_id" = ?`
		}
		var quoted, quotedValues []string
		for _, col := range t.Columns {
			quoted = append(quoted, nativeQuoteID(col))
			// quote(TEXT) truncates at embedded NUL. Hex-encode text bytes so
			// recovery preserves their exact value, including NUL and newlines.
			c := nativeQuoteID(col)
			quotedValues = append(quotedValues, "CASE typeof("+c+") WHEN 'text' THEN 'CAST(X''' || hex("+c+") || ''' AS TEXT)' ELSE quote("+c+") END")
		}
		args := []any{id}
		if name == "thread_spawn_edges" {
			args = append(args, id)
		}
		data, err := q.Query("SELECT "+strings.Join(quotedValues, ",")+" FROM "+nativeQuoteID(name)+" WHERE "+t.Where, args...)
		if err != nil {
			return nil, err
		}
		for data.Next() {
			values := make([]string, len(t.Columns))
			dest := make([]any, len(values))
			for i := range dest {
				dest[i] = &values[i]
			}
			if err = data.Scan(dest...); err != nil {
				data.Close()
				return nil, err
			}
			t.Inserts = append(t.Inserts, "INSERT INTO "+nativeQuoteID(name)+" ("+strings.Join(quoted, ",")+") VALUES ("+strings.Join(values, ",")+");")
		}
		err = data.Err()
		data.Close()
		if err != nil {
			return nil, err
		}
		sort.Strings(t.Inserts)
		d.Tables = append(d.Tables, t)
	}
	return d, nil
}

func (p *NativeDeletePlan) prepareCodexMetadata() error {
	paths, err := filepath.Glob(filepath.Join(CodexDir(), "state_*.sqlite"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		if filepath.Base(path) != "state_5.sqlite" {
			return fmt.Errorf("Codex 状态库版本暂不支持: %s", filepath.Base(path))
		}
		db, err := nativeOpenDB(path, false)
		if err != nil {
			return err
		}
		d, err := nativeDBSnapshot(db, path, p.session.ID)
		db.Close()
		if err != nil {
			return err
		}
		p.databases = append(p.databases, d)
		p.Files = append(p.Files, path)
	}
	path := filepath.Join(CodexDir(), "session_index.jsonl")
	if _, err = os.Lstat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	idx, err := nativeReadIndex(path, p.session.ID)
	if err != nil {
		return err
	}
	p.index = idx
	if len(idx.Lines) > 0 {
		p.Files = append(p.Files, path)
	}
	return nil
}

func nativeReadIndex(path, id string) (*nativeIndex, error) { return nativeReadJSONL(path, id, "id") }

func nativeMatchesJSONL(line, id, key string) bool {
	var row map[string]json.RawMessage
	if json.Unmarshal([]byte(line), &row) != nil {
		return false
	}
	for _, k := range strings.Split(key, "|") {
		var value string
		if json.Unmarshal(row[k], &value) == nil && value == id {
			return true
		}
	}
	return false
}

func (p *NativeDeletePlan) prepareHistory() error {
	homes := []string{CodexDir()}
	if p.session.Agent != "codex" {
		homes = []string{ClaudeDir()}
		if p.session.Agent == "claude-desktop" {
			for _, d := range DesktopDataDirs() {
				for _, kind := range []string{"local-agent-mode-sessions", "claude-code-sessions"} {
					hs, _ := filepath.Glob(filepath.Join(d, kind, "*", "*", "local_*", ".claude"))
					homes = append(homes, hs...)
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, home := range homes {
		path := filepath.Join(home, "history.jsonl")
		if seen[path] {
			continue
		}
		seen[path] = true
		idx, err := nativeReadJSONL(path, p.session.ID, "session_id|sessionId")
		if os.IsNotExist(err) {
			p.histories = append(p.histories, &nativeIndex{Path: path, Key: "session_id|sessionId", Missing: true})
			continue
		}
		if err != nil {
			return err
		}
		p.histories = append(p.histories, idx)
		if len(idx.Lines) > 0 {
			p.Files = append(p.Files, path)
		}
	}
	return nil
}

func nativeReadJSONL(path, id, key string) (*nativeIndex, error) {
	if err := nativeSafePath(path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	r := bufio.NewReader(io.TeeReader(f, h))
	idx := &nativeIndex{Path: path, Key: key}
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			if strings.TrimSpace(line) != "" && !json.Valid([]byte(line)) {
				return nil, fmt.Errorf("原始共享索引含不完整或无效 JSON，拒绝删除: %s", path)
			}
			if nativeMatchesJSONL(line, id, key) {
				idx.Lines = append(idx.Lines, line)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	idx.Hash = hex.EncodeToString(h.Sum(nil))
	return idx, nil
}

func (p *NativeDeletePlan) validateMetadata() error {
	for _, idx := range p.histories {
		now, err := nativeReadJSONL(idx.Path, p.session.ID, idx.Key)
		if idx.Missing && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if idx.Missing || idx.expected() != now.Hash {
			return errors.New("原始 history 索引发生变化，请重新预览")
		}
	}
	if p.index != nil {
		idx, err := nativeReadIndex(p.index.Path, p.session.ID)
		if err != nil {
			return err
		}
		if idx.Hash != p.index.expected() {
			return errors.New("Codex 标题索引发生变化，请重新预览")
		}
	}
	if p.session.Agent == "codex" {
		paths, err := filepath.Glob(filepath.Join(CodexDir(), "state_*.sqlite"))
		if err != nil {
			return err
		}
		if len(paths) != len(p.databases) {
			return errors.New("Codex 状态库范围发生变化，请重新预览")
		}
		if p.index == nil {
			if _, err := os.Lstat(filepath.Join(CodexDir(), "session_index.jsonl")); !os.IsNotExist(err) {
				return errors.New("Codex 标题索引范围发生变化，请重新预览")
			}
		}
	}
	for _, old := range p.databases {
		db, err := nativeOpenDB(old.Path, false)
		if err != nil {
			return err
		}
		now, err := nativeDBSnapshot(db, old.Path, p.session.ID)
		db.Close()
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(old.expected(), now.Tables) || old.Schema != now.Schema {
			return errors.New("Codex 会话元数据发生变化，请重新预览")
		}
	}
	return nil
}

func (p *NativeDeletePlan) deleteMetadata() error {
	for _, old := range p.databases {
		db, err := nativeOpenDB(old.Path, true)
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			db.Close()
			return err
		}
		now, err := nativeDBSnapshot(tx, old.Path, p.session.ID)
		if err == nil && (old.Schema != now.Schema || !reflect.DeepEqual(old.expected(), now.Tables)) {
			err = errors.New("Codex 会话元数据发生变化")
		}
		if err == nil {
			for i := len(old.Tables) - 1; i >= 0; i-- {
				t := old.Tables[i]
				args := []any{p.session.ID}
				if t.Name == "thread_spawn_edges" {
					args = append(args, p.session.ID)
				}
				if _, err = tx.Exec("DELETE FROM "+nativeQuoteID(t.Name)+" WHERE "+t.Where, args...); err != nil {
					break
				}
			}
		}
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
		db.Close()
		if err != nil {
			return err
		}
		old.removed = true
	}
	if p.index != nil && len(p.index.Lines) > 0 {
		if err := p.rewriteIndex(p.index, false); err != nil {
			return err
		}
		p.index.removed = true
	}
	for _, idx := range p.histories {
		if len(idx.Lines) == 0 {
			continue
		}
		if err := p.rewriteIndex(idx, false); err != nil {
			return err
		}
		idx.removed = true
	}
	return nil
}

func (p *NativeDeletePlan) restoreMetadata() error {
	var errs []error
	for _, old := range p.databases {
		if !old.removed {
			continue
		}
		db, err := nativeOpenDB(old.Path, true)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			db.Close()
			errs = append(errs, err)
			continue
		}
		now, err := nativeDBSnapshot(tx, old.Path, p.session.ID)
		if err == nil && now.Schema != old.Schema {
			err = errors.New("Codex 状态库结构已变化，恢复中止")
		}
		existing := map[string]bool{}
		allowed := map[string]bool{}
		for _, t := range old.Tables {
			for _, insert := range t.Inserts {
				allowed[insert] = true
			}
		}
		if err == nil {
			for _, t := range now.Tables {
				for _, insert := range t.Inserts {
					if !allowed[insert] {
						err = errors.New("Codex 会话元数据已重建，恢复冲突")
						break
					}
					existing[insert] = true
				}
				if err != nil {
					break
				}
			}
		}
		if err == nil {
			for _, t := range old.Tables {
				for _, insert := range t.Inserts {
					if existing[insert] {
						continue
					}
					if _, err = tx.Exec(insert); err != nil {
						break
					}
				}
				if err != nil {
					break
				}
			}
		}
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
		db.Close()
		if err != nil {
			errs = append(errs, err)
		} else {
			old.removed = false
		}
	}
	if p.index != nil && p.index.removed {
		if err := p.rewriteIndex(p.index, true); err != nil {
			errs = append(errs, err)
		} else {
			p.index.removed = false
		}
	}
	for _, idx := range p.histories {
		if !idx.removed {
			continue
		}
		if err := p.rewriteIndex(idx, true); err != nil {
			errs = append(errs, err)
		} else {
			idx.removed = false
		}
	}
	return errors.Join(errs...)
}

// Rewrite uses a same-directory temporary file and compares the entire input
// again before replacing it. Undo merges only the selected ID's saved lines.
func (p *NativeDeletePlan) rewriteIndex(idx *nativeIndex, restore bool) error {
	now, err := nativeReadJSONL(idx.Path, p.session.ID, idx.Key)
	if err != nil {
		return err
	}
	if !restore && now.Hash != idx.expected() {
		return errors.New("Codex 标题索引已变化")
	}
	if restore && len(now.Lines) > 0 {
		if reflect.DeepEqual(now.Lines, idx.Lines) {
			return nil
		}
		return errors.New("Codex 标题索引已重建，恢复冲突")
	}
	src, err := os.Open(idx.Path)
	if err != nil {
		return err
	}
	defer src.Close()
	st, err := src.Stat()
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(idx.Path), ".agentdeck-index-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	defer tmp.Close()
	if err = tmp.Chmod(st.Mode().Perm()); err != nil {
		return err
	}
	r := bufio.NewReader(src)
	h := sha256.New()
	var last byte
	for {
		line, e := r.ReadString('\n')
		if line != "" {
			h.Write([]byte(line))
			last = line[len(line)-1]
			keep := true
			if !restore {
				if nativeMatchesJSONL(line, p.session.ID, idx.Key) {
					keep = false
				}
			}
			if keep {
				if _, err = io.WriteString(tmp, line); err != nil {
					return err
				}
			}
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
	}
	if hex.EncodeToString(h.Sum(nil)) != now.Hash {
		return errors.New("Codex 标题索引读取期间变化")
	}
	if restore {
		if st.Size() > 0 && last != '\n' {
			if _, err = tmp.Write([]byte{'\n'}); err != nil {
				return err
			}
		}
		for _, line := range idx.Lines {
			if _, err = io.WriteString(tmp, line); err != nil {
				return err
			}
		}
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	afterHash, _, err := nativeHash(name)
	if err != nil {
		return err
	}
	check, _, err := nativeHash(idx.Path)
	if err != nil {
		return err
	}
	if check != now.Hash {
		return errors.New("Codex 标题索引写入期间变化")
	}
	src.Close()
	if err = os.Rename(name, idx.Path); err != nil {
		return err
	}
	if !restore {
		idx.afterHash = afterHash
	}
	return nil
}
