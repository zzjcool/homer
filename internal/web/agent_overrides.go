package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/zzjcool/homer-cli/internal/core"
	"github.com/zzjcool/homer-cli/internal/pluginregistry"
)

const agentOverridesFilename = "agent-overrides.json"
const agentOverridesSchemaVersion = 1

// AgentOverrides is the hub-owned per-machine adapter override set. Only
// disabled adapters are persisted; enabling one removes the override and
// restores the center generation's default.
type AgentOverrides struct {
	mu       sync.RWMutex
	path     string
	disabled map[string]map[string]bool
}

type agentOverridesDiskState struct {
	SchemaVersion int                        `json:"schemaVersion"`
	Disabled      map[string]map[string]bool `json:"disabled"`
}

// LoadAgentOverrides loads <home>/agent-overrides.json. Missing or invalid
// state starts empty; the next successful change writes the current schema.
func LoadAgentOverrides(home string) *AgentOverrides {
	if strings.TrimSpace(home) == "" {
		home = core.GetHomerPaths(nil).Home
	}
	overrides := &AgentOverrides{
		path:     filepath.Join(home, agentOverridesFilename),
		disabled: make(map[string]map[string]bool),
	}
	data, err := os.ReadFile(overrides.path)
	if err != nil {
		return overrides
	}
	var saved agentOverridesDiskState
	if json.Unmarshal(data, &saved) != nil || saved.SchemaVersion != agentOverridesSchemaVersion {
		return overrides
	}
	for agentID, adapters := range saved.Disabled {
		if strings.TrimSpace(agentID) == "" {
			continue
		}
		for adapterID, disabled := range adapters {
			if !disabled || !core.ValidAdapterID(adapterID) {
				continue
			}
			if overrides.disabled[agentID] == nil {
				overrides.disabled[agentID] = make(map[string]bool)
			}
			overrides.disabled[agentID][adapterID] = true
		}
	}
	return overrides
}

// SetDisabled records or removes one disabled adapter fact and persists the
// update atomically. Plugin-role/installation validation is performed by the
// authenticated API handler, which has access to the live plugin state.
func (o *AgentOverrides) SetDisabled(agentID, adapterID string, disabled bool) error {
	if o == nil {
		return errors.New("agent overrides are unavailable")
	}
	if strings.TrimSpace(agentID) == "" {
		return errors.New("agent id must not be empty")
	}
	if !core.ValidAdapterID(adapterID) {
		return fmt.Errorf("invalid adapter id %q", adapterID)
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	if o.disabled[agentID][adapterID] == disabled {
		return nil
	}
	next := cloneAgentOverrides(o.disabled)
	if disabled {
		if next[agentID] == nil {
			next[agentID] = make(map[string]bool)
		}
		next[agentID][adapterID] = true
	} else if adapters := next[agentID]; adapters != nil {
		delete(adapters, adapterID)
		if len(adapters) == 0 {
			delete(next, agentID)
		}
	}
	if err := o.persist(next); err != nil {
		return err
	}
	o.disabled = next
	return nil
}

// Disabled reports whether adapterID is disabled for agentID.
func (o *AgentOverrides) Disabled(agentID, adapterID string) bool {
	if o == nil {
		return false
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.disabled[agentID][adapterID]
}

// DisabledAdapters returns a sorted copy of the disabled adapter IDs for one
// machine. The returned slice is detached from the override state.
func (o *AgentOverrides) DisabledAdapters(agentID string) []string {
	if o == nil {
		return []string{}
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	adapters := o.disabled[agentID]
	out := make([]string, 0, len(adapters))
	for adapterID, disabled := range adapters {
		if disabled {
			out = append(out, adapterID)
		}
	}
	sort.Strings(out)
	return out
}

func (o *AgentOverrides) persist(disabled map[string]map[string]bool) error {
	saved := agentOverridesDiskState{
		SchemaVersion: agentOverridesSchemaVersion,
		Disabled:      disabled,
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal agent overrides: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(o.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create agent overrides directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".agent-overrides-*.tmp")
	if err != nil {
		return fmt.Errorf("create agent overrides temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set agent overrides permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write agent overrides: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync agent overrides: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close agent overrides: %w", err)
	}
	if err := os.Rename(tmpPath, o.path); err != nil {
		return fmt.Errorf("publish agent overrides: %w", err)
	}
	return nil
}

func cloneAgentOverrides(source map[string]map[string]bool) map[string]map[string]bool {
	clone := make(map[string]map[string]bool, len(source))
	for agentID, adapters := range source {
		for adapterID, disabled := range adapters {
			if !disabled {
				continue
			}
			if clone[agentID] == nil {
				clone[agentID] = make(map[string]bool)
			}
			clone[agentID][adapterID] = true
		}
	}
	return clone
}

type agentAdapterView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Disabled bool   `json:"disabled"`
}

func (s *Server) handleAgentAdapters(w http.ResponseWriter, _ *http.Request, agentID string) {
	if strings.TrimSpace(agentID) == "" {
		writeError(w, http.StatusBadRequest, "override-invalid", "agent id 不能为空", nil)
		return
	}

	adaptersByID := make(map[string]agentAdapterView)
	if s.opts.Plugins != nil {
		for _, plugin := range s.opts.Plugins.List() {
			if plugin.Role != pluginregistry.RoleAdapter || !core.ValidAdapterID(plugin.ID) {
				continue
			}
			name := strings.TrimSpace(plugin.Name)
			if name == "" {
				name = plugin.ID
			}
			adaptersByID[plugin.ID] = agentAdapterView{ID: plugin.ID, Name: name}
		}
	}

	// The last machine report keeps an uninstalled/unknown adapter visible in
	// this machine's view. Built-in non-adapter plugins (keyring/action) are
	// deliberately omitted so they can never acquire a disable control.
	if source, ok := s.opts.Agents.(ReportedAdaptersSource); ok {
		for _, adapterID := range source.AgentReportedAdapters(agentID) {
			if !core.ValidAdapterID(adapterID) {
				continue
			}
			if builtin, known := pluginregistry.Builtin(adapterID); known && builtin.Role != pluginregistry.RoleAdapter {
				continue
			}
			if _, exists := adaptersByID[adapterID]; exists {
				continue
			}
			name := adapterID
			if builtin, known := pluginregistry.Builtin(adapterID); known && strings.TrimSpace(builtin.Name) != "" {
				name = builtin.Name
			}
			adaptersByID[adapterID] = agentAdapterView{ID: adapterID, Name: name}
		}
	}

	ids := make([]string, 0, len(adaptersByID))
	for adapterID := range adaptersByID {
		ids = append(ids, adapterID)
	}
	sort.Strings(ids)
	adapters := make([]agentAdapterView, 0, len(ids))
	for _, adapterID := range ids {
		adapter := adaptersByID[adapterID]
		adapter.Disabled = s.opts.Overrides != nil && s.opts.Overrides.Disabled(agentID, adapterID)
		adapters = append(adapters, adapter)
	}
	writeJSON(w, http.StatusOK, struct {
		Adapters []agentAdapterView `json:"adapters"`
	}{Adapters: adapters})
}

func (s *Server) handleAgentAdapterOverride(w http.ResponseWriter, r *http.Request, agentID, adapterID string) {
	if strings.TrimSpace(agentID) == "" {
		writeError(w, http.StatusBadRequest, "override-invalid", "agent id 不能为空", nil)
		return
	}
	if !core.ValidAdapterID(adapterID) {
		writeError(w, http.StatusUnprocessableEntity, "override-invalid", fmt.Sprintf("adapter id %q 无效", adapterID), nil)
		return
	}
	if s.opts.Overrides == nil {
		writeError(w, http.StatusServiceUnavailable, "overrides-disabled", "按机器停用覆盖层未启用", nil)
		return
	}

	var payload struct {
		Disabled *bool `json:"disabled"`
	}
	if err := decodeLimitedJSON(r, &payload); err != nil || payload.Disabled == nil {
		message := "请求体必须包含 disabled 布尔值"
		if err != nil {
			message = err.Error()
		}
		writeError(w, http.StatusBadRequest, "override-invalid", message, nil)
		return
	}
	if *payload.Disabled {
		if s.opts.Plugins == nil {
			writePluginsDisabled(w)
			return
		}
		installedAdapter := false
		for _, plugin := range s.opts.Plugins.List() {
			if plugin.ID == adapterID && plugin.Role == pluginregistry.RoleAdapter {
				installedAdapter = true
				break
			}
		}
		if !installedAdapter {
			writeError(w, http.StatusUnprocessableEntity, "override-invalid", fmt.Sprintf("%s 未安装或不是 adapter 插件，不能停用", adapterID), nil)
			return
		}
	}
	if err := s.opts.Overrides.SetDisabled(agentID, adapterID, *payload.Disabled); err != nil {
		writeError(w, http.StatusInternalServerError, "override-store-failed", "保存机器适配器停用状态失败", []string{err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
