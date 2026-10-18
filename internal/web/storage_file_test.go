package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/zzjcool/homer/internal/gens"
)

func newStorageFileTestServer(t *testing.T) *Server {
	t.Helper()
	home := t.TempDir()
	_, err := gens.New(home).Publish(map[string]map[string]string{
		"pi": {
			"agents/c.md": "agent content",
			"a/./b/c.md":  "nested content",
			"sub/deep.md": "deep content",
			"hub-token":   "adapter content",
		},
	}, []byte(`{"version":1}`))
	if err != nil {
		t.Fatalf("publish test generation: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, "keys"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "keys", "hub-token"), []byte("sensitive hub token"), 0o600); err != nil {
		t.Fatal(err)
	}
	return &Server{opts: ServeOptions{HomerHome: home}}
}

func storageFileRequest(t *testing.T, server *Server, adapter, filePath string) *httptest.ResponseRecorder {
	t.Helper()
	query := url.Values{
		"adapter": []string{adapter},
		"path":    []string{filePath},
	}
	request := httptest.NewRequest(http.MethodGet, "/api/storage/file?"+query.Encode(), nil)
	recorder := httptest.NewRecorder()
	server.handleStorageFile(recorder, request)
	return recorder
}

func TestStorageFileReadsValidPaths(t *testing.T) {
	server := newStorageFileTestServer(t)
	cases := []struct {
		adapter string
		path    string
		want    string
	}{
		{adapter: "pi", path: "agents/c.md", want: "agent content"},
		{adapter: "pi", path: "a/./b/c.md", want: "nested content"},
		{adapter: "pi", path: "sub/deep.md", want: "deep content"},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			recorder := storageFileRequest(t, server, tc.adapter, tc.path)
			if recorder.Code != http.StatusOK {
				t.Fatalf("handleStorageFile status = %d, body = %s", recorder.Code, recorder.Body)
			}
			var response struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode storage response: %v", err)
			}
			if response.Content != tc.want {
				t.Errorf("handleStorageFile content = %q, want %q", response.Content, tc.want)
			}

			content, ok, binary := server.storedFileText(tc.adapter, tc.path)
			if !ok || binary || content != tc.want {
				t.Errorf("storedFileText = (%q, %v, %v), want (%q, true, false)", content, ok, binary, tc.want)
			}
		})
	}
}

func TestStorageFileRejectsTraversal(t *testing.T) {
	server := newStorageFileTestServer(t)
	cases := []struct {
		name    string
		adapter string
		path    string
	}{
		{name: "adapter escapes to keys", adapter: "../../../keys", path: "hub-token"},
		{name: "adapter escapes to etc", adapter: "../../../../../etc", path: "passwd"},
		{name: "adapter parent", adapter: "..", path: "secret"},
		{name: "absolute adapter", adapter: "/etc", path: "passwd"},
		{name: "empty adapter", adapter: "", path: "agents/c.md"},
		{name: "empty path", adapter: "pi", path: ""},
		{name: "path parent", adapter: "pi", path: "../secret"},
		{name: "absolute path", adapter: "pi", path: "/etc/passwd"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := storageFileRequest(t, server, tc.adapter, tc.path)
			if recorder.Code != http.StatusBadRequest {
				t.Errorf("handleStorageFile status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body)
			}
			if content, ok, _ := server.storedFileText(tc.adapter, tc.path); ok {
				t.Errorf("storedFileText returned content %q for rejected path", content)
			}
		})
	}
}
