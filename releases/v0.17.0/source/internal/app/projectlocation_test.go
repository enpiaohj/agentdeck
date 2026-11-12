package app

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

func TestResumeLocationResolution(t *testing.T) {
	a, _, clip, ran := newTestApp(t)
	root := t.TempDir()
	old := filepath.Join(root, "missing")
	target := t.TempDir()
	sess := sessions.Session{Agent: "claude", ID: "location1", Cwd: old, Resume: "claude --resume location1"}
	a.setList([]sessions.Session{sess})
	r := &ResumeService{a: a}
	p := &ProjectService{a: a}
	preview, err := r.Prepare("claude:location1", "")
	if err != nil || !preview.NeedsLocation || preview.Directory != old {
		t.Fatalf("missing = %+v %v", preview, err)
	}
	if err := r.OpenInTerminal("claude:location1"); err == nil || len(*ran) != 0 {
		t.Fatal("launched missing path")
	}
	if err := r.CopyAt("claude:location1", target); err != nil {
		t.Fatal(err)
	}
	if len(*clip) != 1 || !strings.Contains((*clip)[0], target) {
		t.Fatalf("copy = %v", *clip)
	}
	if got, _ := p.Location(old); got != old {
		t.Fatal("one-time selection persisted")
	}
	if err := p.SetLocation(old, target); err != nil {
		t.Fatal(err)
	}
	if err := r.OpenInTerminal("claude:location1"); err != nil {
		t.Fatal(err)
	}
	if len(*ran) != 1 {
		t.Fatal("runner not called")
	}
	if err := p.Open(old); err != nil {
		t.Fatal(err)
	}
	openCommand := "explorer"
	if runtime.GOOS == "darwin" {
		openCommand = "open"
	} else if runtime.GOOS != "windows" {
		openCommand = "xdg-open"
	}
	if len(*ran) != 2 || (*ran)[1] != openCommand+" "+target {
		t.Fatalf("open project = %v", *ran)
	}
	if cmd, err := r.PreviewCommand(old, "claude"); err != nil || !strings.Contains(cmd, target) {
		t.Fatalf("new session preview = %q %v", cmd, err)
	}
	_, byKey, _ := a.snapshot()
	if byKey["claude:location1"].Cwd != old {
		t.Fatal("original cwd changed")
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	preview, err = r.Prepare("claude:location1", "")
	if err != nil || !preview.NeedsLocation || preview.Directory != target {
		t.Fatalf("stale mapping = %+v %v", preview, err)
	}
	if err := r.OpenInTerminal("claude:location1"); err == nil || len(*ran) != 2 {
		t.Fatal("launched stale mapping")
	}
}

func TestResumeWithoutDirectory(t *testing.T) {
	a, _, clip, ran := newTestApp(t)
	a.setList([]sessions.Session{{Agent: "codex", ID: "no-directory"}})
	r := &ResumeService{a: a}
	if err := r.CopyAt("codex:no-directory", ""); err != nil {
		t.Fatal(err)
	}
	if len(*clip) != 1 || (*clip)[0] != "codex resume no-directory" {
		t.Fatalf("clipboard = %v", *clip)
	}
	if err := r.OpenAt("codex:no-directory", ""); err == nil || len(*ran) != 0 {
		t.Fatal("started without directory")
	}
	if err := r.OpenAt("codex:no-directory", t.TempDir()); err != nil || len(*ran) != 1 {
		t.Fatalf("selected directory: %v", err)
	}
}

func TestNewSessionAtDoesNotRemapTarget(t *testing.T) {
	a, _, _, ran := newTestApp(t)
	original, target, other := t.TempDir(), t.TempDir(), t.TempDir()
	if _, err := a.declared.AddWithAgent(original, "Project", "codex"); err != nil {
		t.Fatal(err)
	}
	p := &ProjectService{a: a}
	if err := p.SetLocation(original, target); err != nil {
		t.Fatal(err)
	}
	if err := p.SetLocation(target, other); err != nil {
		t.Fatal(err)
	}
	r := &ResumeService{a: a}
	command, err := r.PreviewCommandAt(original, target, "")
	if err != nil || !strings.Contains(command, target) || strings.Contains(command, other) || !strings.HasSuffix(command, "; codex") {
		t.Fatalf("preview = %q %v", command, err)
	}
	if err := r.NewSessionAt(original, target, ""); err != nil || len(*ran) != 1 {
		t.Fatalf("launch = %v %v", *ran, err)
	}
}

func TestOpenAtResultReportsStartedWarning(t *testing.T) {
	a, _, _, ran := newTestApp(t)
	dir := t.TempDir()
	a.setList([]sessions.Session{{Agent: "codex", ID: "warning1", Cwd: dir, Path: filepath.Join(dir, "rollout.jsonl")}})
	if err := (&MarkService{a: a}).SetTitle("codex:warning1", "名称"); err != nil {
		t.Fatal(err)
	}
	old := codexNameCurrent
	codexNameCurrent = func(string, string) bool { return false }
	t.Cleanup(func() { codexNameCurrent = old })
	a.codexName = func(string, string) error { return errors.New("名称服务不可用") }
	result, err := (&ResumeService{a: a}).OpenAtResult("codex:warning1", "")
	if err != nil || !strings.Contains(result.Warning, "名称服务不可用") || len(*ran) != 1 {
		t.Fatalf("result = %+v %v, launches = %v", result, err, *ran)
	}
	a.cfg.Runner = func(string, ...string) error { return errors.New("终端不可用") }
	result, err = (&ResumeService{a: a}).OpenAtResult("codex:warning1", "")
	if err == nil || result.Warning != "" {
		t.Fatalf("failure = %+v %v", result, err)
	}
}

func TestResumeLocationCommandConstraints(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	original, target := t.TempDir(), t.TempDir()
	cursor := sessions.Session{Agent: "cursor", ID: "cursor1", Cwd: original}
	omp := sessions.Session{Agent: "omp", ID: "omp1", Cwd: original, Path: filepath.Join(t.TempDir(), ".omp", "profiles", "work", "agent", "sessions", "project", "omp1.jsonl")}
	unsafe := sessions.Session{Agent: "claude", ID: "invalid id", Cwd: original, Resume: "old command"}
	a.setList([]sessions.Session{cursor, omp, unsafe})
	r := &ResumeService{a: a}
	if _, err := r.Prepare("cursor:cursor1", target); err == nil {
		t.Fatal("cursor relocation accepted")
	}
	if err := (&ProjectService{a: a}).SetLocation(original, target); err != nil {
		t.Fatal(err)
	}
	if preview, err := r.Prepare("cursor:cursor1", ""); err != nil || !preview.NeedsLocation || preview.Notice == "" {
		t.Fatalf("cursor must allow choosing original: %+v %v", preview, err)
	}
	if preview, err := r.Prepare("cursor:cursor1", original); err != nil || preview.NeedsLocation || preview.Command == "" {
		t.Fatalf("cursor original = %+v %v", preview, err)
	}
	if preview, err := r.Prepare("claude:invalid id", target); err == nil || preview.Command != "" {
		t.Fatalf("invalid command gained name: %+v %v", preview, err)
	}
	preview, err := r.Prepare("omp:omp1", target)
	if err != nil || !strings.Contains(preview.Command, "--profile work") || !strings.Contains(preview.Command, "-ErrorAction Stop") {
		t.Fatalf("omp = %+v %v", preview, err)
	}
}
