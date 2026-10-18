package commands

import (
	"strings"
	"testing"

	"github.com/zzjcool/homer/internal/adapter"
)

func TestSourceErrorMessagesDefaultOmitsDetail(t *testing.T) {
	errors := SnapshotSourceErrors{{
		AdapterID: "pi",
		Errors: []adapter.ScanError{{
			Path:    "pi list",
			Message: "扫描失败，无法列出 pi/packages：exit status 1",
			Detail:  "exit status 1: Error: boom\n    at stack",
		}},
	}}
	got := SourceErrorMessages(errors)
	if len(got) != 1 || strings.Contains(got[0], "boom") || strings.Contains(got[0], "at stack") {
		t.Fatalf("default errors = %#v", got)
	}
	verbose := SourceErrorMessagesVerbose(errors)
	if len(verbose) != 1 || !strings.Contains(verbose[0], "boom") {
		t.Fatalf("verbose errors = %#v", verbose)
	}
}

func TestFilterSnapshotSourceErrorsByAdapter(t *testing.T) {
	errors := SnapshotSourceErrors{
		{AdapterID: "pi", Errors: []adapter.ScanError{{Path: "pi list", Message: "scan fail"}}},
		{AdapterID: "herdr", Errors: []adapter.ScanError{{Path: "root", Message: "ok"}}},
	}
	got := FilterSnapshotSourceErrors(errors, []string{"herdr"})
	if len(got) != 1 || got[0].AdapterID != "herdr" {
		t.Fatalf("filtered = %#v", got)
	}
	lines := FilterAdapterPrefixedLines([]string{
		"pi: pi 未安装，packages 已按空清单处理；pull 时远端清单将视为全量待装",
		"扫描告警: pi (pi list: 扫描失败，无法列出 pi/packages：exit status 1)",
		"herdr: something",
		"扫描告警: herdr (root: ok)",
		"未安装 pi，无法下发适配器 pi。",
		"unscoped noise",
	}, []string{"herdr"})
	if len(lines) != 2 || lines[0] != "herdr: something" || lines[1] != "扫描告警: herdr (root: ok)" {
		t.Fatalf("prefixed = %#v", lines)
	}
}
