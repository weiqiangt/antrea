// Copyright 2020 Antrea Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vmware-tanzu/antrea/pkg/antctl"
	"github.com/vmware-tanzu/antrea/pkg/antctl/framework"
	"github.com/vmware-tanzu/antrea/pkg/antctl/runtime"
)

type cmdAndReturnCode struct {
	args               []string
	expectedReturnCode int
}

// GetDebugCommands returns all commands supported by Controller or Agent that
// are used for debugging purpose.
func GetDebugCommands(cl *framework.CommandList, mode string) [][]string {
	var allCommands [][]string
	for _, def := range cl.Definitions {
		// TODO: incorporate query commands into e2e testing once proxy access is implemented
		if def.CommandGroup == framework.CommandGroupQuery {
			continue
		}
		if mode == runtime.ModeController && def.Use == "log-level" {
			// log-level command does not support remote execution.
			continue
		}
		if mode == runtime.ModeAgent && def.AgentEndpoint != nil ||
			mode == runtime.ModeController && def.ControllerEndpoint != nil {
			var currentCommand []string
			if group, ok := framework.GroupCommands[def.CommandGroup]; ok {
				currentCommand = append(currentCommand, group.Use)
			}
			currentCommand = append(currentCommand, def.Use)
			allCommands = append(allCommands, currentCommand)
		}
	}
	for _, cmd := range cl.RawCommands {
		if cmd.CobraCommand.Use == "proxy" {
			// proxy will keep running until interrupted so it
			// cannot be used as is in e2e tests.
			continue
		}
		if mode == runtime.ModeController && cmd.SupportController ||
			mode == runtime.ModeAgent && cmd.SupportAgent {
			allCommands = append(allCommands, strings.Split(cmd.CobraCommand.Use, " ")[:1])
		}
	}
	return allCommands
}

// antctlOutput is a helper function for logging antctl outputs.
func antctlOutput(stdout, stderr string, tb testing.TB) {
	tb.Logf("antctl stdout:\n%s", stdout)
	tb.Logf("antctl stderr:\n%s", stderr)
}

// runAntctl runs antctl commands on antrea Pods, the controller, or agents.
func runAntctl(podName string, cmds []string, data *TestData) (string, string, error) {
	var containerName string
	if strings.Contains(podName, "agent") {
		containerName = "antrea-agent"
	} else {
		containerName = "antrea-controller"
	}
	stdout, stderr, err := data.runCommandFromPod(antreaNamespace, podName, containerName, cmds)
	// remove Bincover metadata if needed
	if err == nil {
		index := strings.Index(stdout, "START_BINCOVER_METADATA")
		if index != -1 {
			stdout = stdout[:index]
		}
	}
	return stdout, stderr, err
}

func antctlCoverageArgs(antctlPath string) []string {
	const timeFormat = "20060102T150405Z0700"
	timeStamp := time.Now().Format(timeFormat)
	return []string{antctlPath, "-test.run=TestBincoverRunMain", fmt.Sprintf("-test.coverprofile=antctl-%s.out", timeStamp)}
}

// TestAntctlAgentLocalAccess ensures antctl is accessible in a agent Pod.
func TestAntctlAgentLocalAccess(t *testing.T) {
	data, err := setupTest(t)
	if err != nil {
		t.Fatalf("Error when setting up test: %v", err)
	}
	defer teardownTest(t, data)
	podName, err := data.getAntreaPodOnNode(controlPlaneNodeName())
	if err != nil {
		t.Fatalf("Error when getting antrea-agent pod name: %v", err)
	}
	for _, c := range GetDebugCommands(antctl.CommandList, runtime.ModeAgent) {
		args := []string{}
		if testOptions.enableCoverage {
			antctlCovArgs := antctlCoverageArgs("antctl-coverage")
			args = append(antctlCovArgs, c...)
		} else {
			args = append([]string{"antctl", "-v"}, c...)
		}
		t.Logf("args: %s", args)

		cmd := strings.Join(args, " ")
		t.Run(cmd, func(t *testing.T) {
			stdout, stderr, err := runAntctl(podName, args, data)
			antctlOutput(stdout, stderr, t)
			if err != nil {
				t.Fatalf("Error when running `antctl %s` from %s: %v", c, podName, err)
			}
		})
	}
}

func copyAntctlToNode(data *TestData, nodeName string, antctlName string, nodeAntctlPath string) error {
	podName, err := data.getAntreaPodOnNode(controlPlaneNodeName())
	if err != nil {
		return fmt.Errorf("error when retrieving Antrea Controller Pod name: %v", err)
	}
	// Just try our best to clean up.
	RunCommandOnNode(nodeName, fmt.Sprintf("rm -f %s", nodeAntctlPath))
	// Copy antctl from the controller Pod to the Node.
	cmd := fmt.Sprintf("kubectl cp %s/%s:/usr/local/bin/%s %s", antreaNamespace, podName, antctlName, nodeAntctlPath)
	rc, stdout, stderr, err := RunCommandOnNode(nodeName, cmd)
	if err != nil {
		return fmt.Errorf("error when running command '%s' on Node: %v", cmd, err)
	}
	if rc != 0 {
		return fmt.Errorf("error when copying %s from %s, stdout: <%v>, stderr: <%v>", antctlName, podName, stdout, stderr)
	}
	// Make sure the antctl binary is executable on the Node.
	cmd = fmt.Sprintf("chmod +x %s", nodeAntctlPath)
	rc, stdout, stderr, err = RunCommandOnNode(nodeName, cmd)
	if err != nil {
		return fmt.Errorf("error when running command '%s' on Node: %v", cmd, err)
	}
	if rc != 0 {
		return fmt.Errorf("error when making antctl executable on Node, stdout: <%v>, stderr: <%v>", stdout, stderr)
	}
	return nil
}

// TestAntctlControllerRemoteAccess ensures antctl is able to be run outside of
// the kubernetes cluster. It uses the antctl client binary copied from the controller
// Pod.
func TestAntctlControllerRemoteAccess(t *testing.T) {
	data, err := setupTest(t)
	if err != nil {
		t.Fatalf("Error when setting up test: %v", err)
	}
	defer teardownTest(t, data)
	antctlName := "antctl"
	nodeAntctlPath := "~/antctl"
	if testOptions.enableCoverage {
		antctlName = "antctl-coverage"
		nodeAntctlPath = "~/antctl-coverage"
	}
	if err := copyAntctlToNode(data, controlPlaneNodeName(), antctlName, nodeAntctlPath); err != nil {
		t.Fatalf("Cannot copy %s on control-plane Node: %v", antctlName, err)
	}

	testCmds := []cmdAndReturnCode{}
	// Add all controller commands.
	for _, c := range GetDebugCommands(antctl.CommandList, runtime.ModeController) {
		cmd := append([]string{nodeAntctlPath, "-v"}, c...)
		if testOptions.enableCoverage {
			antctlCovArgs := antctlCoverageArgs(nodeAntctlPath)
			cmd = append(antctlCovArgs, c...)
		}
		testCmds = append(testCmds, cmdAndReturnCode{args: cmd, expectedReturnCode: 0})
	}
	if testOptions.enableCoverage {
		testCmds = append(testCmds,
			// Malformed config
			cmdAndReturnCode{
				args:               []string{nodeAntctlPath, "version", "--kubeconfig", "/dev/null"},
				expectedReturnCode: 1,
			},
		)

	} else {
		testCmds = append(testCmds,
			// Malformed config
			cmdAndReturnCode{
				args:               []string{nodeAntctlPath, "-v", "version", "--kubeconfig", "/dev/null"},
				expectedReturnCode: 1,
			},
		)
	}

	for _, tc := range testCmds {
		cmd := strings.Join(tc.args, " ")
		t.Run(cmd, func(t *testing.T) {
			rc, stdout, stderr, err := RunCommandOnNode(controlPlaneNodeName(), cmd)
			antctlOutput(stdout, stderr, t)
			assert.Equal(t, tc.expectedReturnCode, rc)
			if err != nil {
				t.Fatalf("Error when running `%s` from %s: %v", cmd, controlPlaneNodeName(), err)
			}
		})
	}
}

// TestAntctlVerboseMode ensures no unexpected outputs during the execution of
// the antctl client.
func TestAntctlVerboseMode(t *testing.T) {
	data, err := setupTest(t)
	if err != nil {
		t.Fatalf("Error when setting up test: %v", err)
	}
	defer teardownTest(t, data)
	podName, err := data.getAntreaPodOnNode(controlPlaneNodeName())
	require.Nil(t, err, "Error when retrieving antrea controller pod name")
	for _, tc := range []struct {
		name      string
		hasStderr bool
		commands  []string
	}{
		{name: "RootNonVerbose", hasStderr: false, commands: []string{"antctl"}},
		{name: "RootVerbose", hasStderr: false, commands: []string{"antctl", "-v"}},
		{name: "CommandNonVerbose", hasStderr: false, commands: []string{"antctl", "version"}},
		{name: "CommandVerbose", hasStderr: true, commands: []string{"antctl", "-v", "version"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Running commnand `%s` on pod %s", tc.commands, podName)
			stdout, stderr, err := runAntctl(podName, tc.commands, data)
			antctlOutput(stdout, stderr, t)
			assert.Nil(t, err)
			if !tc.hasStderr {
				assert.Empty(t, stderr)
			} else {
				assert.NotEmpty(t, stderr)
			}
		})
	}
}

// runAntctProxy runs the antctl reverse proxy on the provided Node; to stop the
// proxy call the returned function.
func runAntctProxy(nodeName string, antctlName string, nodeAntctlPath string, proxyPort int, agentNodeName string) (func() error, error) {
	waitCh := make(chan struct{})
	go func() {
		proxyCmd := []string{nodeAntctlPath, "proxy"}
		if agentNodeName == "" {
			proxyCmd = append(proxyCmd, "--controller")
		} else {
			proxyCmd = append(proxyCmd, "--agent-node", agentNodeName)
		}
		cmd := strings.Join(proxyCmd, " ")
		RunCommandOnNode(nodeName, cmd)
		waitCh <- struct{}{}
	}()
	// wait for 1 second to make sure the proxy is running and to detect if
	// it errors on start.
	time.Sleep(time.Second)
	cmd := fmt.Sprintf("pgrep %s", antctlName)
	rc, stdout, stderr, err := RunCommandOnNode(nodeName, cmd)
	if err != nil {
		return nil, fmt.Errorf("error when running command '%s' on Node: %v", cmd, err)
	}
	if rc != 0 {
		return nil, fmt.Errorf("error when retrieving 'antctl proxy' PID, stdout: <%v>, stderr: <%v>", stdout, stderr)
	}
	pid := strings.TrimSpace(stdout)
	return func() error {
		cmd := fmt.Sprintf("kill -INT %s", pid)
		rc, stdout, stderr, err := RunCommandOnNode(nodeName, cmd)
		if err != nil {
			return fmt.Errorf("error when running command '%s' on Node: %v", cmd, err)
		}
		if rc != 0 {
			return fmt.Errorf("error when stopping PID %s, stdout: <%v>, stderr: <%v>", pid, stdout, stderr)
		}
		<-waitCh
		return nil
	}, nil
}

// TestAntctlProxy validates "antctl proxy" for both the Antrea Controller and
// Agent API.
func TestAntctlProxy(t *testing.T) {
	skipIfIPv6Cluster(t)
	const proxyPort = 8001

	data, err := setupTest(t)
	if err != nil {
		t.Fatalf("Error when setting up test: %v", err)
	}
	defer teardownTest(t, data)
	antctlName := "antctl"
	nodeAntctlPath := "~/antctl"
	if testOptions.enableCoverage {
		antctlName = "antctl-coverage"
		nodeAntctlPath = "~/antctl-coverage"
	}
	if err := copyAntctlToNode(data, controlPlaneNodeName(), antctlName, nodeAntctlPath); err != nil {
		t.Fatalf("Cannot copy %s on control-plane Node: %v", antctlName, err)
	}

	checkAPIAccess := func() error {
		t.Logf("Checking for API access through antctl proxy")
		cmd := fmt.Sprintf("curl 127.0.0.1:%d/apis", proxyPort)
		rc, stdout, stderr, err := RunCommandOnNode(controlPlaneNodeName(), cmd)
		if err != nil {
			return fmt.Errorf("error when running command '%s' on Node: %v", cmd, err)
		}
		if rc != 0 {
			return fmt.Errorf("error when accessing API, stdout: <%v>, stderr: <%v>", stdout, stderr)
		}
		return nil
	}

	testcases := []struct {
		name          string
		agentNodeName string
	}{
		{"ControllerProxy", ""},
		{"AgentProxy", controlPlaneNodeName()},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Starting antctl proxy")
			stopProxyFn, err := runAntctProxy(controlPlaneNodeName(), antctlName, nodeAntctlPath, proxyPort, "")
			if err != nil {
				t.Fatalf("Could not start antctl proxy: %v", err)
			}
			if err := checkAPIAccess(); err != nil {
				t.Errorf("API check failed: %v", err)
			}
			t.Logf("Stopping antctl proxy")
			if err := stopProxyFn(); err != nil {
				t.Errorf("Error when stopping antctl proxy: %v", err)
			}
		})
	}
}
