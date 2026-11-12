package chatgpthistory

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// a valid 1x1 PNG
var tinyPNG = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 13, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89}

// imageChat has one user turn showing three pictures: one stored under a
// "sediment" pointer, the same bytes under a "file-service" pointer, one the
// export does not carry — plus a text file the user attached.
func imageChat() tnode {
	img := func(ptr string) tnode { return tnode{"content_type": "image_asset_pointer", "asset_pointer": ptr} }
	return tnode{
		"id": "ci", "conversation_id": "ci", "title": "带图片", "create_time": 1_760_000_000, "update_time": 1_760_000_100,
		"current_node": "a", "conversation_template_id": nil,
		"mapping": tnode{
			"r": msgNode("r", nil, []string{"u"}, "", "", nil, 0, nil),
			"u": msgNode("u", "r", []string{"a"}, "user", "multimodal_text",
				[]any{"看图", img("sediment://file_img1"), img("file-service://file-img2#x"), img("sediment://file_missing"), img("sediment://file_bad")}, 1_760_000_001,
				tnode{"attachments": []any{
					tnode{"id": "file-t", "name": "报表.txt", "mime_type": "text/plain", "size": 2048},
					tnode{"id": "file-p", "name": "截图.png", "mime_type": "image/png", "size": 10},
					tnode{"id": "file_img1", "name": "相机 [1].png", "mime_type": "image/png", "size": 10},
				}}),
			"a": msgNode("a", "u", nil, "assistant", "text", []any{"收到"}, 1_760_000_002, nil),
		},
	}
}

func writeImageExport(t *testing.T, dir string) string {
	t.Helper()
	zp := filepath.Join(dir, "img-export.zip")
	f, err := os.Create(zp)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("conversations-000.json")
	json.NewEncoder(w).Encode([]tnode{imageChat()})
	for name, body := range map[string][]byte{"file_img1.dat": tinyPNG, "file-img2.dat": tinyPNG, "file_bad.dat": []byte("just text, not an image"), "file-t.dat": []byte("attachment text")} {
		w, _ := zw.Create(name)
		w.Write(body)
	}
	zw.Close()
	f.Close()
	return zp
}

func transcriptText(t *testing.T, s sessions.Session) string {
	t.Helper()
	tr, err := sessions.TranscriptOf(s)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, p := range tr.Parts {
		b.WriteString(p.Text + "\n")
	}
	return b.String()
}

func TestImportImages(t *testing.T) {
	dir := t.TempDir()
	zp := writeImageExport(t, dir)
	root := filepath.Join(dir, "store")

	pre, err := Preview(root, zp, "A")
	if err != nil {
		t.Fatal(err)
	}
	// three referenced members the export carries (img1, img2, bad); the missing one is not counted
	if pre.Images != 3 || pre.ImageBytes != int64(2*len(tinyPNG)+len("just text, not an image")) {
		t.Fatalf("preview images = %d / %d bytes", pre.Images, pre.ImageBytes)
	}

	rep, err := Import(root, zp, "A", Options{Images: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Images != 1 {
		t.Fatalf("同样的字节只存一份，Images = %d", rep.Images)
	}
	accountID := digest("a")
	assets, _ := os.ReadDir(filepath.Join(root, accountID, "assets"))
	if len(assets) != 1 || !strings.HasSuffix(assets[0].Name(), ".png") {
		t.Fatalf("assets = %v", assets)
	}
	list, _ := List(root)
	if len(list) != 1 {
		t.Fatalf("list = %d", len(list))
	}
	text := transcriptText(t, list[0])
	addr := "![图片](" + AssetPrefix + accountID + "/" + assets[0].Name() + ")"
	if strings.Count(text, "("+AssetPrefix+accountID+"/"+assets[0].Name()+")") != 2 || strings.Count(text, addr) != 1 {
		t.Fatalf("两个指针应指向同一个存下来的图片 %q:\n%s", addr, text)
	}
	if strings.Count(text, "*[图片]*") != 2 { // the missing one and the non-image one
		t.Fatalf("没有保存的图片应留占位:\n%s", text)
	}
	if !strings.Contains(text, "*[附件：报表.txt · 2 KB]*") || strings.Contains(text, "截图.png") {
		t.Fatalf("非图片附件写文件名，图片附件不重复写:\n%s", text)
	}
	// the reader shows the stored picture under the name it was attached with
	if !strings.Contains(text, "![相机 1 .png]("+AssetPrefix+accountID+"/"+assets[0].Name()+")") {
		t.Fatalf("图片应带着上传时的文件名作替代文字:\n%s", text)
	}
	if !AssetAddress.MatchString(addr) {
		t.Fatalf("AssetAddress 不认识 %q", addr)
	}

	// the same export, same choice: nothing new; without images it is another batch
	if again, err := Import(root, zp, "A", Options{Images: true}); err != nil || !again.AlreadyImported || again.Images != 1 {
		t.Fatalf("again = %+v %v", again, err)
	}
	plain, err := Import(root, zp, "A", Options{})
	if err != nil || plain.AlreadyImported || plain.Images != 0 {
		t.Fatalf("plain = %+v %v", plain, err)
	}
	// equally new, the copy with its pictures is the one shown — even though the other came later
	list, _ = List(root)
	if len(list) != 1 || !strings.Contains(transcriptText(t, list[0]), AssetPrefix) {
		t.Fatal("同样新的两份，带图片的应优先显示")
	}
}

func TestImportWithoutImagesKeepsPlaceholders(t *testing.T) {
	dir := t.TempDir()
	zp := writeImageExport(t, dir)
	root := filepath.Join(dir, "store")
	if _, err := Import(root, zp, "A", Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, digest("a"), "assets")); err == nil {
		t.Fatal("不要图片时不该建 assets 目录")
	}
	list, _ := List(root)
	if text := transcriptText(t, list[0]); strings.Count(text, "*[图片") != 4 || strings.Count(text, "*[图片：相机 1 .png]*") != 1 || strings.Contains(text, AssetPrefix) {
		t.Fatalf("应全是占位:\n%s", text)
	}
}

func TestAssetMiddleware(t *testing.T) {
	dir := t.TempDir()
	zp := writeImageExport(t, dir)
	root := filepath.Join(dir, "store")
	if _, err := Import(root, zp, "A", Options{Images: true}); err != nil {
		t.Fatal(err)
	}
	accountID := digest("a")
	assets, _ := os.ReadDir(filepath.Join(root, accountID, "assets"))
	good := AssetPrefix + accountID + "/" + assets[0].Name()

	// a file outside any assets folder, reachable only if the handler were sloppy
	if err := os.WriteFile(filepath.Join(root, accountID, "secret.png"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	passed := false
	h := AssetMiddleware(root)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { passed = true }))
	do := func(method, target string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
		return rec
	}
	if rec := do("GET", good); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("good: %d %v", rec.Code, rec.Header())
	} else if b, _ := io.ReadAll(rec.Body); string(b) != string(tinyPNG) {
		t.Fatal("字节不对")
	}
	if rec := do("HEAD", good); rec.Code != 200 {
		t.Fatalf("HEAD: %d", rec.Code)
	}
	for _, bad := range []string{
		AssetPrefix + accountID + "/../secret.png",
		AssetPrefix + accountID + "/secret.png",
		AssetPrefix + accountID + "/" + strings.Repeat("0", 64) + ".png", // not stored
		AssetPrefix + "../../" + accountID + "/assets/" + assets[0].Name(),
		AssetPrefix + accountID[:63] + "/" + assets[0].Name(),
		AssetPrefix + accountID + "/" + strings.TrimSuffix(assets[0].Name(), ".png") + ".svg",
		AssetPrefix,
	} {
		if rec := do("GET", bad); rec.Code != 404 {
			t.Errorf("%q = %d，应 404", bad, rec.Code)
		}
	}
	if rec := do("POST", good); rec.Code != 404 {
		t.Errorf("POST = %d", rec.Code)
	}
	if passed {
		t.Fatal("带前缀的请求不该落到下一个处理器")
	}
	do("GET", "/index.html")
	if !passed {
		t.Fatal("别的路径必须交给下一个处理器")
	}

	if file, name, ok := AssetFile(root, good); !ok || name != assets[0].Name() || !strings.HasPrefix(file, root) {
		t.Fatalf("AssetFile = %q %q %v", file, name, ok)
	}
	if _, _, ok := AssetFile(root, AssetPrefix+accountID+"/../secret.png"); ok {
		t.Fatal("AssetFile 接受了越界路径")
	}
}

func TestImportRefusesWhenDiskIsTooSmall(t *testing.T) {
	dir := t.TempDir()
	zp := writeImageExport(t, dir)
	root := filepath.Join(dir, "store")
	_, err := Import(root, zp, "A", Options{Images: true, FreeSpace: func(string) (int64, error) { return 1 << 20, nil }})
	if err == nil || !strings.Contains(err.Error(), "磁盘空间不足") {
		t.Fatalf("err = %v", err)
	}
	if list, _ := List(root); len(list) != 0 {
		t.Fatal("空间不足时不该留下数据")
	}
	// the same import is fine with room, and without images the check is skipped
	if _, err := Import(root, zp, "A", Options{Images: true, FreeSpace: func(string) (int64, error) { return 10 << 30, nil }}); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(filepath.Join(dir, "other"), zp, "A", Options{FreeSpace: func(string) (int64, error) { return 0, nil }}); err != nil {
		t.Fatalf("不拷图片就不查空间: %v", err)
	}
}

// One damaged batch must not hide every other ChatGPT session.
func TestListKeepsWhatCanBeRead(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "store")
	zA := writeExport(t, mk(t, dir, "a"), fixtureShards())
	if _, err := Import(root, zA, "A", Options{}); err != nil {
		t.Fatal(err)
	}
	zB := writeImageExport(t, mk(t, dir, "b"))
	if _, err := Import(root, zB, "A", Options{}); err != nil {
		t.Fatal(err)
	}
	manifests, _ := filepath.Glob(filepath.Join(root, "*", "*", "manifest.json"))
	if len(manifests) != 2 {
		t.Fatalf("manifests = %d", len(manifests))
	}
	os.WriteFile(manifests[0], []byte("{broken"), 0o600)
	list, err := List(root)
	if err == nil || !strings.Contains(err.Error(), "已损坏") {
		t.Fatalf("应报告哪里坏了: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("一个批次损坏不该让其余会话全部消失")
	}
	// a transcript file that went missing drops only that session
	var gone string
	list, _ = List(root)
	gone = list[0].Path
	os.Remove(gone)
	after, err := List(root)
	if err == nil || len(after) != len(list)-1 {
		t.Fatalf("缺一个正文文件只少一个会话: %d → %d, %v", len(list), len(after), err)
	}
}

func TestProjectNamesAreSafeAsPathElements(t *testing.T) {
	for in, want := range map[string]string{"..": "", ".": "", " . . ": "", "...a": "...a", "a/b": "a b", "  ": ""} {
		if got := cleanName(in); got != want {
			t.Errorf("cleanName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := seconds(1e30); !got.IsZero() {
		t.Errorf("越界的时间戳应视为未知，得到 %v", got)
	}
	if got := truncateRunes(oneLine(strings.Repeat("长 ", 400)), maxTitleRunes); len([]rune(got)) > maxTitleRunes+1 {
		t.Errorf("标题没有截断: %d", len([]rune(got)))
	}
}

func TestSweepLeftoversOnlyTouchesOldOwnFiles(t *testing.T) {
	acc := t.TempDir()
	assets := filepath.Join(acc, "assets")
	os.MkdirAll(assets, 0o700)
	oldStage, freshStage := filepath.Join(acc, ".stage-old"), filepath.Join(acc, ".stage-fresh")
	os.MkdirAll(oldStage, 0o700)
	os.MkdirAll(freshStage, 0o700)
	oldTmp, freshTmp, keep := filepath.Join(assets, ".image-1.tmp"), filepath.Join(assets, ".image-2.tmp"), filepath.Join(assets, strings.Repeat("a", 64)+".png")
	for _, p := range []string{oldTmp, freshTmp, keep} {
		os.WriteFile(p, []byte("x"), 0o600)
	}
	past := time.Now().Add(-3 * time.Hour)
	for _, p := range []string{oldStage, oldTmp, keep} {
		os.Chtimes(p, past, past)
	}
	sweepLeftovers(acc)
	for p, wantGone := range map[string]bool{oldStage: true, oldTmp: true, freshStage: false, freshTmp: false, keep: false} {
		if _, err := os.Stat(p); (err != nil) != wantGone {
			t.Errorf("%s: gone=%v, want %v", filepath.Base(p), err != nil, wantGone)
		}
	}
}

func TestAssetResponseHeaders(t *testing.T) {
	dir := t.TempDir()
	zp := writeImageExport(t, dir)
	root := filepath.Join(dir, "store")
	if _, err := Import(root, zp, "A", Options{Images: true}); err != nil {
		t.Fatal(err)
	}
	assets, _ := os.ReadDir(filepath.Join(root, digest("a"), "assets"))
	rec := httptest.NewRecorder()
	AssetMiddleware(root)(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest("GET", AssetPrefix+digest("a")+"/"+assets[0].Name(), nil))
	for k, v := range map[string]string{"Content-Security-Policy": "default-src 'none'; sandbox", "Cross-Origin-Resource-Policy": "same-origin", "X-Content-Type-Options": "nosniff"} {
		if rec.Header().Get(k) != v {
			t.Errorf("%s = %q", k, rec.Header().Get(k))
		}
	}
}

// A message with pictures is still one question: one text block, pictures before
// it when they came first, after it otherwise.
func TestBlocksKeepOneTextBlockPerMessage(t *testing.T) {
	url := func(p string) string {
		return AssetPrefix + strings.Repeat("a", 64) + "/" + strings.Repeat("b", 63) + string(p[len(p)-1]) + ".png"
	}
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	img := func(p string) json.RawMessage {
		return raw(tnode{"content_type": "image_asset_pointer", "asset_pointer": p})
	}
	kinds := func(bs []map[string]any) string {
		var k []string
		for _, b := range bs {
			k = append(k, b["type"].(string))
		}
		return strings.Join(k, ",")
	}
	var m message
	m.Content.Type = "multimodal_text"
	m.Content.Parts = []json.RawMessage{img("sediment://1"), raw("看这张"), img("sediment://2"), raw("还有这张")}
	m.Metadata.Attachments = append(m.Metadata.Attachments, struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Mime string `json:"mime_type"`
		Size int64  `json:"size"`
	}{"file-t", "a.txt", "text/plain", 5})
	bs, _ := blocks(&m, url)
	if got := kinds(bs); got != "image,text,image" {
		t.Fatalf("块的顺序 = %s", got)
	}
	text := bs[1]["text"].(string)
	if !strings.Contains(text, "看这张") || !strings.Contains(text, "还有这张") || !strings.Contains(text, "a.txt") {
		t.Fatalf("文字应合成一块: %q", text)
	}
	// text first: the pictures follow it
	m.Content.Parts = []json.RawMessage{raw("先说话"), img("sediment://1"), img("sediment://2")}
	if bs, _ = blocks(&m, url); kinds(bs) != "text,image,image" {
		t.Fatalf("块的顺序 = %s", kinds(bs))
	}
}

// A picture is labelled with the file name the user attached it under; a picture
// that was not kept keeps the name in its placeholder.
func TestPictureNames(t *testing.T) {
	url := func(p string) string {
		if strings.HasSuffix(p, "kept") {
			return AssetPrefix + strings.Repeat("a", 64) + "/" + strings.Repeat("b", 64) + ".png"
		}
		return ""
	}
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	img := func(p string) json.RawMessage {
		return raw(tnode{"content_type": "image_asset_pointer", "asset_pointer": p})
	}
	var m message
	m.Content.Type = "multimodal_text"
	m.Content.Parts = []json.RawMessage{img("sediment://file_kept"), img("file-service://file-lost#x")}
	type att = struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Mime string `json:"mime_type"`
		Size int64  `json:"size"`
	}
	m.Metadata.Attachments = []att{{"file-kept", "截图 [1].png", "image/png", 9}, {"file-lost", "旧图.png", "image/png", 9}}
	bs, _ := blocks(&m, url)
	src, _ := bs[0]["source"].(map[string]string)
	if bs[0]["type"] != "image" || src["name"] != "截图 1 .png" {
		t.Fatalf("保存下来的图应带文件名（去掉会破坏 Markdown 的符号）: %v", bs[0])
	}
	if text, _ := bs[1]["text"].(string); !strings.Contains(text, "*[图片：旧图.png]*") {
		t.Fatalf("没保存的图占位符应带文件名: %v", bs[1])
	}
}
