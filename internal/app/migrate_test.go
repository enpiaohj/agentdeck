package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/enpiaohj/agentdeck/internal/appconfig"
	"github.com/enpiaohj/agentdeck/internal/history"
	"github.com/enpiaohj/agentdeck/internal/marks"
	"github.com/enpiaohj/agentdeck/internal/projects"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// newStorageApp 装配一个数据全落在一个临时目录里的 App：迁移的源目录就是
// 这个目录，测试因此不必碰本机任何真实位置。
//
// 应用配置文件的位置由 %APPDATA% 决定，这里一并隔离，避免多个用例互相
// 看到对方写入的数据位置指针。%LOCALAPPDATA% 同样隔离：默认位置的本地
// 目录（索引、会话库、导入副本）不能被测试碰到。
func newStorageApp(t *testing.T) (*App, string) {
	t.Helper()
	t.Setenv("APPDATA", filepath.Join(t.TempDir(), "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "Local"))
	root := t.TempDir()
	a, err := New(Config{
		MarksPath:    filepath.Join(root, "marks.db"),
		ProjectsPath: filepath.Join(root, "projects.db"),
		IndexPath:    filepath.Join(root, "index.db"),
		LibraryPath:  filepath.Join(root, "library", "history.db"),
		ImportDir:    filepath.Join(root, "imports", "claude"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a, root
}

// newRestartedApp 模拟重启：按应用配置重新装配，数据落点因此切到指针所指
// 的位置。旧位置的清理只能在重启之后进行，测试据此覆盖真实流程。
func newRestartedApp(t *testing.T) *App {
	t.Helper()
	a, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a
}

// seedStorage 写入各类数据：标注、声明项目、永久会话库、删除备份、导入副本，
// 以及不参与迁移的索引与运行标记。
func seedStorage(t *testing.T, a *App, root string) {
	t.Helper()
	if err := a.marks.SetFavorite("claude:one", true); err != nil {
		t.Fatal(err)
	}
	if _, err := a.declared.Add(filepath.Join(t.TempDir(), "proj"), "示例项目"); err != nil {
		t.Fatal(err)
	}
	sess := sessions.Session{Agent: "claude", ID: "one", Title: "会话", Last: time.Now()}
	if err := a.history.Save(sess, sessions.Transcript{Parts: []sessions.Part{{Role: "user", Text: "正文"}}}); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(root, "library", "deletions", "job1", "operation.json"), `{"keys":[]}`)
	writeFixture(t, filepath.Join(root, "library", "data-recovery", "backup1", "marks.db"), "x")
	writeFixture(t, filepath.Join(root, "imports", "claude", "chat.jsonl"), "{}\n")
	// 不参与迁移的两项：全文索引与崩溃标记（session.lock 由 New 写出）。
	writeFixture(t, filepath.Join(root, "index.db"), "index")
	// 索引的旁文件与索引同属可重建项：迁移不复制，清理时要一并删除。
	writeFixture(t, filepath.Join(root, "index.db-journal"), "journal")
	// 不属于 AgentDeck 的文件：任何清理动作都不能碰它。
	writeFixture(t, filepath.Join(root, "keep.txt"), "用户自己的文件")
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestMigrateCopiesDataAndSwitchesRoot 迁移的成功路径：三个库用一致副本、
// 其余文件逐个复制并校验、索引与运行标记不复制、源目录一个字节不动、
// 全部通过后才写数据位置指针。
func TestMigrateCopiesDataAndSwitchesRoot(t *testing.T) {
	a, root := newStorageApp(t)
	seedStorage(t, a, root)
	dest := filepath.Join(t.TempDir(), "DeckData")
	svc := a.NewSettingsService()

	report, err := svc.Migrate(dest)
	if err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	if report.Dest != dest || report.Files == 0 {
		t.Fatalf("迁移报告 = %+v", report)
	}
	for _, rel := range []string{"marks.db", "projects.db", filepath.Join("library", "history.db"),
		filepath.Join("library", "deletions", "job1", "operation.json"),
		filepath.Join("library", "data-recovery", "backup1", "marks.db"),
		filepath.Join("imports", "claude", "chat.jsonl")} {
		if _, err := os.Stat(filepath.Join(dest, rel)); err != nil {
			t.Fatalf("目标缺少 %s：%v", rel, err)
		}
	}
	// 可重建与运行标记不复制。
	for _, rel := range []string{"index.db", "index.db-journal", "session.lock"} {
		if _, err := os.Stat(filepath.Join(dest, rel)); !os.IsNotExist(err) {
			t.Fatalf("%s 不应被复制", rel)
		}
	}
	if len(report.Skipped) == 0 {
		t.Fatal("跳过的项应当如实报出去")
	}
	// 三个副本必须各自通过结构校验。
	if _, err := history.ValidateBackup(filepath.Join(dest, "library", "history.db")); err != nil {
		t.Fatalf("会话库副本无效：%v", err)
	}
	if err := marks.Validate(filepath.Join(dest, "marks.db")); err != nil {
		t.Fatalf("标注库副本无效：%v", err)
	}
	if err := projects.ValidateBackup(filepath.Join(dest, "projects.db")); err != nil {
		t.Fatalf("项目声明副本无效：%v", err)
	}
	// 源目录不动：库、备份、导入副本与用户自己的文件都还在。
	for _, rel := range []string{"marks.db", "projects.db", "index.db", "keep.txt",
		filepath.Join("library", "history.db"), filepath.Join("library", "deletions", "job1", "operation.json")} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("源目录的 %s 不见了：%v", rel, err)
		}
	}
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataRoot != dest {
		t.Fatalf("数据位置指针 = %q，期望 %q", cfg.DataRoot, dest)
	}
	if !containsPath(cfg.PreviousDirs, root) {
		t.Fatalf("旧位置未记录：%v", cfg.PreviousDirs)
	}
	view := svc.Storage()
	if view.UsingDefault || view.Root != dest {
		t.Fatalf("状态 = %+v", view)
	}
	if len(view.PrevDirs) == 0 || view.PrevBytes == 0 {
		t.Fatalf("旧位置应如实列出：%+v", view)
	}
	if !view.PrevInUse {
		t.Fatal("尚未重启时，旧位置仍是本次运行使用的目录")
	}
	// 重启后数据落点切到新位置，旧位置不再是当前目录。
	restarted := newRestartedApp(t)
	after := restarted.NewSettingsService().Storage()
	if after.UsingDefault || after.Root != dest {
		t.Fatalf("重启后的状态 = %+v", after)
	}
	if after.PrevInUse {
		t.Fatal("重启后旧位置不再被本次运行使用")
	}
	if _, err := os.Stat(filepath.Join(dest, "marks.db")); err != nil {
		t.Fatalf("重启后应按新位置打开标注库：%v", err)
	}
}

// TestMigrateRefusesUnsafeDestination 前置校验不通过时必须整批拒绝：不写
// 指针、不复制任何文件、源目录不动。
func TestMigrateRefusesUnsafeDestination(t *testing.T) {
	a, root := newStorageApp(t)
	seedStorage(t, a, root)
	svc := a.NewSettingsService()

	busy := filepath.Join(t.TempDir(), "notempty")
	writeFixture(t, filepath.Join(busy, "existing.txt"), "已有文件")
	cases := []struct {
		name string
		dest string
	}{
		{"目标目录非空", busy},
		{"目标就在源目录之内", filepath.Join(root, "sub")},
		{"目标就是源目录", root},
		{"目标包含源目录", filepath.Dir(root)},
		{"相对路径", "relative/data"},
		{"空路径", "  "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := svc.Migrate(c.dest); err == nil {
				t.Fatalf("应拒绝：%s", c.dest)
			}
			cfg, err := appconfig.Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.DataRoot != "" {
				t.Fatalf("被拒绝的迁移写入了指针：%q", cfg.DataRoot)
			}
			if _, err := os.Stat(filepath.Join(root, "marks.db")); err != nil {
				t.Fatalf("源目录被动过：%v", err)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(busy, "marks.db")); !os.IsNotExist(err) {
		t.Fatal("拒绝的迁移不应在目标里留下文件")
	}
}

// TestMigrateRollsBackOwnFiles 复制中途失败时只删本次写出的文件：源目录
// 不变、配置不改、目标里不留半份数据。
func TestMigrateRollsBackOwnFiles(t *testing.T) {
	a, root := newStorageApp(t)
	seedStorage(t, a, root)
	dest := filepath.Join(t.TempDir(), "DeckData")
	// 复制到最后一个文件时失败：此前写出的副本必须全部清掉。
	a.copyFileFn = func(src, dest string) (copyRecord, error) {
		if filepath.Base(src) == "operation.json" {
			return copyRecord{}, errors.New("模拟复制失败")
		}
		return copyFile(src, dest)
	}
	svc := a.NewSettingsService()
	_, err := svc.Migrate(dest)
	if err == nil {
		t.Fatal("复制失败时迁移应当报错")
	}
	if !strings.Contains(err.Error(), "已清理") {
		t.Fatalf("错误应说明已回滚：%v", err)
	}
	if files := filesUnder(dest); len(files) != 0 {
		t.Fatalf("目标里残留了 %d 个文件", len(files))
	}
	for _, rel := range []string{"marks.db", "projects.db", "index.db", "keep.txt",
		filepath.Join("library", "history.db"), filepath.Join("imports", "claude", "chat.jsonl")} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("失败后源目录的 %s 不见了：%v", rel, err)
		}
	}
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataRoot != "" || len(cfg.PreviousDirs) != 0 {
		t.Fatalf("失败后不应写指针：%+v", cfg)
	}
}

// TestCleanupOldRemovesOnlyOwnFiles 清理只删旧位置里属于 AgentDeck 的文件，
// 用户自己的文件与拖着的非空目录都留下。
func TestCleanupOldRemovesOnlyOwnFiles(t *testing.T) {
	a, root := newStorageApp(t)
	seedStorage(t, a, root)
	dest := filepath.Join(t.TempDir(), "DeckData")
	svc := a.NewSettingsService()
	if _, err := svc.Migrate(dest); err != nil {
		t.Fatal(err)
	}
	// 旧位置里再放一个用户自己的目录，清理不得碰它。
	writeFixture(t, filepath.Join(root, "我自己的目录", "note.txt"), "保留")
	// 旧实例必须先退出：它持有的库文件句柄会挡住删除，真实流程也正是
	// 「迁移 → 重启 → 清理」。
	a.Close()

	// 清理只能在重启之后：重新装配，数据落点已切到新位置。
	restarted := newRestartedApp(t)
	after := restarted.NewSettingsService()
	plan, err := after.CleanupPreview()
	if err != nil {
		t.Fatal(err)
	}
	if plan.Token == "" || plan.Files == 0 || plan.Bytes == 0 {
		t.Fatalf("预览 = %+v", plan)
	}
	if len(plan.Other) == 0 {
		t.Fatal("不属于 AgentDeck 的条目应当列出")
	}
	if _, err := after.CleanupOld("wrong-token"); err == nil {
		t.Fatal("token 不匹配应整批拒绝")
	}
	report, err := after.CleanupOld(plan.Token)
	if err != nil {
		t.Fatalf("清理失败：%v", err)
	}
	if report.Files != plan.Files || report.Bytes != plan.Bytes {
		t.Fatalf("清理结果 = %+v，预览 = %+v", report, plan)
	}
	for _, rel := range []string{"marks.db", "projects.db", filepath.Join("library", "history.db"),
		filepath.Join("library", "deletions", "job1", "operation.json"), filepath.Join("imports", "claude", "chat.jsonl"),
		"index.db", "index.db-journal"} {
		if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Fatalf("旧位置的 %s 应已删除", rel)
		}
	}
	for _, rel := range []string{"keep.txt", filepath.Join("我自己的目录", "note.txt")} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("不得删除 %s：%v", rel, err)
		}
	}
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.PreviousDirs) != 0 {
		t.Fatalf("清理后旧位置应不再提示：%v", cfg.PreviousDirs)
	}
	// 新位置不受影响。
	if _, err := os.Stat(filepath.Join(dest, "marks.db")); err != nil {
		t.Fatalf("新位置被动过：%v", err)
	}
}

// TestCleanupRefusesCurrentLocation 旧位置与当前数据目录相同时拒绝清理：
// 否则一次点击就删掉正在使用的数据。
func TestCleanupRefusesCurrentLocation(t *testing.T) {
	a, root := newStorageApp(t)
	seedStorage(t, a, root)
	a.settingsMu.Lock()
	cfg := a.settings
	cfg.DataRoot = root
	cfg.PreviousDirs = []string{root}
	a.settings = cfg
	a.settingsMu.Unlock()

	svc := a.NewSettingsService()
	if _, err := svc.CleanupPreview(); err == nil {
		t.Fatal("旧位置就是当前位置时应拒绝")
	}
	if _, err := os.Stat(filepath.Join(root, "marks.db")); err != nil {
		t.Fatalf("拒绝时必须什么都没删：%v", err)
	}
}

// TestResetRootKeepsPointerAndData 改回默认位置只改指针：刚离开的位置数据
// 仍在，并记入旧位置清单，供用户确认后再清理。
func TestResetRootKeepsPointerAndData(t *testing.T) {
	a, root := newStorageApp(t)
	seedStorage(t, a, root)
	dest := filepath.Join(t.TempDir(), "DeckData")
	if _, err := a.NewSettingsService().Migrate(dest); err != nil {
		t.Fatal(err)
	}
	// 重启后当前数据目录是 dest，改回默认位置就是离开它。
	svc := newRestartedApp(t).NewSettingsService()
	if err := svc.ResetRoot(); err != nil {
		t.Fatal(err)
	}
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataRoot != "" {
		t.Fatalf("数据位置应为默认：%q", cfg.DataRoot)
	}
	for _, dir := range []string{dest, root} {
		if !containsPath(cfg.PreviousDirs, dir) {
			t.Fatalf("旧位置清单应含 %s：%v", dir, cfg.PreviousDirs)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "marks.db")); err != nil {
		t.Fatalf("改回默认位置不应搬动数据：%v", err)
	}
}

// TestResetRootDropsCurrentLocationFromPrevious 改回默认位置后，原先记下的
// 默认位置目录又成了当前目录：它必须从旧位置清单里去掉，否则「清理旧位置
// 数据」会因为「旧位置就是当前数据目录」被整批拒绝，用户反而清不掉迁走后
// 留在目标位置的那一份。
func TestResetRootDropsCurrentLocationFromPrevious(t *testing.T) {
	a, root := newStorageApp(t)
	seedStorage(t, a, root)
	dest := filepath.Join(t.TempDir(), "DeckData")
	if _, err := a.NewSettingsService().Migrate(dest); err != nil {
		t.Fatal(err)
	}
	defaults, err := appconfig.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	// 造出「曾经在默认位置、迁到 dest、又改回默认位置」的清单。
	a.settingsMu.Lock()
	cfg := a.settings
	cfg.DataRoot = dest
	cfg.PreviousDirs = []string{defaults.ConfigDir, defaults.LocalDir, root}
	a.settings = cfg
	a.settingsMu.Unlock()
	if err := appconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}
	// 先正常退出，再按指针重新装配，等价于重启。
	a.Close()
	restarted := newRestartedApp(t)
	svc := restarted.NewSettingsService()
	if err := svc.ResetRoot(); err != nil {
		t.Fatalf("改回默认位置失败：%v", err)
	}
	got, err := appconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.DataRoot != "" {
		t.Fatalf("数据位置应为默认：%q", got.DataRoot)
	}
	for _, dir := range []string{defaults.ConfigDir, defaults.LocalDir} {
		if containsPath(got.PreviousDirs, dir) {
			t.Fatalf("默认位置又成了当前目录，不该留在旧位置清单：%v", got.PreviousDirs)
		}
	}
	for _, dir := range []string{root, dest} {
		if !containsPath(got.PreviousDirs, dir) {
			t.Fatalf("旧位置清单应含 %s：%v", dir, got.PreviousDirs)
		}
	}
	// 清理仍然可用：能列出迁走后留在目标位置的那一份数据。改回默认位置后
	// 必须再重启一次，本次运行才真的不再使用 dest（重启前连带清理的拒绝是
	// 保护，见 cleanupPlan）。
	restarted2 := newRestartedApp(t)
	after := restarted2.NewSettingsService()
	view := after.Storage()
	if !view.UsingDefault || view.PrevInUse {
		t.Fatalf("重启后应使用默认位置且旧位置不再在用：%+v", view)
	}
	plan, err := after.CleanupPreview()
	if err != nil {
		t.Fatalf("清理预览应当可用：%v", err)
	}
	if plan.Files == 0 || plan.Bytes == 0 {
		t.Fatalf("留在旧位置的数据应可清理：%+v", plan)
	}
	if !containsPath(plan.Dirs, dest) {
		t.Fatalf("清理清单应含迁移目标位置 %s：%v", dest, plan.Dirs)
	}
}

// TestCleanupPrunesEmptyLocation 旧位置只剩 AgentDeck 自己的数据时，清理要
// 把索引旁文件一并删掉并把空目录收干净。只删 index.db 会让用户点完「清理旧
// 位置数据」后仍看到一个留着日志文件的旧目录。
func TestCleanupPrunesEmptyLocation(t *testing.T) {
	a, root := newStorageApp(t)
	seedStorage(t, a, root)
	dest := filepath.Join(t.TempDir(), "DeckData")
	if _, err := a.NewSettingsService().Migrate(dest); err != nil {
		t.Fatal(err)
	}
	// 去掉用户自己的文件：旧位置这时只剩 AgentDeck 的数据。
	if err := os.Remove(filepath.Join(root, "keep.txt")); err != nil {
		t.Fatal(err)
	}
	a.Close()

	after := newRestartedApp(t).NewSettingsService()
	plan, err := after.CleanupPreview()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.db", "index.db-journal"} {
		found := false
		for _, dir := range plan.Dirs {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				found = true
			}
		}
		if !found {
			t.Fatalf("预览应含 %s（%s）", name, root)
		}
	}
	if _, err := after.CleanupOld(plan.Token); err != nil {
		t.Fatalf("清理失败：%v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("旧位置应已收干净：%v", err)
	}
}

// TestRestartWithoutCallback 未注入重启能力时如实报错，界面据此提示手动重启。
func TestRestartWithoutCallback(t *testing.T) {
	a, _ := newStorageApp(t)
	if err := a.NewSettingsService().Restart(); err == nil || !strings.Contains(err.Error(), "手动") {
		t.Fatalf("重启错误 = %v", err)
	}
}
