package app

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// writeChatGPTImageExport makes a small ChatGPT export ZIP: one project
// conversation showing one picture.
func writeChatGPTImageExport(t *testing.T, dir string) (zipPath string, png []byte) {
	t.Helper()
	png = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 13, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89}
	shard := `[{"id":"cb","title":"备份里的图","create_time":1760000000,"update_time":1760000100,"current_node":"a","conversation_template_id":"g-p-0123456789abcdef","default_model_slug":"gpt-5",
	 "mapping":{"r":{"id":"r","parent":null,"children":["u"],"message":null},
	 "u":{"id":"u","parent":"r","children":["a"],"message":{"id":"u","author":{"role":"user"},"create_time":1760000001,"content":{"content_type":"multimodal_text","parts":["看图",{"content_type":"image_asset_pointer","asset_pointer":"sediment://file_img1"}]}}},
	 "a":{"id":"a","parent":"u","children":[],"message":{"id":"a","author":{"role":"assistant"},"create_time":1760000002,"content":{"content_type":"text","parts":["好的"]}}}}}]`
	zipPath = filepath.Join(dir, "export.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("conversations-000.json")
	w.Write([]byte(shard))
	w, _ = zw.Create("file_img1.dat")
	w.Write(png)
	zw.Close()
	f.Close()
	return zipPath, png
}

// 完整备份带上 ChatGPT 的会话、图片与项目名；恢复到另一台机器（另一个数据目录）后
// 照常显示，图片能被服务；目标上已有、内容不同的项目名文件不算冲突。
func TestBackupCarriesChatGPTImportsAndImages(t *testing.T) {
	src, _, _, _ := newTestApp(t)
	zp, png := writeChatGPTImageExport(t, t.TempDir())
	svc := src.NewSessionService()
	if _, err := svc.ImportChatGPT(zp, "个人", map[string]string{"g-p-0123456789abcdef": "备份项目"}, true); err != nil {
		t.Fatal(err)
	}
	backup, err := svc.BackupData(t.TempDir(), `{}`)
	if err != nil {
		t.Fatal(err)
	}

	dst, _, _, _ := newTestApp(t)
	// 目标上已经有同一账号、但项目名不同的 projects.json：已有的优先，恢复不报冲突
	other := t.TempDir()
	zp2, _ := writeChatGPTImageExport(t, other)
	dstSvc := dst.NewSessionService()
	if _, err := dstSvc.ImportChatGPT(zp2, "个人", map[string]string{"g-p-0123456789abcdef": "本机已有的名字"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := dstSvc.RestoreData(backup, `{}`); err != nil {
		t.Fatalf("恢复不该因项目名文件不同而失败: %v", err)
	}
	dstSvc.Refresh()
	rows := dstSvc.List(Filter{Agent: "chatgpt"})
	if len(rows) != 1 || filepath.Base(rows[0].Cwd) != "本机已有的名字" {
		t.Fatalf("恢复后的会话: %+v", rows)
	}

	// 只在备份里的图片能在目标上显示（没有被"不带图片"的本机批次盖掉）
	tr, err := dstSvc.Transcript(rows[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	var address string
	for _, p := range tr.Parts {
		if p.Kind == "image" {
			address = regexp.MustCompile(`/_chatgpt/[^)]+`).FindString(p.Text)
		}
	}
	if address == "" {
		t.Fatalf("恢复后应显示带图片的那一批（同样新的两份，带图的优先）: %+v", tr.Parts)
	}
	rec := httptest.NewRecorder()
	dst.ChatGPTAssets()(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest("GET", address, nil))
	if rec.Code != 200 || rec.Body.Len() != len(png) {
		t.Fatalf("恢复后的图片: %d %d", rec.Code, rec.Body.Len())
	}
}

// 备份里确实有图片文件：这是"备份不漏图"的直接证据。
func TestBackupArchiveContainsImageFiles(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	zp, _ := writeChatGPTImageExport(t, t.TempDir())
	svc := a.NewSessionService()
	if _, err := svc.ImportChatGPT(zp, "个人", nil, true); err != nil {
		t.Fatal(err)
	}
	backup, err := svc.BackupData(t.TempDir(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(backup)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var images, transcripts int
	for _, f := range zr.File {
		if matched, _ := regexp.MatchString(`^imports/chatgpt/[0-9a-f]{64}/assets/[0-9a-f]{64}\.png$`, f.Name); matched {
			images++
		}
		if matched, _ := regexp.MatchString(`^imports/chatgpt/.+\.jsonl$`, f.Name); matched {
			transcripts++
		}
	}
	if images != 1 || transcripts != 1 {
		t.Fatalf("备份里的图片 %d 个、正文 %d 个", images, transcripts)
	}
}
