package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeProfile(t *testing.T, roaming, name, content string, modified time.Time) string {
	t.Helper()
	dir := filepath.Join(roaming, name, localStorageRel)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "000003.log")
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(file, modified, modified); err != nil {
		t.Fatal(err)
	}
	return dir
}

// 从最近使用过的旧版本目录复制 localStorage；旧目录不被改动，无关目录不参与。
func TestMigrateLegacyLocalStorageCopiesNewestProfile(t *testing.T) {
	roaming, data := t.TempDir(), filepath.Join(t.TempDir(), webviewDirName)
	now := time.Now()
	writeProfile(t, roaming, "AgentDeck-v0.11.0-win-x64.exe", "old", now.Add(-48*time.Hour))
	newest := writeProfile(t, roaming, "AgentDeck-v0.12.1-win-x64.exe", "newest", now.Add(-time.Hour))
	writeProfile(t, roaming, "SomethingElse.exe", "unrelated", now)

	from, err := migrateLegacyLocalStorage(roaming, data)
	if err != nil || from != newest {
		t.Fatalf("来源 = %q, %v；期望 %q", from, err, newest)
	}
	got, err := os.ReadFile(filepath.Join(data, localStorageRel, "000003.log"))
	if err != nil || string(got) != "newest" {
		t.Fatalf("复制内容 = %q, %v", got, err)
	}
	if src, _ := os.ReadFile(filepath.Join(newest, "000003.log")); string(src) != "newest" {
		t.Fatal("旧目录被改动")
	}
	leftovers, _ := filepath.Glob(filepath.Join(data, "EBWebView", "Default", "Local Storage", ".leveldb-migrate-*"))
	if len(leftovers) != 0 {
		t.Fatalf("残留临时目录: %v", leftovers)
	}
}

// 新目录已经有偏好时不覆盖；没有旧目录时什么也不做。
func TestMigrateLegacyLocalStorageNeverOverwrites(t *testing.T) {
	roaming, data := t.TempDir(), filepath.Join(t.TempDir(), webviewDirName)
	writeProfile(t, roaming, "AgentDeck-v0.12.1-win-x64.exe", "legacy", time.Now())
	existing := filepath.Join(data, localStorageRel)
	if err := os.MkdirAll(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(existing, "000003.log"), []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	if from, err := migrateLegacyLocalStorage(roaming, data); err != nil || from != "" {
		t.Fatalf("已有偏好仍被迁移: %q %v", from, err)
	}
	if got, _ := os.ReadFile(filepath.Join(existing, "000003.log")); string(got) != "current" {
		t.Fatal("已有偏好被覆盖")
	}
	if from, err := migrateLegacyLocalStorage(t.TempDir(), filepath.Join(t.TempDir(), webviewDirName)); err != nil || from != "" {
		t.Fatalf("没有旧目录时 = %q %v", from, err)
	}
}

// 旧目录里出现子目录或链接时拒绝复制，并且不留下半份目标。
func TestMigrateLegacyLocalStorageRejectsNonRegularEntries(t *testing.T) {
	roaming, data := t.TempDir(), filepath.Join(t.TempDir(), webviewDirName)
	dir := writeProfile(t, roaming, "AgentDeck-v0.12.1-win-x64.exe", "legacy", time.Now())
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := migrateLegacyLocalStorage(roaming, data); err == nil {
		t.Fatal("含子目录的旧目录应被拒绝")
	}
	if _, err := os.Stat(filepath.Join(data, localStorageRel)); !os.IsNotExist(err) {
		t.Fatal("失败后留下了目标目录")
	}
}
