package e2e

import (
	"strings"
	"testing"
)

func antctlOutput(stdout, stderr string, tb testing.TB) {
	tb.Logf("`antctl version` stdout:\n%s", stdout)
	tb.Logf("`antctl version` stderr:\n%s", stderr)
}

func runAntctl(podName string, subCmds []string, data *TestData, tb testing.TB) (string, string, error) {
	var containerName string
	if strings.Contains(podName, "agent") { // simple hack here
		containerName = "antrea-agent"
	} else {
		containerName = "antrea-controller"
	}

	cmds := []string{"antctl"}
	stdout, stderr, err := data.runCommandFromPod(AntreaNamespace, podName, containerName, append(cmds, subCmds...))
	antctlOutput(stdout, stderr, tb)

	return stdout, stderr, err
}

func TestAntctlAgentLocalAccess(t *testing.T) {
	data, err := setupTest(t)
	if err != nil {
		t.Fatalf("Error when setting up test: %v", err)
	}
	defer teardownTest(t, data)
	podName, err := data.getAntreaPodOnNode(masterNodeName())
	if err != nil {
		t.Fatalf("Error when getting antrea-agent pod name: %v", err)
	}
	if _, _, err := runAntctl(podName, []string{"version"}, data, t); err != nil {
		t.Fatalf("Error when running `antctl version` from %s: %v", podName, err)
	}
}

func TestAntctlControllerLocalAccess(t *testing.T) {
	data, err := setupTest(t)
	if err != nil {
		t.Fatalf("Error when setting up test: %v", err)
	}
	defer teardownTest(t, data)
	podName, err := data.getAntreaController()
	if err != nil {
		t.Fatalf("Error when getting antrea-controller pod name: %v", err)
	}
	if _, _, err := runAntctl(podName, []string{"version"}, data, t); err != nil {
		t.Fatalf("Error when running `antctl version` from %s: %v", podName, err)
	}
}

func TestAntctlControllerRemoteAccessControl(t *testing.T) {
	data, err := setupTest(t)
	if err != nil {
		t.Fatalf("Error when setting up test: %v", err)
	}
	defer teardownTest(t, data)

	_, stdout, stderr, err := RunCommandOnNode(masterNodeName(), "~/antctl version")
	antctlOutput(stdout, stderr, t)
	if err != nil {
		t.Fatalf("Error when running `antctl version` from %s: %v", masterNodeName(), err)
	}
}
