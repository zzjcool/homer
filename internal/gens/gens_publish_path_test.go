package gens

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishAllowsSafeStorePaths(t *testing.T) {
	paths := []string{
		"agents/c.md",
		"AGENTS.md",
		"sub/dir/deep.md",
		"a/./b/c.md",
	}
	store := map[string]map[string]string{"pi": {}}
	for i, name := range paths {
		store["pi"][name] = string(rune('a' + i))
	}

	layout := New(filepath.Join(t.TempDir(), "home"))
	_, err := layout.Publish(store, []byte("{}"))
	if err != nil {
		t.Fatalf("Publish rejected safe paths: %v", err)
	}

	head, ok := layout.Read()
	if !ok {
		t.Fatal("published generation is missing")
	}
	for i, name := range paths {
		got, err := os.ReadFile(filepath.Join(head.StoreDir, "pi", filepath.FromSlash(name)))
		if err != nil {
			t.Errorf("read %q: %v", name, err)
			continue
		}
		if want := string(rune('a' + i)); string(got) != want {
			t.Errorf("%q = %q, want %q", name, got, want)
		}
	}
}

func TestPublishRejectsUnsafeStoreFilePaths(t *testing.T) {
	paths := []string{
		"../../../../keys/hub-token",
		"../../../../../../tmp/ESCAPE",
		"../evil",
		"a/../../evil",
		"",
		"/abs/path",
		".",
		"..",
	}
	for _, name := range paths {
		t.Run(name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "home")
			layout := New(home)
			if _, err := layout.Publish(map[string]map[string]string{"pi": {name: "must not be written"}}, []byte("{}")); err == nil {
				t.Errorf("Publish accepted unsafe file path %q", name)
			}
			assertNoFiles(t, home)
		})
	}
}

func TestPublishRejectsUnsafeAdapterPaths(t *testing.T) {
	adapters := []string{"../../../../escape-adapter", "../evil", "", "/abs/path", ".", ".."}
	for _, adapter := range adapters {
		t.Run(adapter, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "home")
			layout := New(home)
			store := map[string]map[string]string{adapter: {"safe.md": "must not be written"}}
			if _, err := layout.Publish(store, []byte("{}")); err == nil {
				t.Errorf("Publish accepted unsafe adapter path %q", adapter)
			}
			assertNoFiles(t, home)
		})
	}
}

func assertNoFiles(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return
	} else if err != nil {
		t.Fatalf("stat %q: %v", root, err)
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root && !entry.IsDir() {
			t.Errorf("unexpected file created at %q", path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk %q: %v", root, err)
	}
}
