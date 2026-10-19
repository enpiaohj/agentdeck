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
	return newStorageAppIn(t, "data")
}

// newStorageAppIn 与 newStorageApp 相同，但数据根的目录名可以指定。固定相对
// 路径（storageLeafRelative）会影响用例的构造方式：「目标就是当前数据目录」
// 只有数据根正好落在 `…\AgentDeck\Data` 上时，才可能用「选中它的上级目录」
// 构造出来。
func newStorageAppIn(t *testing.T, name string) (*App, string) {
	t.Helper()
	t.Setenv("APPDATA", filepath.Join(t.TempDir(), "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "Local"))
	root := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
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

// pickParent 返回一个可迁移的选定目录与补全后的数据根。用户在界面上选的是
// 放在哪个目录下，实际数据根固定是它下面的 AgentDeck\Data。
func pickParent(t *testing.T) (picked, dest string) {
	t.Helper()
	picked = t.TempDir()
	return picked, filepath.Join(picked, storageDir, storageLeaf)
}

// assertMigrationRefused 确认一次被拒绝的迁移没留下任何痕迹：指针没写、
// 源数据没动。
func assertMigrationRefused(t *testing.T, root string) {
	t.Helper()
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

// TestStorageDestination 数据根固定取所选定目录下的 AgentDeck\Data；所选目录
// 本身就叫 AgentDeck 时只追加 Data（用户常常直接选到 `D:\software\AgentDeck`，
// 再套一层会得到 `…\AgentDeck\AgentDeck\Data`）；已经选到数据根本身时原样使用。
func TestStorageDestination(t *testing.T) {
	cases := []struct {
		name   string
		picked string
		want   string
	}{
		{"盘符根", `D:\`, `D:\AgentDeck\Data`},
		{"只有盘符", `D:`, `D:\AgentDeck\Data`},
		{"普通目录", `D:\software`, `D:\software\AgentDeck\Data`},
		{"带尾分隔符", `D:\software\`, `D:\software\AgentDeck\Data`},
		{"末级叫 AgentDeck", `D:\software\AgentDeck`, `D:\software\AgentDeck\Data`},
		{"末级叫 AgentDeck 带尾分隔符", `D:\software\AgentDeck\`, `D:\software\AgentDeck\Data`},
		{"末级大小写不敏感", `d:\software\agentdeck`, `d:\software\agentdeck\Data`},
		{"已经选到数据根本身", `D:\software\AgentDeck\Data`, `D:\software\AgentDeck\Data`},
		{"已经选到数据根本身带尾分隔符", `D:\software\AgentDeck\Data\`, `D:\software\AgentDeck\Data`},
		{"只是末级叫 Data", `D:\software\Data`, `D:\software\Data\AgentDeck\Data`},
		{"只是前缀相同", `D:\AgentDeckData`, `D:\AgentDeckData\AgentDeck\Data`},
		{"带空白", "  D:\\software  ", `D:\software\AgentDeck\Data`},
		{"相对路径不补成绝对", `relative\data`, `relative\data\AgentDeck\Data`},
		{"空路径", "   ", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := storageDestination(c.picked); got != c.want {
				t.Fatalf("storageDestination(%q) = %q，期望 %q", c.picked, got, c.want)
			}
		})
	}
}

// TestResolveDestinationReportsExistingDirectory 补全后的目录是否已经存在必须
// 如实报出来：不存在与空目录可用，已有内容的目录不可用（迁移不覆盖），当前
// 数据目录不可用。
func TestResolveDestinationReportsExistingDirectory(t *testing.T) {
	// 数据根正好落在 `…\AgentDeck\Data` 上，才能用「选中它的上级目录」构造出
	// 「目标就是当前数据目录」这一种选法。
	a, root := newStorageAppIn(t, storageLeafRelative)
	svc := a.NewSettingsService()
	picked := t.TempDir()
	dest := filepath.Join(picked, storageDir, storageLeaf)

	view := svc.ResolveDestination(picked)
	if view.Picked != picked || view.Dest != dest || view.Leaf != storageLeafRelative {
		t.Fatalf("补全结果 = %+v", view)
	}
	if view.Rule == "" || !strings.Contains(view.Rule, storageLeafRelative) {
		t.Fatalf("补全规则应如实说明：%q", view.Rule)
	}
	if view.Exists || !view.Empty || !view.Ready || view.Reason != "" {
		t.Fatalf("不存在的目录应当可用：%+v", view)
	}
	if view.Files != 0 || view.Bytes != 0 {
		t.Fatalf("不存在的目录不应有占用：%+v", view)
	}

	// 已存在且为空：仍然可用（上次迁移清理干净后留下的空目录）。
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if view = svc.ResolveDestination(picked); !view.Exists || !view.Empty || !view.Ready {
		t.Fatalf("空目录应当可用：%+v", view)
	}

	// 已有内容：不可用，并如实报出内容规模。
	writeFixture(t, filepath.Join(dest, "library", "history.db"), "旧数据")
	if view = svc.ResolveDestination(picked); view.Ready || view.Reason == "" {
		t.Fatalf("已有内容的目录应当拒绝：%+v", view)
	}
	if view.Exists && (view.Files != 1 || view.Bytes == 0) {
		t.Fatalf("应当报出已有内容的规模：%+v", view)
	}

	// 用户直接选到 AgentDeck 目录本身与选它的上级目录，落在同一个位置。
	direct := svc.ResolveDestination(filepath.Dir(dest))
	if direct.Dest != filepath.Clean(dest) {
		t.Fatalf("直接选中 AgentDeck 目录不应重复追加：%q", direct.Dest)
	}
	if again := svc.ResolveDestination(picked); again.Dest != direct.Dest {
		t.Fatalf("两种选法应落在同一位置：%q 与 %q", again.Dest, direct.Dest)
	}

	// 当前数据目录不能被当成目标：选到数据根本身、或选到它的上级目录，补全后
	// 都是当前数据目录本身。
	if current := svc.ResolveDestination(root); current.Ready || current.Reason == "" {
		t.Fatalf("当前的 %s 不能被当成迁移目标：%+v", root, current)
	}
	if parent := svc.ResolveDestination(filepath.Dir(root)); parent.Dest != root || parent.Ready {
		t.Fatalf("选到数据根的上级目录应补全成当前数据目录并拒绝：%+v", parent)
	}
}

// TestMigrateCopiesDataAndSwitchesRoot 迁移的成功路径：数据落在所选定目录下的
// AgentDeck 子目录，三个库用一致副本、其余文件逐个复制并校验、索引与运行标记
// 不复制、源目录一个字节不动、全部通过后才写数据位置指针。
func TestMigrateCopiesDataAndSwitchesRoot(t *testing.T) {
	a, root := newStorageApp(t)
	seedStorage(t, a, root)
	picked, dest := pickParent(t)
	svc := a.NewSettingsService()

	report, err := svc.Migrate(picked)
	if err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	if report.Dest != dest || report.Files == 0 {
		t.Fatalf("迁移报告 = %+v", report)
	}
	// 数据根固定是所选目录下的 AgentDeck\Data，而不是所选目录本身。
	if _, err := os.Stat(filepath.Join(picked, "marks.db")); !os.IsNotExist(err) {
		t.Fatalf("数据不应直接落在所选目录下：%s", picked)
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

	// 选定的目录下已经有补全后的数据目录且里面有东西：目标非空，必须整批拒绝
	// 而不是往里追加。用户重复选同一个位置时正是这种情形。
	busy := t.TempDir()
	writeFixture(t, filepath.Join(busy, storageDir, storageLeaf, "existing.txt"), "已有文件")
	cases := []struct {
		name   string
		picked string
	}{
		{"目标目录已存在且不为空", busy},
		{"目标就在源目录之内", filepath.Join(root, "sub")},
		{"相对路径", "relative/data"},
		{"空路径", "  "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := svc.Migrate(c.picked); err == nil {
				t.Fatalf("应拒绝：%s", c.picked)
			}
			assertMigrationRefused(t, root)
		})
	}
	if _, err := os.Stat(filepath.Join(busy, storageDir, storageLeaf, "marks.db")); !os.IsNotExist(err) {
		t.Fatal("拒绝的迁移不应在目标里留下文件")
	}
}

// TestMigrateRefusesCurrentLocation 数据根固定在 `…\AgentDeck\Data` 上，于是
// 「选中它的上级目录」与「选中更上一级目录」就是把当前数据目录、或当前数据
// 目录的上层当成目标：一次点击就可能拿正在使用的数据当迁移目标，两种都必须拒绝。
func TestMigrateRefusesCurrentLocation(t *testing.T) {
	t.Run("目标就是当前数据目录", func(t *testing.T) {
		a, root := newStorageAppIn(t, storageLeafRelative)
		seedStorage(t, a, root)
		svc := a.NewSettingsService()
		// root 是 `…\AgentDeck\Data`：选中它的上级目录，补全后正是 root。
		if _, err := svc.Migrate(filepath.Dir(root)); err == nil {
			t.Fatal("目标就是当前数据目录时应拒绝")
		}
		assertMigrationRefused(t, root)
	})

	t.Run("目标包含当前数据目录", func(t *testing.T) {
		// 数据根是 <tmp>\AgentDeck\Data\data：选中 <tmp> 会把 <tmp>\AgentDeck\Data
		// 当目标，而它正是当前数据目录的上层。
		a, root := newStorageAppIn(t, filepath.Join(storageLeafRelative, "data"))
		seedStorage(t, a, root)
		svc := a.NewSettingsService()
		if _, err := svc.Migrate(filepath.Dir(filepath.Dir(filepath.Dir(root)))); err == nil {
			t.Fatal("目标包含当前数据目录时应拒绝")
		}
		assertMigrationRefused(t, root)
	})
}

// TestMigrateRollsBackOwnFiles 复制中途失败时只删本次写出的文件：源目录
// 不变、配置不改、目标里不留半份数据。
func TestMigrateRollsBackOwnFiles(t *testing.T) {
	a, root := newStorageApp(t)
	seedStorage(t, a, root)
	picked, dest := pickParent(t)
	// 复制到最后一个文件时失败：此前写出的副本必须全部清掉。
	a.copyFileFn = func(src, dest string) (copyRecord, error) {
		if filepath.Base(src) == "operation.json" {
			return copyRecord{}, errors.New("模拟复制失败")
		}
		return copyFile(src, dest)
	}
	svc := a.NewSettingsService()
	_, err := svc.Migrate(picked)
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
	picked, dest := pickParent(t)
	svc := a.NewSettingsService()
	if _, err := svc.Migrate(picked); err != nil {
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
	picked, dest := pickParent(t)
	if _, err := a.NewSettingsService().Migrate(picked); err != nil {
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
	picked, dest := pickParent(t)
	if _, err := a.NewSettingsService().Migrate(picked); err != nil {
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
	picked, _ := pickParent(t)
	if _, err := a.NewSettingsService().Migrate(picked); err != nil {
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
