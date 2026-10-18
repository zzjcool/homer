package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzjcool/homer/internal/core"
	"github.com/zzjcool/homer/internal/doctor"
	"github.com/zzjcool/homer/internal/gitx"
)

func isolateMissingGitIdentity(t *testing.T) string {
	t.Helper()
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL"} {
		value, ok := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Cleanup(func() { _ = os.Setenv(key, value) })
		}
	}
	return global
}

func TestPushYesMissingIdentitySetsLocalIdentityAndCommits(t *testing.T) {
	global := isolateMissingGitIdentity(t)
	before, err := os.ReadFile(global)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	paths := core.GetHomerPaths(func(name string) string {
		if name == "HOMER_HOME" {
			return home
		}
		return os.Getenv(name)
	})
	tool := filepath.Join(home, "tool")
	writeTestConfig(paths, tool, core.SyncModeMirror)
	if err := os.MkdirAll(tool, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tool, "settings.json"), []byte("baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := RunPush(PushOptions{HomerHome: home, Yes: true}, nil)
	if report.Status != PushStatusPushed || report.ExitCode() != 0 || report.Commit == "" {
		t.Fatalf("missing-identity push = %#v", report)
	}
	if gitx.HeadCommit(home) == "" || !gitx.IsPushClean(home) {
		t.Fatal("push --yes did not establish a baseline commit")
	}
	if got := strings.TrimSpace(gitx.Exec(home, []string{"config", "--local", "--get", "user.name"}, 0).Stdout); got != gitx.LocalIdentityName {
		t.Fatalf("local user.name = %q", got)
	}
	if got := strings.TrimSpace(gitx.Exec(home, []string{"config", "--local", "--get", "user.email"}, 0).Stdout); got != gitx.LocalIdentityEmail {
		t.Fatalf("local user.email = %q", got)
	}
	after, err := os.ReadFile(global)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("global gitconfig changed from %q to %q", before, after)
	}
	if !strings.Contains(strings.Join(report.Warnings, "\n"), "仅在本仓库设置") {
		t.Fatalf("warnings = %#v", report.Warnings)
	}
	author := strings.TrimSpace(gitx.Exec(home, []string{"log", "-1", "--format=%an <%ae>"}, 0).Stdout)
	if author != gitx.LocalIdentityName+" <"+gitx.LocalIdentityEmail+">" {
		t.Fatalf("commit author = %q", author)
	}
}

func TestPushYesKeepsExistingLocalIdentity(t *testing.T) {
	isolateMissingGitIdentity(t)
	home := t.TempDir()
	paths := core.GetHomerPaths(func(name string) string {
		if name == "HOMER_HOME" {
			return home
		}
		return os.Getenv(name)
	})
	writeTestConfig(paths, filepath.Join(home, "tool"), core.SyncModeMirror)
	if err := gitx.EnsureGitRepo(home); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"config", "--local", "user.name", "Keep Me"}, {"config", "--local", "user.email", "keep@example.invalid"}} {
		if result := gitx.Exec(home, args, 0); !result.OK {
			t.Fatal(result.Stderr)
		}
	}
	if err := os.MkdirAll(filepath.Join(home, "tool"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "tool", "settings.json"), []byte("baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := RunPush(PushOptions{HomerHome: home, Yes: true}, nil)
	if report.Status != PushStatusPushed || report.ExitCode() != 0 || report.Commit == "" {
		t.Fatalf("existing-identity push = %#v", report)
	}
	if got := strings.TrimSpace(gitx.Exec(home, []string{"config", "--local", "--get", "user.name"}, 0).Stdout); got != "Keep Me" {
		t.Fatalf("local user.name = %q", got)
	}
	if strings.Contains(strings.Join(report.Warnings, "\n"), "仅在本仓库设置") {
		t.Fatalf("existing identity was rewritten: %#v", report.Warnings)
	}
}

func TestPushYesCommitFailureExitsNonZeroAndDropsUnbornRepo(t *testing.T) {
	root := t.TempDir()
	hooks := filepath.Join(root, "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	script := []byte("#!/bin/sh\nexit 1\n")
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), script, 0o755); err != nil {
		t.Fatal(err)
	}
	global := isolateMissingGitIdentity(t)
	if err := os.WriteFile(global, []byte("[core]\n\thooksPath = "+hooks+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	paths := core.GetHomerPaths(func(name string) string {
		if name == "HOMER_HOME" {
			return home
		}
		return os.Getenv(name)
	})
	writeTestConfig(paths, filepath.Join(home, "tool"), core.SyncModeMirror)
	if err := os.MkdirAll(filepath.Join(home, "tool"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "tool", "settings.json"), []byte("baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := RunPush(PushOptions{HomerHome: home, Yes: true}, nil)
	if report.OK || report.Status != PushStatusError || report.ExitCode() != 1 {
		t.Fatalf("failed baseline push = %#v", report)
	}
	joined := strings.Join(report.Errors, "\n")
	if !strings.Contains(joined, "同步基线未建立") || !strings.Contains(joined, "已撤回本次 git init") || !strings.Contains(joined, "homer push --yes") {
		t.Fatalf("errors = %#v", report.Errors)
	}
	if gitx.IsGitRepo(home) || gitx.HeadCommit(home) != "" {
		t.Fatal("failed first push left a half-initialized git repository")
	}
}

func TestExistingUnbornRepoStaysAndStatusMarksMissingBaseline(t *testing.T) {
	isolateMissingGitIdentity(t)
	home := t.TempDir()
	paths := core.GetHomerPaths(func(name string) string {
		if name == "HOMER_HOME" {
			return home
		}
		return os.Getenv(name)
	})
	config := writeTestConfig(paths, filepath.Join(home, "tool"), core.SyncModeMirror)
	if err := gitx.EnsureGitRepo(home); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(home, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "tool"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "tool", "settings.json"), []byte("baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := RunPush(PushOptions{HomerHome: home, Yes: true}, nil)
	if report.ExitCode() != 1 || report.Status != PushStatusError {
		t.Fatalf("unborn push = %#v", report)
	}
	joined := strings.Join(report.Errors, "\n")
	if !strings.Contains(joined, "同步基线未建立") || strings.Contains(joined, "已撤回本次 git init") {
		t.Fatalf("errors = %#v", report.Errors)
	}
	if !gitx.IsGitRepo(home) || gitx.HeadCommit(home) != "" {
		t.Fatal("pre-existing repository should remain, still without a baseline commit")
	}

	sources, err := CollectSnapshotSources(paths, &config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(sources.Warnings, "\n"), "同步基线未建立") {
		t.Fatalf("status warnings = %#v", sources.Warnings)
	}
	check := doctor.CheckRepoAndStore(paths)[0]
	if check.Status != doctor.CheckWarn || !strings.Contains(check.Message, "同步基线未建立") {
		t.Fatalf("doctor repo = %#v", check)
	}
}
