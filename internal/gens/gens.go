// Package gens implements the hub's current-state layout (advisor ruling
// 2026-09-28, no-git data plane): immutable generations under
// generations/<n>/{store,homer.json} plus a single HEAD file naming the
// live generation. Writers publish a complete tree and then flip HEAD
// atomically; readers only ever trust HEAD, so a crash before the flip
// leaves the previous generation fully intact.
package gens

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Layout is the on-disk generation store rooted at a homer home.
type Layout struct {
	home string
}

// ErrInvalidStorePath marks an adapter or file path that is not safe to publish.
var ErrInvalidStorePath = errors.New("invalid snapshot store path")

// New builds the layout for a homer home directory.
func New(home string) *Layout {
	return &Layout{home: home}
}

// Head is the currently published generation: where its files live and
// what generation number it carries.
type Head struct {
	Generation int
	StoreDir   string
	Meta       []byte // homer.json bytes of that generation
}

// Publish writes a complete generation and flips HEAD to it. The store
// argument maps adapter ID -> relative path -> file content. The
// generation directory is built under a temporary name first, so a crash
// mid-write never becomes visible through HEAD.
func (l *Layout) Publish(store map[string]map[string]string, meta []byte) (int, error) {
	generationsDir := filepath.Join(l.home, "generations")
	next := l.currentGeneration() + 1
	target := filepath.Join(generationsDir, strconv.Itoa(next))
	staging := filepath.Join(generationsDir, fmt.Sprintf(".staging-%d", next))
	storeDir := filepath.Join(staging, "store")

	// Validate every path before touching the filesystem so an invalid entry
	// rejects the complete snapshot without partially publishing any files.
	for adapter, files := range store {
		adapterDir, err := safeStorePath(storeDir, adapter, "adapter")
		if err != nil {
			return 0, err
		}
		for name := range files {
			if _, err := safeStorePath(adapterDir, name, "file"); err != nil {
				return 0, err
			}
		}
	}

	if err := os.MkdirAll(generationsDir, 0o755); err != nil {
		return 0, fmt.Errorf("创建 generations 目录: %w", err)
	}

	// Staging must not survive a previous crashed attempt.
	_ = os.RemoveAll(staging)
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		return 0, fmt.Errorf("创建 staging 目录: %w", err)
	}

	for adapter, files := range store {
		adapterDir, err := safeStorePath(storeDir, adapter, "adapter")
		if err != nil {
			return 0, err
		}
		if err := os.MkdirAll(adapterDir, 0o755); err != nil {
			return 0, fmt.Errorf("创建 adapter 目录: %w", err)
		}
		for name, content := range files {
			path, err := safeStorePath(adapterDir, name, "file")
			if err != nil {
				return 0, err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return 0, fmt.Errorf("创建文件目录: %w", err)
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return 0, fmt.Errorf("写入文件: %w", err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(staging, "homer.json"), meta, 0o644); err != nil {
		return 0, fmt.Errorf("写入 homer.json: %w", err)
	}

	// Move the finished tree into place, then flip HEAD. rename(2) on the
	// staged directory is atomic from the reader's perspective: before it,
	// the generation does not exist; after it, it is complete.
	if err := os.Rename(staging, target); err != nil {
		return 0, fmt.Errorf("落盘 generation: %w", err)
	}
	headPath := filepath.Join(l.home, "HEAD")
	headTmp := headPath + ".tmp"
	if err := os.WriteFile(headTmp, []byte(strconv.Itoa(next)+"\n"), 0o644); err != nil {
		return 0, fmt.Errorf("写入 HEAD: %w", err)
	}
	if err := os.Rename(headTmp, headPath); err != nil {
		return 0, fmt.Errorf("翻转 HEAD: %w", err)
	}
	return next, nil
}

// Read returns the generation HEAD currently names. A missing or
// malformed HEAD reports no generation.
func safeStorePath(root, name, kind string) (string, error) {
	invalid := func(reason string) (string, error) {
		return "", fmt.Errorf("%w: %s %q: %s", ErrInvalidStorePath, kind, name, reason)
	}
	if name == "" {
		return invalid("empty path")
	}
	if filepath.IsAbs(filepath.FromSlash(name)) {
		return invalid("absolute path")
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == ".." {
			return invalid("parent traversal segment")
		}
	}

	path := filepath.Join(root, filepath.FromSlash(name))
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return invalid("cannot verify path is relative to its root")
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return invalid("path escapes its root or names the root itself")
	}
	return path, nil
}

func (l *Layout) Read() (Head, bool) {
	data, err := os.ReadFile(filepath.Join(l.home, "HEAD"))
	if err != nil {
		return Head{}, false
	}
	number, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || number < 1 {
		return Head{}, false
	}
	meta, _ := os.ReadFile(filepath.Join(l.home, "generations", strconv.Itoa(number), "homer.json"))
	return Head{
		Generation: number,
		StoreDir:   filepath.Join(l.home, "generations", strconv.Itoa(number), "store"),
		Meta:       meta,
	}, true
}

// currentGeneration reads HEAD without interpreting missing as zero-error.
func (l *Layout) currentGeneration() int {
	head, ok := l.Read()
	if !ok {
		return 0
	}
	return head.Generation
}
