package gitx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		unsetEnv(t, key)
	}
	return global
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	value, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Cleanup(func() { _ = os.Setenv(key, value) })
	}
}

func TestEnsureLocalIdentityWritesOnlyThisRepo(t *testing.T) {
	global := isolateMissingGitIdentity(t)
	home := t.TempDir()
	if err := EnsureGitRepo(home); err != nil {
		t.Fatal(err)
	}
	if HasCommitIdentity(home) {
		t.Fatal("isolated repo unexpectedly has a commit identity")
	}

	wrote, err := EnsureLocalIdentity(home)
	if err != nil || !wrote {
		t.Fatalf("EnsureLocalIdentity = %v, %v", wrote, err)
	}
	if !HasCommitIdentity(home) {
		t.Fatal("local identity did not satisfy git")
	}
	if got := strings.TrimSpace(mustExec(t, home, "config", "--local", "--get", "user.name")); got != LocalIdentityName {
		t.Fatalf("local user.name = %q", got)
	}
	if got := strings.TrimSpace(mustExec(t, home, "config", "--local", "--get", "user.email")); got != LocalIdentityEmail {
		t.Fatalf("local user.email = %q", got)
	}
	globalText, err := os.ReadFile(global)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(globalText)) != "" {
		t.Fatalf("global gitconfig was modified: %q", globalText)
	}
	if again, err := EnsureLocalIdentity(home); err != nil || again {
		t.Fatalf("second EnsureLocalIdentity = %v, %v; want false, nil", again, err)
	}
}

func TestEnsureLocalIdentityFillsOnlyTheMissingKey(t *testing.T) {
	global := isolateMissingGitIdentity(t)
	if err := os.WriteFile(global, []byte("[user]\n\tname = Kept Global\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if err := EnsureGitRepo(home); err != nil {
		t.Fatal(err)
	}
	wrote, err := EnsureLocalIdentity(home)
	if err != nil || !wrote {
		t.Fatalf("EnsureLocalIdentity = %v, %v", wrote, err)
	}
	if result := Exec(home, []string{"config", "--local", "--get", "user.name"}, 0); result.OK {
		t.Fatalf("local user.name should stay unset, got %q", result.Stdout)
	}
	if got := strings.TrimSpace(mustExec(t, home, "config", "--local", "--get", "user.email")); got != LocalIdentityEmail {
		t.Fatalf("local user.email = %q", got)
	}
	if got := strings.TrimSpace(mustExec(t, home, "config", "--get", "user.name")); got != "Kept Global" {
		t.Fatalf("resolved user.name = %q", got)
	}
}

func TestEnsureLocalIdentityLeavesExistingIdentityUntouched(t *testing.T) {
	isolateMissingGitIdentity(t)
	home := t.TempDir()
	if err := EnsureGitRepo(home); err != nil {
		t.Fatal(err)
	}
	mustExec(t, home, "config", "--local", "user.name", "Keep Me")
	mustExec(t, home, "config", "--local", "user.email", "keep@example.invalid")
	wrote, err := EnsureLocalIdentity(home)
	if err != nil || wrote {
		t.Fatalf("EnsureLocalIdentity = %v, %v; want false, nil", wrote, err)
	}
	if got := strings.TrimSpace(mustExec(t, home, "config", "--local", "--get", "user.name")); got != "Keep Me" {
		t.Fatalf("local user.name = %q", got)
	}
}

func TestRemoveUnbornRepoKeepsHistory(t *testing.T) {
	isolateMissingGitIdentity(t)
	home := t.TempDir()
	if err := EnsureGitRepo(home); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureLocalIdentity(home); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(home, "homer.json"), "{}\n")
	if CommitAllStore(home, "baseline") == "" {
		t.Fatal("baseline commit missing")
	}
	if err := RemoveUnbornRepo(home); err == nil {
		t.Fatal("RemoveUnbornRepo removed a repository that already has a commit")
	}
	if !IsGitRepo(home) || HeadCommit(home) == "" {
		t.Fatal("history repository was damaged")
	}

	unborn := t.TempDir()
	if err := EnsureGitRepo(unborn); err != nil {
		t.Fatal(err)
	}
	if err := RemoveUnbornRepo(unborn); err != nil {
		t.Fatal(err)
	}
	if IsGitRepo(unborn) {
		t.Fatal("unborn repository was not removed")
	}
}
