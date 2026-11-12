// Package chatgpthistory imports ChatGPT's official account export
// (Settings → Data controls → Export data) into an AgentDeck-owned store.
// The export ZIP or JSON is only read; nothing is written to it, and no login
// credential is ever involved.
//
// The export holds each conversation as a message tree (`mapping`); the
// conversation the user saw is the path from `current_node` up to the root.
// Conversations inside a ChatGPT Project carry the project's id in
// `conversation_template_id` (`g-p-...`); the export does NOT include the
// project's name, so names are supplied by the user and kept beside the
// imported batches.
package chatgpthistory

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// Agent is the agent key sessions imported from ChatGPT carry.
const Agent = "chatgpt"

// maxShard bounds one conversations-NNN.json (the export shards them; the
// largest seen is about 22 MiB). It is a sanity limit, not the export's size:
// the attachments in the ZIP, which can be GiB, are never read.
const maxShard = 512 << 20

const (
	maxMembers      = 200_000 // a real export has some thousands
	maxTitleRunes   = 300
	maxAccountRunes = 80
	maxProjectRunes = 60
	projectsFile    = "projects.json"
	assetsDirName   = "assets"
)

type Report struct {
	Conversations   int    `json:"conversations"`
	Projects        int    `json:"projects"`
	Account         string `json:"account"`
	AlreadyImported bool   `json:"already_imported"`
	SkippedBlocks   int    `json:"skipped_blocks"`
	Images          int    `json:"images"` // distinct images copied into the store
	// Reclaimed is how many older batches this import made redundant (every
	// conversation in them has a newer or richer copy elsewhere) and removed.
	Reclaimed      int   `json:"reclaimed"`
	ReclaimedBytes int64 `json:"reclaimed_bytes"`
}

// Options are what the user chose for one import.
type Options struct {
	// Names maps project ids to the names the user chose (missing = keep what is
	// already stored, else a default).
	Names map[string]string
	// Images also copies the images the conversations show (and the ones
	// ChatGPT drew) into the store, so they can be read in place.
	Images bool
	// FreeSpace reports the free bytes of the volume holding dir. When set, an
	// import that copies images first checks there is room for them.
	FreeSpace func(dir string) (int64, error)
}

// ProjectInfo is a ChatGPT Project found in an export, for the user to name.
type ProjectInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`    // already chosen for this account, else ""
	Default string `json:"default"` // what it is called until named
	// Suggested is the name the ChatGPT desktop app on this computer gives it
	// (LocalProjectNames); the export itself carries no project names.
	Suggested string `json:"suggested"`
	// Files are names of files uploaded to the project (from the export's
	// library_files.json): another way to recognise it, needing nothing but the export.
	Files         []string `json:"files"`
	Conversations int      `json:"conversations"`
	Samples       []string `json:"samples"` // titles of the latest conversations, to recognise it by
	// Recommended is a name worked out from the project's conversation titles and
	// uploaded files (recommendName), RecommendedBasis says from which. A guess to
	// be confirmed; "" when there is nothing to go on.
	Recommended      string `json:"recommended"`
	RecommendedBasis string `json:"recommended_basis"`
}

// maxTitlesPerProject bounds the titles kept per project to work a name out of.
const maxTitlesPerProject = 500

// PreviewResult is what an export holds, before anything is imported.
// ExistingAccount is an account already imported whose conversations overlap the export.
type ExistingAccount struct {
	Name    string `json:"name"`
	Overlap int    `json:"overlap"` // conversations of the export already imported under it
	// Missing counts conversations imported under it that the export no longer holds
	// (deleted in ChatGPT since, or from before the export's window).
	Missing int `json:"missing"`
}

type PreviewResult struct {
	// Accounts lists the already-imported accounts that share conversations with the
	// export, most shared first. A second account name for the same ChatGPT account
	// would store every conversation twice, so the import page points it out.
	Accounts      []ExistingAccount `json:"accounts"`
	Conversations int               `json:"conversations"`
	Projects      []ProjectInfo     `json:"projects"`
	Images        int               `json:"images"`      // distinct images the conversations use that the export carries
	ImageBytes    int64             `json:"image_bytes"` // what copying them would take
}

type entry struct {
	ID, Title        string
	Created, Updated time.Time
	Messages         int
	Project          string // `g-p-...`, "" for a conversation outside any project
	Model            string
}

type manifest struct {
	Version       int
	Account       string
	ImportedAt    time.Time
	SkippedBlocks int
	Images        int
	WithImages    bool
	Entries       []entry
}

// ---- the export's shape ----

type conversation struct {
	ID          string          `json:"id"`
	Alt         string          `json:"conversation_id"`
	Title       string          `json:"title"`
	Created     float64         `json:"create_time"`
	Updated     float64         `json:"update_time"`
	CurrentNode string          `json:"current_node"`
	Template    *string         `json:"conversation_template_id"`
	Model       string          `json:"default_model_slug"`
	Mapping     map[string]node `json:"mapping"`
}

func (c conversation) id() string {
	if c.ID != "" {
		return c.ID
	}
	return c.Alt
}

type node struct {
	ID       string   `json:"id"`
	Parent   *string  `json:"parent"`
	Children []string `json:"children"`
	Message  *message `json:"message"`
}

type message struct {
	Author struct {
		Role string `json:"role"`
	} `json:"author"`
	Created *float64 `json:"create_time"`
	Content struct {
		Type     string            `json:"content_type"`
		Parts    []json.RawMessage `json:"parts"`
		Thoughts []struct {
			Summary string `json:"summary"`
			Content string `json:"content"`
		} `json:"thoughts"`
	} `json:"content"`
	Metadata struct {
		Hidden      bool   `json:"is_visually_hidden_from_conversation"`
		Model       string `json:"model_slug"`
		Attachments []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Mime string `json:"mime_type"`
			Size int64  `json:"size"`
		} `json:"attachments"`
	} `json:"metadata"`
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func seconds(v float64) time.Time {
	// 4e9 s is the year 2096: anything beyond is a corrupt value, not a date
	if v <= 0 || v > 4e9 {
		return time.Time{}
	}
	return time.Unix(int64(v), int64((v-float64(int64(v)))*1e9)).UTC()
}

// ---- reading the export ----

type shard struct {
	name string
	size uint64
	open func() (io.ReadCloser, error)
}

// exportSource is an opened export: its conversation shards, and (for a ZIP)
// the attachment files it carries, which are only opened when asked for.
type exportSource struct {
	shards []shard
	assets map[string]*zip.File // member name without ".dat" → file
	// library is library_files.json: the files the user uploaded, among them the
	// ones that belong to a Project (the only place the export names a project's files).
	library *zip.File
	close   func()
}

var shardName = regexp.MustCompile(`^conversations(-\d+)?\.json$`)

// openSource lists the conversation shards of a ChatGPT export: a ZIP holding
// conversations-000.json, conversations-001.json… (or a single
// conversations.json), or one such JSON file by itself.
func openSource(source string) (*exportSource, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("请选择 ChatGPT 导出的 ZIP 文件或 conversations JSON 文件")
	}
	switch strings.ToLower(filepath.Ext(source)) {
	case ".json":
		if info.Size() > maxShard {
			return nil, errors.New("文件超过 512 MiB 的上限，无法导入")
		}
		return &exportSource{shards: []shard{{name: filepath.Base(source), size: uint64(info.Size()), open: func() (io.ReadCloser, error) { return os.Open(source) }}}, close: func() {}}, nil
	case ".zip":
		zr, err := zip.OpenReader(source)
		if err != nil {
			return nil, errors.New("无法读取此 ZIP 文件，它可能已损坏或下载不完整")
		}
		if len(zr.File) > maxMembers {
			zr.Close()
			return nil, fmt.Errorf("ZIP 文件中的文件数量（%d）异常，这不像是 ChatGPT 的数据导出", len(zr.File))
		}
		var shards []shard
		assets := map[string]*zip.File{}
		var library *zip.File
		for _, f := range zr.File {
			base := path.Base(strings.ReplaceAll(f.Name, "\\", "/"))
			if !f.FileInfo().IsDir() && strings.HasSuffix(base, ".dat") && !strings.Contains(strings.TrimSuffix(f.Name, base), "/") {
				assets[strings.TrimSuffix(base, ".dat")] = f
				continue
			}
			if base == "library_files.json" && f.Name == base && f.UncompressedSize64 <= 64<<20 {
				library = f
				continue
			}
			if f.FileInfo().IsDir() || !shardName.MatchString(base) {
				continue
			}
			if f.UncompressedSize64 > maxShard {
				zr.Close()
				return nil, fmt.Errorf("%s 解压后超过 512 MiB 的上限，无法导入", f.Name)
			}
			f := f
			shards = append(shards, shard{name: f.Name, size: f.UncompressedSize64, open: f.Open})
		}
		if len(shards) == 0 {
			zr.Close()
			return nil, errors.New("在 ZIP 文件中找不到 conversations-*.json。请确认选择的是 ChatGPT 导出的数据包")
		}
		sort.Slice(shards, func(i, j int) bool { return shards[i].name < shards[j].name })
		return &exportSource{shards: shards, assets: assets, library: library, close: func() { zr.Close() }}, nil
	}
	return nil, errors.New("仅支持 ChatGPT 导出的 ZIP 文件或 JSON 文件")
}

// countImages counts the images the active branch of c shows that the export
// carries and have not been counted yet (seen is keyed by export member), and
// their size.
func (e *exportSource) countImages(c conversation, seen map[string]bool) (n int, size int64) {
	if len(e.assets) == 0 {
		return 0, 0
	}
	for _, node := range activePath(c) {
		m := node.Message
		if m == nil || m.Metadata.Hidden || (m.Author.Role != "user" && m.Author.Role != "assistant") {
			continue
		}
		for _, raw := range m.Content.Parts {
			var part struct {
				Type    string `json:"content_type"`
				Pointer string `json:"asset_pointer"`
			}
			if json.Unmarshal(raw, &part) != nil || part.Type != "image_asset_pointer" {
				continue
			}
			if f := e.asset(part.Pointer); f != nil && f.UncompressedSize64 <= maxImage && !seen[f.Name] {
				seen[f.Name] = true
				n++
				size += int64(f.UncompressedSize64)
			}
		}
	}
	return n, size
}

// projectFiles lists, per project id, the names of up to three files uploaded
// to it (newest first), from library_files.json. Deleted files are left out.
func (e *exportSource) projectFiles() map[string][]string {
	out := map[string][]string{}
	if e.library == nil {
		return out
	}
	rc, err := e.library.Open()
	if err != nil {
		return out
	}
	defer rc.Close()
	var entries []struct {
		Name      string  `json:"file_name"`
		Project   string  `json:"gizmo_id"`
		Created   string  `json:"created_at"`
		DeletedAt *string `json:"deleted_at"`
		Visible   *bool   `json:"is_visible"`
	}
	if json.NewDecoder(io.LimitReader(rc, 64<<20)).Decode(&entries) != nil {
		return out
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Created > entries[j].Created })
	seen := map[string]bool{}
	for _, f := range entries {
		name := truncateRunes(oneLine(f.Name), 60)
		key := f.Project + "\x00" + name
		if !strings.HasPrefix(f.Project, "g-p-") || name == "" || f.DeletedAt != nil || (f.Visible != nil && !*f.Visible) || seen[key] || len(out[f.Project]) >= 3 {
			continue
		}
		seen[key] = true
		out[f.Project] = append(out[f.Project], name)
	}
	return out
}

// asset finds the file an asset pointer ("file-service://file-XXXX" or
// "sediment://file_XXXX") names, or nil when the export does not carry it.
func (e *exportSource) asset(pointer string) *zip.File {
	_, ident, ok := strings.Cut(pointer, "://")
	if !ok {
		return nil
	}
	ident, _, _ = strings.Cut(ident, "#")
	for _, id := range []string{ident, strings.Replace(ident, "file-", "file_", 1), strings.Replace(ident, "file_", "file-", 1)} {
		if f := e.assets[id]; f != nil {
			return f
		}
	}
	return nil
}

// eachConversation streams the conversations of every shard, one at a time.
func eachConversation(shards []shard, fn func(conversation) error) error {
	for _, sh := range shards {
		rc, err := sh.open()
		if err != nil {
			return err
		}
		err = func() error {
			defer rc.Close()
			dec := json.NewDecoder(io.LimitReader(rc, maxShard+1))
			if tok, err := dec.Token(); err != nil || tok != json.Delim('[') {
				return fmt.Errorf("%s 的格式不是 ChatGPT 的会话数据，未导入任何内容", sh.name)
			}
			for dec.More() {
				var c conversation
				if err := dec.Decode(&c); err != nil {
					return fmt.Errorf("无法解析 %s，未导入任何内容。文件可能已损坏", sh.name)
				}
				if err := fn(c); err != nil {
					return err
				}
			}
			if tok, err := dec.Token(); err != nil || tok != json.Delim(']') {
				return fmt.Errorf("%s 不完整，未导入任何内容", sh.name)
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

// batchHash identifies the export by its conversation bytes alone, so the
// same export is recognised whatever its attachments or file name.
func batchHash(shards []shard) (string, error) {
	h := sha256.New()
	for _, sh := range shards {
		rc, err := sh.open()
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00", path.Base(strings.ReplaceAll(sh.name, "\\", "/")))
		_, err = io.Copy(h, io.LimitReader(rc, maxShard+1))
		rc.Close()
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ---- the message tree ----

// activePath returns the nodes from the root to current_node: the branch the
// user was on. Without a usable current_node it follows the last child down.
func activePath(c conversation) []node {
	var out []node
	seen := map[string]bool{}
	if cur, ok := c.Mapping[c.CurrentNode]; ok && c.CurrentNode != "" {
		for {
			if seen[cur.ID] && cur.ID != "" {
				break
			}
			seen[cur.ID] = true
			out = append(out, cur)
			if cur.Parent == nil {
				break
			}
			next, ok := c.Mapping[*cur.Parent]
			if !ok {
				break
			}
			if next.ID == "" {
				next.ID = *cur.Parent
			}
			cur = next
		}
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
		return out
	}
	for id, n := range c.Mapping {
		if n.Parent == nil {
			n.ID = id
			out = append(out, n)
			break
		}
	}
	for len(out) > 0 {
		last := out[len(out)-1]
		if len(last.Children) == 0 || seen[last.ID] {
			break
		}
		seen[last.ID] = true
		next, ok := c.Mapping[last.Children[len(last.Children)-1]]
		if !ok {
			break
		}
		next.ID = last.Children[len(last.Children)-1]
		out = append(out, next)
	}
	return out
}

// ---- cleaning ChatGPT's inline markers ----

// ChatGPT writes citations and entity links into the text between private-use
// characters:  kind  payload . A citation is dropped; an
// entity keeps its display name.    only bracket spans.
var inlineMarker = regexp.MustCompile("([^]*)")

var stray = strings.NewReplacer("", "", "", "", "", "", "", "", "", "", "", "")

func cleanText(s string) string {
	if !strings.ContainsAny(s, "") {
		return s
	}
	s = inlineMarker.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.TrimSuffix(strings.TrimPrefix(m, ""), "")
		kind, rest, _ := strings.Cut(inner, "")
		if kind != "entity" {
			return ""
		}
		var fields []any
		if json.Unmarshal([]byte(rest), &fields) == nil && len(fields) >= 2 {
			if name, ok := fields[1].(string); ok {
				return name
			}
		}
		return ""
	})
	return stray.Replace(s)
}

// blocks turns one message into Claude-shaped content blocks (the transcript
// reader already understands them) and counts what it had to leave out.
//
// imageURL turns an image's asset pointer into the address the reader shows it
// from, or "" when the image was not kept (nil = images are not being kept).
func blocks(m *message, imageURL func(pointer string) string) (out []map[string]any, skipped int) {
	add := func(typ, key, text string) {
		if text = strings.TrimSpace(cleanText(text)); text != "" {
			out = append(out, map[string]any{"type": typ, key: text})
		}
	}
	switch m.Content.Type {
	case "thoughts":
		var parts []string
		for _, t := range m.Content.Thoughts {
			if t.Summary != "" {
				parts = append(parts, "**"+strings.TrimSpace(t.Summary)+"**")
			}
			if strings.TrimSpace(t.Content) != "" {
				parts = append(parts, strings.TrimSpace(t.Content))
			}
		}
		add("thinking", "thinking", strings.Join(parts, "\n\n"))
	case "reasoning_recap":
		// "Thought for 12s": a label, not content
	default:
		// A message stays one text block (the reader counts one question per user
		// text block). A stored picture is a block of its own: the full-text index
		// leaves image blocks out, so its address never shows up in a search
		// result or matches a query. Pictures that come before the first text go
		// before the text block, the rest after it.
		// The pictures the user attached carry their file name in the message's
		// metadata; it labels the picture (or its placeholder).
		pictureNames := map[string]string{}
		for _, a := range m.Metadata.Attachments {
			if strings.HasPrefix(a.Mime, "image/") && a.Name != "" {
				pictureNames[fileKey(a.ID)] = a.Name
			}
		}
		var texts []string
		var lead, trail []map[string]any
		pic := func(u, name string) {
			src := map[string]string{"type": "url", "url": u}
			if name != "" {
				src["name"] = name
			}
			b := map[string]any{"type": "image", "source": src}
			if len(texts) == 0 {
				lead = append(lead, b)
			} else {
				trail = append(trail, b)
			}
		}
		for _, raw := range m.Content.Parts {
			var s string
			if json.Unmarshal(raw, &s) == nil {
				texts = append(texts, s)
				continue
			}
			var part struct {
				Type    string `json:"content_type"`
				Text    string `json:"text"`
				Pointer string `json:"asset_pointer"`
			}
			if json.Unmarshal(raw, &part) != nil {
				skipped++
				continue
			}
			switch part.Type {
			case "audio_transcription":
				texts = append(texts, part.Text)
			case "image_asset_pointer":
				if imageURL != nil {
					if u := imageURL(part.Pointer); u != "" {
						pic(u, label(pictureNames[fileKey(part.Pointer)]))
						continue
					}
				}
				if name := label(pictureNames[fileKey(part.Pointer)]); name != "" {
					texts = append(texts, "*[图片："+name+"]*")
				} else {
					texts = append(texts, "*[图片]*")
				}
				skipped++
			default:
				// raw audio / video pointers: nothing to read
				skipped++
			}
		}
		// Files the user attached (not images: those are shown as images). The
		// export carries the files, not their text, so what is kept is the name.
		for _, a := range m.Metadata.Attachments {
			if a.Name != "" && !strings.HasPrefix(a.Mime, "image/") {
				texts = append(texts, "*[附件："+oneLine(a.Name)+humanSize(a.Size)+"]*")
			}
		}
		out = append(out, lead...)
		add("text", "text", strings.Join(texts, "\n\n"))
		out = append(out, trail...)
		if len(m.Content.Parts) == 0 && m.Content.Type != "text" && m.Content.Type != "multimodal_text" {
			skipped++
		}
	}
	return out, skipped
}

// fileKey reduces an attachment id or an asset pointer to the bare file id, so
// "file-service://file-AbC#x", "sediment://file_AbC" and "file-AbC" compare equal.
func fileKey(s string) string {
	if _, after, ok := strings.Cut(s, "://"); ok {
		s = after
	}
	s, _, _ = strings.Cut(s, "#")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "file-"), "file_")
	return s
}

// label makes a file name safe to sit inside Markdown brackets: one line, no
// brackets or parentheses, not unreasonably long.
func label(name string) string {
	name = strings.NewReplacer("[", " ", "]", " ", "(", " ", ")", " ", "*", " ", "`", " ").Replace(name)
	return truncateRunes(oneLine(name), 60)
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func humanSize(n int64) string {
	switch {
	case n <= 0:
		return ""
	case n < 1<<10:
		return fmt.Sprintf(" · %d B", n)
	case n < 1<<20:
		return fmt.Sprintf(" · %.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf(" · %.1f MB", float64(n)/(1<<20))
}

// ---- images ----

// maxImage bounds one image copied into the store.
const maxImage = 64 << 20

// imageExt names the image type from a file's first bytes; "" if it is not one
// the reader shows.
func imageExt(head []byte) string {
	switch {
	case len(head) >= 8 && string(head[:8]) == "\x89PNG\r\n\x1a\n":
		return "png"
	case len(head) >= 3 && head[0] == 0xff && head[1] == 0xd8 && head[2] == 0xff:
		return "jpg"
	case len(head) >= 6 && (string(head[:6]) == "GIF87a" || string(head[:6]) == "GIF89a"):
		return "gif"
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return "webp"
	}
	return ""
}

// storeImage copies one export member into dir under the hash of its bytes
// (the same image is kept once, however many conversations or exports use it)
// and returns the file name, or "" when it is not an image or too large.
func storeImage(dir string, f *zip.File) (string, error) {
	if f.UncompressedSize64 > maxImage {
		return "", nil
	}
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	head := make([]byte, 16)
	n, _ := io.ReadFull(rc, head)
	ext := imageExt(head[:n])
	if ext == "" {
		return "", nil
	}
	tmp, err := os.CreateTemp(dir, ".image-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	fail := func(err error) (string, error) {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	h := sha256.New()
	w := io.MultiWriter(tmp, h)
	if _, err := w.Write(head[:n]); err != nil {
		return fail(err)
	}
	if _, err := io.Copy(w, io.LimitReader(rc, maxImage)); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	name := hex.EncodeToString(h.Sum(nil)) + "." + ext
	final := filepath.Join(dir, name)
	if _, err := os.Lstat(final); err == nil {
		os.Remove(tmpName) // already kept
		return name, nil
	}
	if err := os.Rename(tmpName, final); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	return name, nil
}

// AssetPrefix is where the reader asks for stored images (see AssetMiddleware).
const AssetPrefix = sessions.LocalImagePrefix

// ---- Import ----

// canonicalAccount keeps the label the account was first imported under, so
// later batches cannot split one account into several filter options. A batch
// whose manifest cannot be read is ignored here (List reports it): one damaged
// batch must not take the account's other batches with it.
func canonicalAccount(accountRoot, fallback string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(accountRoot, "*", "manifest.json"))
	if err != nil {
		return "", err
	}
	var first time.Time
	label := fallback
	for _, p := range paths {
		if strings.HasPrefix(filepath.Base(filepath.Dir(p)), ".stage-") {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var meta struct {
			Version    int
			Account    string
			ImportedAt time.Time
		}
		if json.Unmarshal(data, &meta) != nil || meta.Version != 1 || strings.TrimSpace(meta.Account) == "" {
			continue
		}
		if first.IsZero() || meta.ImportedAt.Before(first) {
			first, label = meta.ImportedAt, meta.Account
		}
	}
	return label, nil
}

func validAccount(account string) (string, error) {
	account = strings.TrimSpace(account)
	if account == "" || len([]rune(account)) > maxAccountRunes {
		return "", errors.New("请填写 1–80 字的账号名称，用于区分不同账号的记录")
	}
	return account, nil
}

// Import reads the export at source and stores every conversation under root.
// opts.Names maps project ids to the names the user chose. Each batch is staged
// and published by one rename; the same export imported twice (with the same
// image choice) changes nothing.
func Import(root, source, account string, opts Options) (Report, error) {
	names := opts.Names
	report := Report{}
	account, err := validAccount(account)
	if err != nil {
		return report, err
	}
	report.Account = account
	if root == "" {
		return report, errors.New("未配置聊天导入存储目录")
	}
	if root, err = filepath.Abs(root); err != nil {
		return report, err
	}
	src, err := openSource(source)
	if err != nil {
		return report, err
	}
	defer src.close()
	shards := src.shards
	accountID := digest(strings.ToLower(account))
	accountRoot := filepath.Join(root, accountID)
	if account, err = canonicalAccount(accountRoot, account); err != nil {
		return report, err
	}
	report.Account = account
	hash, err := batchHash(shards)
	if err != nil {
		return report, err
	}
	if opts.Images {
		hash += "-img" // the same export with its images is a different batch
	}
	batch := filepath.Join(accountRoot, hash)
	if len(names) > 0 {
		if err := saveProjectNames(accountRoot, names); err != nil {
			return report, err
		}
	}
	if data, err := os.ReadFile(filepath.Join(batch, "manifest.json")); err == nil {
		var prior manifest
		if json.Unmarshal(data, &prior) != nil || prior.Version != 1 {
			return report, errors.New("已有的导入记录已损坏，原有文件已保留")
		}
		report.Conversations, report.AlreadyImported, report.SkippedBlocks = len(prior.Entries), true, prior.SkippedBlocks
		report.Projects, report.Images = countProjects(prior.Entries), prior.Images
		return report, nil
	}
	if err := os.MkdirAll(accountRoot, 0o700); err != nil {
		return report, err
	}
	sweepLeftovers(accountRoot)
	if opts.Images && opts.FreeSpace != nil && len(src.assets) > 0 {
		var need int64
		counted := map[string]bool{}
		if err := eachConversation(shards, func(c conversation) error {
			_, size := src.countImages(c, counted)
			need += size
			return nil
		}); err != nil {
			return report, err
		}
		if free, err := opts.FreeSpace(accountRoot); err == nil && free < need+need/10+(64<<20) {
			return report, fmt.Errorf("磁盘空间不足：导入图片约需 %s，当前可用约 %s。请取消选中“同时导入图片”，或释放一些空间后重试", humanBytes(need), humanBytes(free))
		}
	}
	stage, err := os.MkdirTemp(accountRoot, ".stage-")
	if err != nil {
		return report, err
	}
	defer func() {
		if stage != "" {
			// stage was created by this call, directly under accountRoot
			if rel, err := filepath.Rel(accountRoot, stage); err == nil && filepath.Dir(rel) == "." && strings.HasPrefix(filepath.Base(rel), ".stage-") {
				os.RemoveAll(stage)
			}
		}
	}()
	m := manifest{Version: 1, Account: account, ImportedAt: time.Now().UTC(), WithImages: opts.Images}
	// Images are kept in the account's own content-addressed folder, outside the
	// batch: the same picture in two exports is stored once. A failed import can
	// leave images behind; they are reused by the next attempt.
	var imageURL func(string) string
	var storeErr error
	keptAs := map[string]string{} // export member → stored name ("" = not kept)
	distinct := map[string]bool{}
	if opts.Images && len(src.assets) > 0 {
		assetsDir := filepath.Join(accountRoot, assetsDirName)
		if err := os.MkdirAll(assetsDir, 0o700); err != nil {
			return report, err
		}
		imageURL = func(pointer string) string {
			f := src.asset(pointer)
			if f == nil {
				return ""
			}
			name, done := keptAs[f.Name]
			if !done {
				var err error
				if name, err = storeImage(assetsDir, f); err != nil && storeErr == nil {
					storeErr = err
				}
				keptAs[f.Name] = name
			}
			if name == "" {
				return ""
			}
			distinct[name] = true
			return AssetPrefix + accountID + "/" + name
		}
	}
	seen := map[string]bool{}
	err = eachConversation(shards, func(c conversation) error {
		if c.id() == "" || c.Mapping == nil {
			return errors.New("有会话缺少必要的字段（id 或 mapping），未导入任何内容")
		}
		id := "gpt_" + digest(accountID+":"+c.id())
		if seen[id] {
			return errors.New("导出文件中有重复的会话 ID，未导入任何内容")
		}
		seen[id] = true
		e, skipped, err := writeConversation(filepath.Join(stage, id+".jsonl"), id, c, imageURL)
		if err != nil {
			return err
		}
		if e.Messages == 0 && strings.TrimSpace(c.Title) == "" {
			os.Remove(filepath.Join(stage, id+".jsonl")) // an empty, untitled chat is nothing to keep
			return nil
		}
		m.Entries = append(m.Entries, e)
		m.SkippedBlocks += skipped
		return nil
	})
	if err == nil && storeErr != nil {
		err = fmt.Errorf("无法复制图片，未导入任何内容。可以取消选中“同时导入图片”后重试（%v）", storeErr)
	}
	if err != nil {
		return report, err
	}
	if len(m.Entries) == 0 {
		return report, errors.New("导出文件中没有会话")
	}
	m.Images = len(distinct)
	data, err := json.Marshal(m)
	if err != nil {
		return report, err
	}
	if err := os.WriteFile(filepath.Join(stage, "manifest.json"), data, 0o600); err != nil {
		return report, err
	}
	if err := os.Rename(stage, batch); err != nil {
		return report, fmt.Errorf("无法保存导入的内容，原有数据未改动（%v）", err)
	}
	stage = ""
	report.Conversations, report.SkippedBlocks, report.Projects, report.Images = len(m.Entries), m.SkippedBlocks, countProjects(m.Entries), m.Images
	report.Reclaimed, report.ReclaimedBytes = reclaim(accountRoot)
	return report, nil
}

// sweepLeftovers removes what an import that was cut short (crash, power loss)
// leaves behind: its staging folder and half-written image copies. Only entries
// of exactly those names, and only old ones, so a running import is never touched.
func sweepLeftovers(accountRoot string) {
	old := func(info os.FileInfo) bool { return time.Since(info.ModTime()) > time.Hour }
	if entries, err := os.ReadDir(accountRoot); err == nil {
		for _, e := range entries {
			if info, err := e.Info(); err == nil && e.IsDir() && strings.HasPrefix(e.Name(), ".stage-") && old(info) {
				os.RemoveAll(filepath.Join(accountRoot, e.Name()))
			}
		}
	}
	if entries, err := os.ReadDir(filepath.Join(accountRoot, assetsDirName)); err == nil {
		for _, e := range entries {
			if info, err := e.Info(); err == nil && !e.IsDir() && strings.HasPrefix(e.Name(), ".image-") && strings.HasSuffix(e.Name(), ".tmp") && old(info) {
				os.Remove(filepath.Join(accountRoot, assetsDirName, e.Name()))
			}
		}
	}
}

func humanBytes(n int64) string {
	switch {
	case n < 1<<20:
		return fmt.Sprintf("%d KB", (n+1023)>>10)
	case n < 1<<30:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
}

func countProjects(entries []entry) int {
	ids := map[string]bool{}
	for _, e := range entries {
		if e.Project != "" {
			ids[e.Project] = true
		}
	}
	return len(ids)
}

func writeConversation(file, id string, c conversation, imageURL func(string) string) (entry, int, error) {
	e := entry{ID: id, Title: truncateRunes(oneLine(c.Title), maxTitleRunes), Created: seconds(c.Created), Updated: seconds(c.Updated), Model: c.Model}
	if c.Template != nil && strings.HasPrefix(*c.Template, "g-p-") {
		e.Project = *c.Template
	}
	if e.Title == "" {
		e.Title = "未命名聊天"
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return e, 0, err
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(map[string]any{"type": "ai-title", "aiTitle": e.Title}); err != nil {
		f.Close()
		return e, 0, err
	}
	skipped := 0
	for index, n := range activePath(c) {
		m := n.Message
		if m == nil || m.Metadata.Hidden {
			continue
		}
		role := m.Author.Role
		if role != "user" && role != "assistant" {
			// system prompts and tool traffic are not what was said
			continue
		}
		bs, sk := blocks(m, imageURL)
		skipped += sk
		if len(bs) == 0 {
			continue
		}
		when := time.Time{}
		if m.Created != nil {
			when = seconds(*m.Created)
		}
		if e.Created.IsZero() || (!when.IsZero() && when.Before(e.Created)) {
			e.Created = when
		}
		if when.After(e.Updated) {
			e.Updated = when
		}
		line := map[string]any{"type": role, "uuid": fmt.Sprintf("message-%d", index), "timestamp": when, "sessionId": id,
			"message": map[string]any{"role": role, "content": bs}}
		if m.Metadata.Model != "" && role == "assistant" {
			line["message"].(map[string]any)["model"] = m.Metadata.Model
		}
		if err := enc.Encode(line); err != nil {
			f.Close()
			return e, skipped, err
		}
		e.Messages++
	}
	return e, skipped, f.Close()
}

// ---- Preview ----

// Preview reads an export without storing anything: how many conversations it
// holds and which ChatGPT Projects they belong to, so the user can name them.
func Preview(root, source, account string) (PreviewResult, error) {
	res := PreviewResult{}
	src, err := openSource(source)
	if err != nil {
		return res, err
	}
	defer src.close()
	shards := src.shards
	var names map[string]string
	if acc := strings.TrimSpace(account); acc != "" && root != "" {
		if abs, err := filepath.Abs(root); err == nil {
			names = loadProjectNames(filepath.Join(abs, digest(strings.ToLower(acc))))
		}
	}
	type seenProject struct {
		count   int
		titles  []string
		samples []struct {
			title string
			at    float64
		}
	}
	byID := map[string]*seenProject{}
	images := map[string]bool{} // export members the conversations show as images
	var convIDs []string
	err = eachConversation(shards, func(c conversation) error {
		res.Conversations++
		if id := c.id(); id != "" {
			convIDs = append(convIDs, id)
		}
		n, size := src.countImages(c, images)
		res.Images += n
		res.ImageBytes += size
		if c.Template == nil || !strings.HasPrefix(*c.Template, "g-p-") {
			return nil
		}
		p := byID[*c.Template]
		if p == nil {
			p = &seenProject{}
			byID[*c.Template] = p
		}
		p.count++
		if t := strings.TrimSpace(c.Title); t != "" && len(p.titles) < maxTitlesPerProject {
			p.titles = append(p.titles, t)
		}
		p.samples = append(p.samples, struct {
			title string
			at    float64
		}{strings.TrimSpace(c.Title), c.Updated})
		if len(p.samples) > 12 { // keep memory flat: only the latest few matter
			sort.Slice(p.samples, func(i, j int) bool { return p.samples[i].at > p.samples[j].at })
			p.samples = p.samples[:3]
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	res.Accounts = existingAccounts(root, convIDs)
	files := src.projectFiles()
	for id, p := range byID {
		sort.Slice(p.samples, func(i, j int) bool { return p.samples[i].at > p.samples[j].at })
		info := ProjectInfo{ID: id, Name: names[id], Default: defaultName(id), Conversations: p.count, Files: files[id]}
		for i := 0; i < len(p.samples) && i < 3; i++ {
			if p.samples[i].title != "" {
				info.Samples = append(info.Samples, p.samples[i].title)
			}
		}
		latest := ""
		if len(info.Samples) > 0 {
			latest = info.Samples[0]
		}
		info.Recommended, info.RecommendedBasis = recommendName(p.titles, latest, info.Files)
		res.Projects = append(res.Projects, info)
	}
	sort.Slice(res.Projects, func(i, j int) bool {
		if res.Projects[i].Conversations != res.Projects[j].Conversations {
			return res.Projects[i].Conversations > res.Projects[j].Conversations
		}
		return res.Projects[i].ID < res.Projects[j].ID
	})
	return res, nil
}

// ---- project names ----

func defaultName(id string) string {
	short := strings.TrimPrefix(id, "g-p-")
	if len(short) > 6 {
		short = short[:6]
	}
	return "ChatGPT 项目 " + short
}

// cleanName makes a project name safe to be a path element (it becomes the
// project's folder-like label in the list).
func cleanName(name string) string {
	name = strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) || r < 0x20 {
			return ' '
		}
		return r
	}, name)), " ")
	if r := []rune(name); len(r) > maxProjectRunes {
		name = strings.TrimSpace(string(r[:maxProjectRunes]))
	}
	if strings.Trim(name, ". ") == "" {
		return ""
	}
	return name
}

func loadProjectNames(accountRoot string) map[string]string {
	data, err := os.ReadFile(filepath.Join(accountRoot, projectsFile))
	if err != nil {
		return map[string]string{}
	}
	var f struct {
		Version int
		Names   map[string]string
	}
	if json.Unmarshal(data, &f) != nil || f.Names == nil {
		return map[string]string{}
	}
	return f.Names
}

func saveProjectNames(accountRoot string, names map[string]string) error {
	if err := os.MkdirAll(accountRoot, 0o700); err != nil {
		return err
	}
	merged := loadProjectNames(accountRoot)
	for id, name := range names {
		if !strings.HasPrefix(id, "g-p-") {
			continue
		}
		if name = cleanName(name); name == "" {
			delete(merged, id)
		} else {
			merged[id] = name
		}
	}
	data, err := json.Marshal(map[string]any{"version": 1, "names": merged})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(accountRoot, ".projects-*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(accountRoot, projectsFile)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// SetProjectNames renames projects of an already imported account.
func SetProjectNames(root, account string, names map[string]string) error {
	account, err := validAccount(account)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	return saveProjectNames(filepath.Join(abs, digest(strings.ToLower(account))), names)
}

// ---- List ----

var validID = regexp.MustCompile(`^gpt_[a-f0-9]{64}$`)

// copyInfo is what decides which of two stored copies of one conversation is shown.
type copyInfo struct {
	updated    time.Time
	importedAt time.Time
	images     bool // the batch that carries the pictures
}

// beats reports whether copy a is the one to show instead of b: the newer
// conversation; of two equally new, the one with its pictures (it shows everything
// the other does); then the later import.
func (a copyInfo) beats(b copyInfo) bool {
	if !a.updated.Equal(b.updated) {
		return a.updated.After(b.updated)
	}
	if a.images != b.images {
		return a.images
	}
	return a.importedAt.After(b.importedAt)
}

// List merges every stored batch by stable account+conversation identity; an
// older export never replaces a newer copy of the same conversation, and a copy
// with its pictures is preferred to one without. A project
// conversation's Cwd is "ChatGPT/<project name>" — a label, not a real folder —
// which is what groups it under that project in the sidebar.
//
// A damaged batch or a missing transcript file skips only what it affects: the
// sessions that can be read are returned together with an error that says what
// was left out. skip, when given, names conversation ids that were deleted on
// purpose: they are left out without a word, even where their files are gone.
func List(root string, skip func(id string) bool) ([]sessions.Session, error) {
	paths, err := filepath.Glob(filepath.Join(root, "*", "*", "manifest.json"))
	if err != nil {
		return nil, err
	}
	type stored struct {
		session sessions.Session
		copy    copyInfo
	}
	var problems []error
	byID := map[string]stored{}
	labels := map[string]string{}
	projectNames := map[string]map[string]string{}
	for _, p := range paths {
		if strings.HasPrefix(filepath.Base(filepath.Dir(p)), ".stage-") {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		var m manifest
		if err := json.Unmarshal(data, &m); err != nil || m.Version != 1 {
			problems = append(problems, fmt.Errorf("导入记录已损坏：%s", filepath.Base(filepath.Dir(p))))
			continue
		}
		accountRoot := filepath.Dir(filepath.Dir(p))
		label, ok := labels[accountRoot]
		if !ok {
			if label, err = canonicalAccount(accountRoot, m.Account); err != nil {
				problems = append(problems, err)
				continue
			}
			labels[accountRoot] = label
			projectNames[accountRoot] = loadProjectNames(accountRoot)
		}
		for _, e := range m.Entries {
			if !validID.MatchString(e.ID) {
				problems = append(problems, errors.New("导入记录中有无效的会话 ID"))
				continue
			}
			if skip != nil && skip(e.ID) {
				continue
			}
			file := filepath.Join(filepath.Dir(p), e.ID+".jsonl")
			if info, err := os.Stat(file); err != nil || !info.Mode().IsRegular() {
				problems = append(problems, fmt.Errorf("无法读取已导入的正文文件：%s", filepath.Base(file)))
				continue
			}
			key := filepath.Base(accountRoot) + "/" + e.ID
			c := copyInfo{updated: e.Updated, importedAt: m.ImportedAt, images: m.WithImages}
			if prior, exists := byID[key]; exists && !c.beats(prior.copy) {
				continue
			}
			cwd := ""
			if e.Project != "" {
				name := projectNames[accountRoot][e.Project]
				if name == "" {
					name = defaultName(e.Project)
				}
				cwd = filepath.Join("ChatGPT", name)
			}
			models := []sessions.Model{}
			if e.Model != "" {
				models = append(models, sessions.Model{Model: e.Model})
			}
			byID[key] = stored{session: sessions.Session{Agent: Agent, ID: e.ID, Title: e.Title, Cwd: cwd, Account: label, Imported: true, MessageCount: e.Messages,
				Start: e.Created, Last: e.Updated, ReadOnly: true, Transcript: true, Path: file, Models: models}, copy: c}
		}
	}
	out := make([]sessions.Session, 0, len(byID))
	for _, row := range byID {
		out = append(out, row.session)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Last.Equal(out[j].Last) {
			return out[i].Last.After(out[j].Last)
		}
		return out[i].ID < out[j].ID
	})
	return out, errors.Join(problems...)
}

// TranscriptFiles lists every stored copy of one conversation's transcript, in
// every batch: a conversation imported from several exports has one in each.
// Only regular files inside root's own account/batch folders are returned, so
// deleting "the conversation" can never reach anything else.
func TranscriptFiles(root, id string) ([]string, error) {
	if !validID.MatchString(id) {
		return nil, errors.New("会话 ID 无效")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(abs, "*", "*", id+".jsonl"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range paths {
		batch, account := filepath.Dir(p), filepath.Dir(filepath.Dir(p))
		if strings.HasPrefix(filepath.Base(batch), ".stage-") || !accountDir.MatchString(filepath.Base(account)) || filepath.Dir(account) != abs {
			continue
		}
		if info, err := os.Lstat(p); err != nil || !info.Mode().IsRegular() {
			continue
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

var accountDir = regexp.MustCompile(`^[0-9a-f]{64}$`)

// existingAccounts compares an export's conversation ids with the accounts already
// stored. A session's id is derived from the account folder and the ChatGPT
// conversation id, so the same conversation under another account name has a
// different id — which is how a second spelling of the account name would quietly
// store everything twice. Here the ids are worked out for each stored account.
func existingAccounts(root string, convIDs []string) []ExistingAccount {
	abs, err := filepath.Abs(root)
	if err != nil || len(convIDs) == 0 {
		return nil
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil
	}
	var out []ExistingAccount
	for _, e := range entries {
		if !e.IsDir() || !accountDir.MatchString(e.Name()) {
			continue
		}
		accountRoot := filepath.Join(abs, e.Name())
		have := map[string]bool{}
		manifests, _ := filepath.Glob(filepath.Join(accountRoot, "*", "manifest.json"))
		for _, p := range manifests {
			if strings.HasPrefix(filepath.Base(filepath.Dir(p)), ".stage-") {
				continue
			}
			data, err := os.ReadFile(p)
			var m manifest
			if err != nil || json.Unmarshal(data, &m) != nil || m.Version != 1 {
				continue
			}
			for _, entry := range m.Entries {
				have[entry.ID] = true
			}
		}
		overlap := 0
		for _, id := range convIDs {
			if have["gpt_"+digest(e.Name()+":"+id)] {
				overlap++
			}
		}
		if overlap == 0 {
			continue
		}
		name, _ := canonicalAccount(accountRoot, "")
		out = append(out, ExistingAccount{Name: name, Overlap: overlap, Missing: max(0, len(have)-overlap)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Overlap != out[j].Overlap {
			return out[i].Overlap > out[j].Overlap
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// reclaim removes the batches of one account that no longer show anything: every
// conversation in them has a copy elsewhere that List would show instead (newer,
// or as new with its pictures, or imported later). Each export is stored whole, so
// without this every re-import would leave the unchanged conversations behind one
// more time. A batch is touched only when its manifest reads cleanly; only the
// files that manifest names and the manifest itself are removed, and its folder
// only if that leaves it empty — never a recursive delete. It reports how many
// batches and bytes went; failures just leave the batch for next time.
func reclaim(accountRoot string) (batches int, bytes int64) {
	type batch struct {
		dir string
		m   manifest
	}
	manifests, _ := filepath.Glob(filepath.Join(accountRoot, "*", "manifest.json"))
	var all []batch
	winner := map[string]string{} // conversation id → the batch folder whose copy is shown
	best := map[string]copyInfo{}
	for _, p := range manifests {
		dir := filepath.Dir(p)
		if strings.HasPrefix(filepath.Base(dir), ".stage-") {
			continue
		}
		data, err := os.ReadFile(p)
		var m manifest
		if err != nil || json.Unmarshal(data, &m) != nil || m.Version != 1 {
			continue // never touch a batch that cannot be read
		}
		all = append(all, batch{dir, m})
		for _, e := range m.Entries {
			if !validID.MatchString(e.ID) {
				continue
			}
			if info, err := os.Stat(filepath.Join(dir, e.ID+".jsonl")); err != nil || !info.Mode().IsRegular() {
				continue // a copy that is gone shows nothing
			}
			c := copyInfo{updated: e.Updated, importedAt: m.ImportedAt, images: m.WithImages}
			if prior, ok := best[e.ID]; !ok || c.beats(prior) {
				best[e.ID], winner[e.ID] = c, dir
			}
		}
	}
	for _, b := range all {
		shown := false
		for _, e := range b.m.Entries {
			if winner[e.ID] == b.dir {
				shown = true
				break
			}
		}
		if shown {
			continue
		}
		clean := true
		for _, e := range b.m.Entries {
			if !validID.MatchString(e.ID) {
				continue
			}
			file := filepath.Join(b.dir, e.ID+".jsonl")
			info, err := os.Lstat(file)
			if err != nil {
				continue
			}
			if !info.Mode().IsRegular() {
				clean = false
				continue
			}
			if os.Remove(file) == nil {
				bytes += info.Size()
			} else {
				clean = false
			}
		}
		if os.Remove(filepath.Join(b.dir, "manifest.json")) != nil {
			clean = false
		}
		os.Remove(b.dir) // only if it is empty by now
		if clean {
			batches++
		}
	}
	return batches, bytes
}

// ---- serving stored images to the reader ----

// assetPath is the only shape of address AssetMiddleware serves: the account's
// folder id and the image's content hash, both 64 hex digits.
var assetPath = regexp.MustCompile(`^` + AssetPrefix + `([0-9a-f]{64})/([0-9a-f]{64}\.(?:png|jpg|gif|webp))$`)

var assetTypes = map[string]string{"png": "image/png", "jpg": "image/jpeg", "gif": "image/gif", "webp": "image/webp"}

// AssetMiddleware serves the images stored under root at
// AssetPrefix<account id>/<hash>.<ext> and passes every other request on. It
// reads only files of exactly that name inside an account's own assets folder;
// nothing else on disk can be reached through it.
func AssetMiddleware(root string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, AssetPrefix) {
				next.ServeHTTP(w, r)
				return
			}
			m := assetPath.FindStringSubmatch(r.URL.Path)
			if m == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
				http.NotFound(w, r)
				return
			}
			file := filepath.Join(root, m[1], assetsDirName, m[2])
			info, err := os.Lstat(file)
			if err != nil || !info.Mode().IsRegular() {
				http.NotFound(w, r)
				return
			}
			f, err := os.Open(file)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer f.Close()
			w.Header().Set("Content-Type", assetTypes[strings.TrimPrefix(filepath.Ext(m[2]), ".")])
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
			w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
			w.Header().Set("Cache-Control", "private, max-age=31536000, immutable") // the name is the content's hash
			http.ServeContent(w, r, "", info.ModTime(), f)
		})
	}
}

// AssetFile resolves an image address the reader holds to its file under root,
// for copying it out (Markdown export). ok is false for anything that is not a
// stored image.
func AssetFile(root, address string) (file string, name string, ok bool) {
	m := assetPath.FindStringSubmatch(address)
	if m == nil {
		return "", "", false
	}
	file = filepath.Join(root, m[1], assetsDirName, m[2])
	if info, err := os.Lstat(file); err != nil || !info.Mode().IsRegular() {
		return "", "", false
	}
	return file, m[2], true
}

// AssetAddress matches an image address inside Markdown text:
// ![alt](/_chatgpt/<account>/<hash>.<ext>).
var AssetAddress = regexp.MustCompile(`!\[([^\]]*)\]\((` + AssetPrefix + `[0-9a-f]{64}/[0-9a-f]{64}\.(?:png|jpg|gif|webp))\)`)
