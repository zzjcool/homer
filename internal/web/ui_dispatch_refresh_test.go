package web

import (
	"strings"
	"testing"
)

func TestDispatchRequestCanBeAbortedAndCannotWriteToAnotherDialog(t *testing.T) {
	source := string(staticIndex)
	api := uiIssueSection(t, source, "async function api(path, opts = {}) {", "\nfunction splitNDJSONLines")
	requireUIIssueStrings(t, api,
		"if (opts.signal !== undefined) init.signal = opts.signal;",
		"const res = await fetch(path, init);",
	)

	close := uiIssueSection(t, source, "$(\"dlg-confirm\").addEventListener(\"close\", () => {", "\n// 显式接线")
	requireUIIssueStrings(t, close,
		"if (app.dispatchController) app.dispatchController.abort();",
		"app.dispatchController = null;",
		"$(\"btn-confirm-back\").disabled = false;",
	)

	wait := uiIssueSection(t, source, "function beginDispatchWait(name, plugins, unlocking) {", "\nfunction resolutionOutcomeLines")
	if strings.Contains(wait, "cancel.disabled = true") {
		t.Fatal("the cancel button must remain usable while dispatch is in progress")
	}

	dispatch := uiIssueSection(t, source, "async function executeDispatchOne(agentId, name, adapters, unlocks, stay) {", "\nasync function executeSSHKey")
	requireUIIssueStrings(t, dispatch,
		"const controller = new AbortController();",
		"signal: controller.signal",
		"dispatchDialogOwns(agentId, controller)",
		"finishDispatchInDialog(agentId, controller, name, paths, installFailures(d), d)",
	)

	owner := uiIssueSection(t, source, "function dispatchDialogOwns(", "\nfunction finishDispatchInDialog")
	requireUIIssueStrings(t, owner,
		"app.scopeAgent.agentId === agentId",
		"app.dispatchController === controller",
		"$(\"dlg-confirm\").open",
	)
	finish := uiIssueSection(t, source, "function finishDispatchInDialog(", "\nfunction failDispatchInDialog")
	requireUIIssueStrings(t, finish,
		"if (!dispatchDialogOwns(agentId, controller))",
		"toast(\"已下发到 \" + name)",
	)
}

func TestAgentRefreshSkipsRebuildWhileMoreMenuIsOpen(t *testing.T) {
	source := string(staticIndex)
	render := uiIssueSection(t, source, "function renderAgents(agents, disabledFeature) {", "\nfunction upgradeFailure")
	guard := strings.Index(render, `list.querySelector("details.agent-more[open]")`)
	clear := strings.Index(render, "clear(list);")
	if guard < 0 || clear < 0 || guard > clear {
		t.Fatal("renderAgents must preserve the current cards before clearing while a more menu is open")
	}
	requireUIIssueStrings(t, render,
		"document.activeElement",
		".closest(\".agent-more\")",
		"return;",
	)
}
