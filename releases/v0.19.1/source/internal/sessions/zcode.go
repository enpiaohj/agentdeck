package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ZCode's CLI, OpenCode made over, keeps its sessions in OpenCode's tables
// in a database of its own, ~/.zcode/cli/db/db.sqlite: a session row with
// how its title came about (title_source), and its messages and parts as
// JSON documents. A message counts as OpenCode's does but for its tokens,
// the AI SDK's: input with the cache in it, output with the reasoning. Its
// model is modelId (the older ones say modelID too), and a user message
// says who it is from (semantics.origin), a prompt typed a real_user's. A
// subagent's work is a session whose parent is the one it ran in, as in
// OpenCode. ZCode is a desktop app: no command picks a session up again.

// ZCodeDir is ZCode's folder, ~/.zcode.
func ZCodeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zcode")
}

func zcodeDB() string { return filepath.Join(ZCodeDir(), "cli", "db", "db.sqlite") }

func zcodeFiles() []file {
	if db := zcodeDB(); fileExists(db) {
		return openCodeDBFiles("zcode", db)
	}
	return nil
}

// zcDB is ZCode's database: OpenCode's, with a session's title its own only
// when it was not the default one.
type zcDB struct{ ocDB }

func (s zcDB) info(sid string) (ocInfo, bool) {
	var i ocInfo
	var source string
	err := s.db.QueryRow(`SELECT id, COALESCE(parent_id, ''), directory, title, COALESCE(title_source, ''), time_created, time_updated FROM session WHERE id = ?`, sid).
		Scan(&i.ID, &i.ParentID, &i.Directory, &i.Title, &source, &i.Time.Created, &i.Time.Updated)
	if source == "default" {
		i.Title = ""
	}
	return i, err == nil
}

// messages and parts are ocDB's with the row's own id written into it: ZCode
// keeps a message's id in the column of that name, not in the document as
// OpenCode does, and a message's parts are asked for by that id, so a
// document read without it (ocDB.messages) would leave every part unread.
func (s zcDB) messages(sid string) [][]byte {
	rows, err := s.db.Query(`SELECT id, data FROM message WHERE session_id = ? ORDER BY id`, sid)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var id string
		var b []byte
		if rows.Scan(&id, &b) != nil {
			continue
		}
		if doc := docWithID(id, b); doc != nil {
			out = append(out, doc)
		}
	}
	return out
}

func (s zcDB) parts(mid string) [][]byte {
	return s.rows(`SELECT data FROM part WHERE message_id = ? ORDER BY id`, mid)
}

// docWithID is a document of an object with its id said first, "" if the id
// cannot be written into it.
func docWithID(id string, doc []byte) []byte {
	if len(doc) == 0 || doc[0] != '{' {
		return nil
	}
	quoted, err := json.Marshal(id)
	if err != nil {
		return nil
	}
	out := make([]byte, 0, len(doc)+len(quoted)+6)
	out = append(out, `{"id":`...)
	out = append(out, quoted...)
	out = append(out, ',')
	out = append(out, doc[1:]...)
	return out
}
