package session

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Nox1KCL/Agys/internal/config"
)

func TestCollectSessionsAndTitles(t *testing.T) {
	root := t.TempDir()
	cfg := config.FromRoot(root)

	idAnn := "05551c4c-c30f-41c0-8f9b-be6798f23c6f"
	idKI := "157446c4-09ea-4cba-b0af-b8e688a8a482"
	idMD := "283f4905-0704-4345-9b56-5d46f7359469"
	idTranscript := "3bcf41e6-a55d-4df0-9652-e309b4da6817"
	idEmpty := "48eb02d7-b10d-4d1d-8296-081590a5e4ee"

	for _, dir := range []string{
		filepath.Join(cfg.BrainDir, idAnn),
		filepath.Join(cfg.BrainDir, idKI),
		filepath.Join(cfg.BrainDir, idMD),
		filepath.Join(cfg.BrainDir, idTranscript, ".system_generated", "logs"),
		filepath.Join(cfg.BrainDir, idEmpty),
		filepath.Join(cfg.BrainDir, "tempmediaStorage"),
		filepath.Join(cfg.KnowledgeDir, "ki_cli_design"),
		cfg.AnnotationsDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", dir, err)
		}
	}

	// Non-directory lock file in knowledge/ should be ignored cleanly.
	if err := os.WriteFile(filepath.Join(cfg.KnowledgeDir, "knowledge.lock"), nil, 0o600); err != nil {
		t.Fatalf("write lock file: %v", err)
	}

	// Annotation title takes priority over markdown/transcript.
	annContent := `title:"Annotation Title \"Quoted\"" archived:false last_user_view_time:{seconds:1794494778 nanos:510000000} pinned:true`
	if err := os.WriteFile(filepath.Join(cfg.AnnotationsDir, idAnn+".pbtxt"), []byte(annContent), 0o600); err != nil {
		t.Fatalf("write annotation: %v", err)
	}

	// Knowledge item linked to idAnn and idKI.
	kiMeta := `{
		"title": "KI Architecture Summary",
		"summary": "Reusable knowledge item summary",
		"updatedAt": "2026-11-10T10:00:00Z",
		"references": [
			{"type": "conversation_id", "id": "` + idAnn + `"},
			{"type": "conversation_id", "id": "` + idKI + `"}
		]
	}`
	if err := os.WriteFile(filepath.Join(cfg.KnowledgeDir, "ki_cli_design", "metadata.json"), []byte(kiMeta), 0o600); err != nil {
		t.Fatalf("write ki metadata: %v", err)
	}

	// Markdown H1 fallback from task.md.
	if err := os.WriteFile(filepath.Join(cfg.BrainDir, idMD, "task.md"), []byte("# Task List Title\n- [x] Done\n"), 0o600); err != nil {
		t.Fatalf("write task.md: %v", err)
	}

	// Transcript USER_INPUT fallback.
	transcript := `{"step_index":0,"type":"USER_INPUT","created_at":"2026-10-01T06:50:58Z","content":"<USER_REQUEST>\nFix authentication redirect bug\n</USER_REQUEST>"}` + "\n"
	if err := os.WriteFile(filepath.Join(cfg.BrainDir, idTranscript, ".system_generated", "logs", "transcript.jsonl"), []byte(transcript), 0o600); err != nil {
		t.Fatalf("write transcript.jsonl: %v", err)
	}

	sessions, err := Collect(cfg)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if len(sessions) != 5 {
		t.Fatalf("got %d sessions, want 5 (tempmediaStorage must be excluded)", len(sessions))
	}

	titles := TitlesMap(sessions)
	wantTitles := map[string]struct {
		title  string
		source TitleSource
	}{
		idAnn:        {title: `Annotation Title "Quoted"`, source: TitleSourceAnnotation},
		idKI:         {title: "KI Architecture Summary", source: TitleSourceKnowledge},
		idMD:         {title: "Task List Title", source: TitleSourceMarkdown},
		idTranscript: {title: "Fix authentication redirect bug", source: TitleSourceTranscript},
		idEmpty:      {title: untitledSessionLabel, source: TitleSourceNone},
	}

	for id, want := range wantTitles {
		s, ok := sessions[id]
		if !ok {
			t.Fatalf("missing session %s", id)
		}
		if titles[id] != want.title || s.Title != want.title {
			t.Fatalf("session %s title = %q, want %q", id, s.Title, want.title)
		}
		if s.TitleSource != want.source {
			t.Fatalf("session %s source = %q, want %q", id, s.TitleSource, want.source)
		}
	}

	if !sessions[idAnn].Pinned || sessions[idAnn].Archived {
		t.Fatalf("unexpected annotation flags for %s: pinned=%v archived=%v", idAnn, sessions[idAnn].Pinned, sessions[idAnn].Archived)
	}
	if len(sessions[idAnn].KnowledgeItems) != 1 || sessions[idAnn].KnowledgeItems[0].Summary != "Reusable knowledge item summary" {
		t.Fatalf("expected linked KnowledgeItem on %s, got %+v", idAnn, sessions[idAnn].KnowledgeItems)
	}
}

func TestCollectUsesMaxInnerFileMtime(t *testing.T) {
	root := t.TempDir()
	cfg := config.FromRoot(root)

	idOlderDirNewerFile := "05551c4c-c30f-41c0-8f9b-be6798f23c6f"
	idNewerDirOlderFile := "157446c4-09ea-4cba-b0af-b8e688a8a482"
	idEmptyDir := "283f4905-0704-4345-9b56-5d46f7359469"

	dirA := filepath.Join(cfg.BrainDir, idOlderDirNewerFile)
	subDirA := filepath.Join(dirA, ".system_generated", "logs")
	fileA1 := filepath.Join(dirA, "task.md")
	fileA2 := filepath.Join(subDirA, "transcript.jsonl")

	dirB := filepath.Join(cfg.BrainDir, idNewerDirOlderFile)
	fileB := filepath.Join(dirB, "notes.md")

	dirC := filepath.Join(cfg.BrainDir, idEmptyDir)

	for _, d := range []string{subDirA, dirB, dirC, cfg.AnnotationsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", d, err)
		}
	}

	for _, f := range []string{fileA1, fileA2, fileB} {
		if err := os.WriteFile(f, []byte("# Title\n"), 0o600); err != nil {
			t.Fatalf("write %q: %v", f, err)
		}
	}

	base := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	tFileOldest := base
	tDirOlder := base.Add(1 * time.Hour)
	tDirMiddle := base.Add(2 * time.Hour)
	tDirNewer := base.Add(5 * time.Hour)
	tFileNewest := base.Add(10 * time.Hour)

	// Session A: directory itself is older, one file is old, another nested file is newest.
	if err := os.Chtimes(fileA1, tFileOldest, tFileOldest); err != nil {
		t.Fatalf("chtimes fileA1: %v", err)
	}
	if err := os.Chtimes(fileA2, tFileNewest, tFileNewest); err != nil {
		t.Fatalf("chtimes fileA2: %v", err)
	}
	if err := os.Chtimes(subDirA, tDirOlder, tDirOlder); err != nil {
		t.Fatalf("chtimes subDirA: %v", err)
	}
	if err := os.Chtimes(dirA, tDirOlder, tDirOlder); err != nil {
		t.Fatalf("chtimes dirA: %v", err)
	}

	// Session B: directory mtime and annotation last_user_view_time are newer,
	// but its only inner file is oldest.
	if err := os.Chtimes(fileB, tFileOldest, tFileOldest); err != nil {
		t.Fatalf("chtimes fileB: %v", err)
	}
	if err := os.Chtimes(dirB, tDirNewer, tDirNewer); err != nil {
		t.Fatalf("chtimes dirB: %v", err)
	}
	annFuture := `title:"Viewed Recently" last_user_view_time:{seconds:1800000000 nanos:0}`
	if err := os.WriteFile(filepath.Join(cfg.AnnotationsDir, idNewerDirOlderFile+".pbtxt"), []byte(annFuture), 0o600); err != nil {
		t.Fatalf("write annotation: %v", err)
	}

	// Session C: empty directory falls back to directory ModTime.
	if err := os.Chtimes(dirC, tDirMiddle, tDirMiddle); err != nil {
		t.Fatalf("chtimes dirC: %v", err)
	}

	sessions, err := Collect(cfg)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	ordered := Sorted(sessions)
	if len(ordered) != 3 {
		t.Fatalf("got %d ordered sessions, want 3", len(ordered))
	}

	if ordered[0].ID != idOlderDirNewerFile || !ordered[0].UpdatedAt.Equal(tFileNewest) {
		t.Fatalf("first session = (%s, %v), want (%s, %v)", ordered[0].ID, ordered[0].UpdatedAt, idOlderDirNewerFile, tFileNewest)
	}
	if ordered[1].ID != idEmptyDir || !ordered[1].UpdatedAt.Equal(tDirMiddle) {
		t.Fatalf("second session = (%s, %v), want (%s, %v)", ordered[1].ID, ordered[1].UpdatedAt, idEmptyDir, tDirMiddle)
	}
	if ordered[2].ID != idNewerDirOlderFile || !ordered[2].UpdatedAt.Equal(tFileOldest) {
		t.Fatalf("third session = (%s, %v), want (%s, %v)", ordered[2].ID, ordered[2].UpdatedAt, idNewerDirOlderFile, tFileOldest)
	}
}

func TestCollectBindsSubSessionsToParent(t *testing.T) {
	root := t.TempDir()
	cfg := config.FromRoot(root)

	parentID := "4f4e604e-a528-4f39-8f88-8daefc3248cb"
	childByDescriptorID := "157446c4-09ea-4cba-b0af-b8e688a8a482"
	childByTranscriptID := "56ef5e8f-641e-4581-9575-f4189c8102f5"

	subagentsDir := filepath.Join(cfg.BrainDir, parentID, ".system_generated", "subagents")
	child1Dir := filepath.Join(cfg.BrainDir, childByDescriptorID)
	child2Logs := filepath.Join(cfg.BrainDir, childByTranscriptID, ".system_generated", "logs")

	for _, d := range []string{subagentsDir, child1Dir, child2Logs, cfg.AnnotationsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", d, err)
		}
	}

	if err := os.WriteFile(filepath.Join(cfg.AnnotationsDir, parentID+".pbtxt"), []byte(`title:"Parent Task"`), 0o600); err != nil {
		t.Fatalf("write parent annotation: %v", err)
	}

	descriptor := `{"conversationId":"` + childByDescriptorID + `","subagentDescriptor":{"typeName":"code-fixer","role":"WifiDirect fixes"},"workspaceUris":["file:///tmp/app"]}`
	if err := os.WriteFile(filepath.Join(subagentsDir, childByDescriptorID+".json"), []byte(descriptor), 0o600); err != nil {
		t.Fatalf("write descriptor: %v", err)
	}

	sysTranscript := `{"step_index":0,"source":"SYSTEM","type":"SYSTEM_MESSAGE","content":"[Message] timestamp=2026-11-03T17:18:00Z sender=` + parentID + ` content=Check logs"}` + "\n"
	if err := os.WriteFile(filepath.Join(child2Logs, "transcript.jsonl"), []byte(sysTranscript), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	sessions, err := Collect(cfg)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if len(sessions) != 3 {
		t.Fatalf("got %d total sessions, want 3", len(sessions))
	}

	roots := SortedRoots(sessions)
	if len(roots) != 1 || roots[0].ID != parentID {
		t.Fatalf("got %d root sessions (%+v), want only %s", len(roots), roots, parentID)
	}

	if len(roots[0].SubSessions) != 2 {
		t.Fatalf("got %d subsessions on parent, want 2", len(roots[0].SubSessions))
	}

	child1 := sessions[childByDescriptorID]
	if child1.ParentID != parentID || child1.Title != "WifiDirect fixes" || child1.TitleSource != TitleSourceSubagent || child1.SubagentType != "code-fixer" {
		t.Fatalf("unexpected child1 metadata: %+v", child1)
	}

	child2 := sessions[childByTranscriptID]
	if child2.ParentID != parentID {
		t.Fatalf("expected child2 ParentID %s via transcript sender, got %q", parentID, child2.ParentID)
	}
}

func TestNewResumeCmd(t *testing.T) {
	if _, err := NewResumeCmd("not-a-uuid"); err == nil {
		t.Fatal("expected error for invalid UUID")
	}

	validID := "083f7f21-26a2-4e44-b6ea-fa7fc1795a2f"
	cmd, err := NewResumeCmd(validID)
	if err != nil {
		t.Fatalf("NewResumeCmd failed: %v", err)
	}
	if len(cmd.Args) != 2 || cmd.Args[1] != "--conversation="+validID {
		t.Fatalf("unexpected cmd args: %v", cmd.Args)
	}

	// Verify auto-resolving binary via XDG_BIN_HOME and cmd.Dir fallback from missing subdir.
	home := t.TempDir()
	xdgBin := filepath.Join(home, "custom-bin")
	if err := os.MkdirAll(xdgBin, 0o755); err != nil {
		t.Fatalf("mkdir xdgBin: %v", err)
	}
	fakeAgy := filepath.Join(xdgBin, "agy")
	if err := os.WriteFile(fakeAgy, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake agy: %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("XDG_BIN_HOME", xdgBin)

	projDir := filepath.Join(home, "proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir projDir: %v", err)
	}

	cmdWithDir, err := NewResumeCmdWithDir(validID, "~/proj/missing-subdir")
	if err != nil {
		t.Fatalf("NewResumeCmdWithDir failed: %v", err)
	}
	if cmdWithDir.Path != fakeAgy {
		t.Fatalf("resolved binary = %q, want %q", cmdWithDir.Path, fakeAgy)
	}
	if cmdWithDir.Dir != projDir {
		t.Fatalf("cmd.Dir = %q, want %q", cmdWithDir.Dir, projDir)
	}
}

func TestCollectExtractsPromptsAndWorkDir(t *testing.T) {
	root := t.TempDir()
	cfg := config.FromRoot(root)

	id := "083f7f21-26a2-4e44-b6ea-fa7fc1795a2f"
	logsDir := filepath.Join(cfg.BrainDir, id, ".system_generated", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	transcript := "" +
		`{"step_index":0,"type":"USER_INPUT","created_at":"2026-11-12T14:00:00Z","content":"<USER_REQUEST>\nFirst request about daemon\n</USER_REQUEST>\n<ADDITIONAL_METADATA>\nActive Document: /home/nox/IT-projects/Synapsed/cmd/main.go (GO)\n</ADDITIONAL_METADATA>"}` + "\n" +
		`{"step_index":1,"type":"PLANNER_RESPONSE","created_at":"2026-11-12T14:00:01Z","tool_calls":[{"name":"run_command","args":{"Cwd":"\"/home/nox/IT-projects/Synapsed\""}}]}` + "\n" +
		`{"step_index":2,"type":"USER_INPUT","created_at":"2026-11-12T14:05:00Z","content":"<USER_REQUEST>\nLast request about framing\n</USER_REQUEST>"}` + "\n"

	if err := os.WriteFile(filepath.Join(logsDir, "transcript.jsonl"), []byte(transcript), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	sessions, err := Collect(cfg)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	s := sessions[id]
	if s.FirstPrompt != "First request about daemon" {
		t.Fatalf("FirstPrompt = %q", s.FirstPrompt)
	}
	if s.LastPrompt != "Last request about framing" {
		t.Fatalf("LastPrompt = %q", s.LastPrompt)
	}
	if s.WorkDir != "/home/nox/IT-projects/Synapsed" {
		t.Fatalf("WorkDir = %q", s.WorkDir)
	}
}

func TestCollectHandlesTypedToolCallArgs(t *testing.T) {
	root := t.TempDir()
	cfg := config.FromRoot(root)

	id := "7772dffa-a2df-45d4-862e-ae9b8c25d36e"
	logsDir := filepath.Join(cfg.BrainDir, id, ".system_generated", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}

	targetFile := "/workspace/myproj/main.go"
	transcript := "" +
		`{"step_index":0,"type":"USER_INPUT","created_at":"2026-11-12T16:51:00Z","content":"<USER_REQUEST>\nInspect main.go\n</USER_REQUEST>"}` + "\n" +
		`{"step_index":1,"type":"PLANNER_RESPONSE","created_at":"2026-11-12T16:51:01Z","thinking":"thinking only"}` + "\n" +
		`{"step_index":2,"type":"PLANNER_RESPONSE","created_at":"2026-11-12T16:51:02Z","tool_calls":[{"name":"run_command","args":{"BypassSandbox":false,"WaitMsBeforeAsync":5000,"CommandLine":"go version"}},{"name":"read_file","args":{"StartLine":1,"EndLine":20,"Path":"` + targetFile + `"}}]}` + "\n"

	if err := os.WriteFile(filepath.Join(logsDir, "transcript.jsonl"), []byte(transcript), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	sessions, err := Collect(cfg)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	s, ok := sessions[id]
	if !ok {
		t.Fatalf("missing session %s", id)
	}
	if s.WorkDir != "/workspace/myproj" {
		t.Fatalf("WorkDir = %q, want %q", s.WorkDir, "/workspace/myproj")
	}
}

func TestCollectUsesLastVisitedDirectoryAcrossTranscript(t *testing.T) {
	root := t.TempDir()
	cfg := config.FromRoot(root)

	projectBRoot := filepath.Join(root, "projects", "beta")
	projectBFile := filepath.Join(projectBRoot, "internal", "app.go")
	if err := os.MkdirAll(filepath.Join(projectBRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(projectBFile), 0o755); err != nil {
		t.Fatalf("mkdir projectB: %v", err)
	}
	if err := os.WriteFile(projectBFile, []byte("package internal\n"), 0o600); err != nil {
		t.Fatalf("write app.go: %v", err)
	}
	if got := resolveProjectRoot(projectBFile); got != projectBRoot {
		t.Fatalf("resolveProjectRoot(%q) = %q, want %q", projectBFile, got, projectBRoot)
	}

	id := "9a4c2d1e-7b8e-4c9a-9f12-34567890abcd"
	logsDir := filepath.Join(cfg.BrainDir, id, ".system_generated", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}

	var b strings.Builder
	b.WriteString(`{"step_index":0,"type":"USER_INPUT","created_at":"2026-11-12T10:00:00Z","content":"<USER_REQUEST>\nStart in alpha\n</USER_REQUEST>\n<ADDITIONAL_METADATA>\nActive Document: /workspace/alpha/main.go (GO)\n</ADDITIONAL_METADATA>"}` + "\n")
	b.WriteString(`{"step_index":1,"type":"PLANNER_RESPONSE","created_at":"2026-11-12T10:00:01Z","tool_calls":[{"name":"run_command","args":{"Cwd":"/workspace/alpha"}}]}` + "\n")

	// Simulate >12 intermediate planner turns without paths before visiting beta.
	for i := 2; i < 16; i++ {
		b.WriteString(`{"step_index":` + strconv.Itoa(i) + `,"type":"PLANNER_RESPONSE","created_at":"2026-11-12T10:01:00Z","tool_calls":[{"name":"run_command","args":{"CommandLine":"echo ok"}}]}` + "\n")
	}

	b.WriteString(`{"step_index":16,"type":"PLANNER_RESPONSE","created_at":"2026-11-12T10:02:00Z","tool_calls":[{"name":"read_file","args":{"Path":"/workspace/other/old.go"}},{"name":"read_file","args":{"Path":"/workspace/beta/internal/app.go"}},{"name":"read_file","args":{"Path":"/home/nox/.gemini/antigravity-cli/brain/` + id + `/task.md"}}]}` + "\n")

	if err := os.WriteFile(filepath.Join(logsDir, "transcript.jsonl"), []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	sessions, err := Collect(cfg)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	s, ok := sessions[id]
	if !ok {
		t.Fatalf("missing session %s", id)
	}
	if s.WorkDir != "/workspace/beta/internal" {
		t.Fatalf("WorkDir = %q, want last visited directory %q", s.WorkDir, "/workspace/beta/internal")
	}
}
