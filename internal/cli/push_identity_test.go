package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzjcool/homer-cli/internal/gitx"
)

func TestCLIPushYesMissingIdentityExitsZeroAndFailureExitsOne(t *testing.T) {
	toolHome := t.TempDir()
	t.Setenv("HOME", toolHome)
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

	home := t.TempDir()
	var out, errOut bytes.Buffer
	if code := runWithIO([]string{"homer", "init", "--adapters", "pi", "--json", "--home", home}, &out, &errOut); code != 0 {
		t.Fatalf("init exit = %d\nstdout=%s\nstderr=%s", code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	code := runWithIO([]string{"homer", "push", "--yes", "--json", "--home", home}, &out, &errOut)
	if code != 0 {
		t.Fatalf("push --yes exit = %d\nstdout=%s\nstderr=%s", code, out.String(), errOut.String())
	}
	var report map[string]any
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("push json: %v\n%s", err, out.String())
	}
	if report["ok"] != true || report["status"] != "pushed" || report["commit"] == "" {
		t.Fatalf("push report = %#v", report)
	}
	if gitx.HeadCommit(home) == "" {
		t.Fatal("cli push did not create a baseline commit")
	}
	if got := strings.TrimSpace(gitx.Exec(home, []string{"config", "--local", "--get", "user.email"}, 0).Stdout); got != gitx.LocalIdentityEmail {
		t.Fatalf("local user.email = %q", got)
	}
	globalText, err := os.ReadFile(global)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(globalText)) != "" {
		t.Fatalf("global gitconfig = %q", globalText)
	}

	hooks := filepath.Join(t.TempDir(), "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(global, []byte("[core]\n\thooksPath = "+hooks+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	failedHome := t.TempDir()
	out.Reset()
	errOut.Reset()
	if code := runWithIO([]string{"homer", "init", "--adapters", "pi", "--json", "--home", failedHome}, &out, &errOut); code != 0 {
		t.Fatalf("second init exit = %d\n%s\n%s", code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	code = runWithIO([]string{"homer", "push", "--yes", "--json", "--home", failedHome}, &out, &errOut)
	if code == 0 {
		t.Fatalf("failed push exited 0\nstdout=%s", out.String())
	}
	report = map[string]any{}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("failed push json: %v\n%s", err, out.String())
	}
	if report["ok"] != false {
		t.Fatalf("failed push report = %#v", report)
	}
	if gitx.IsGitRepo(failedHome) {
		t.Fatal("failed cli push left a half-initialized repository")
	}
}
