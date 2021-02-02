// Copyright 2019 Antrea Authors
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

package framework

import (
	"flag"
	"fmt"
	"math"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog"

	"github.com/vmware-tanzu/antrea/pkg/antctl/runtime"
)

// CommandList organizes commands definitions.
// It is the protocol for a pair of antctl client and server.
type CommandList struct {
	Definitions []CommandDefinition
	RawCommands []RawCommand
	Codec       serializer.CodecFactory
}

func (cl *CommandList) applyPersistentFlagsToRoot(root *cobra.Command) {
	root.PersistentFlags().BoolP("verbose", "v", false, "enable verbose output")
	root.PersistentFlags().StringP("kubeconfig", "k", "", "absolute path to the kubeconfig file")
	root.PersistentFlags().DurationP("timeout", "t", 0, "time limit of the execution of the command")
	root.PersistentFlags().StringP("server", "s", "", "address and port of the API server, taking precedence over the default endpoint and the one set in kubeconfig")
}

// ApplyToRootCommand applies the CommandList to the root cobra command, it applies
// each commandDefinition of it to the root command as a sub-command.
func (cl *CommandList) ApplyToRootCommand(wrapper configWrapperType, root *cobra.Command) {
	c := &client{configWrapper: wrapper}
	for _, groupCommand := range GroupCommands {
		root.AddCommand(groupCommand)
	}
	for i := range cl.Definitions {
		def := &cl.Definitions[i]
		if (runtime.Mode == runtime.ModeAgent && def.AgentEndpoint == nil) ||
			(runtime.Mode == runtime.ModeController && def.ControllerEndpoint == nil) {
			continue
		}
		def.applySubCommandToRoot(root, c)
		klog.Infof("Added command %s", def.Use)
	}
	cl.applyPersistentFlagsToRoot(root)
	for _, cmd := range cl.RawCommands {
		if (runtime.Mode == runtime.ModeAgent && cmd.SupportAgent) ||
			(runtime.Mode == runtime.ModeController && cmd.SupportController) {
			root.AddCommand(cmd.CobraCommand)
		}
	}

	root.SilenceUsage = true
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		enableVerbose, err := root.PersistentFlags().GetBool("verbose")
		if err != nil {
			return err
		}
		err = flag.Set("logtostderr", fmt.Sprint(enableVerbose))
		if err != nil {
			return err
		}
		if enableVerbose {
			err := flag.Set("v", fmt.Sprint(math.MaxInt32))
			if err != nil {
				return err
			}
		}
		return nil
	}
	renderDescription(root)
}

// validate checks the validation of the CommandList.
func (cl *CommandList) Validate() []error {
	var errs []error
	if len(cl.Definitions) == 0 {
		return []error{fmt.Errorf("no command found in the command list")}
	}
	for i, c := range cl.Definitions {
		for _, err := range c.validate() {
			errs = append(errs, fmt.Errorf("#%d command<%s>: %w", i, c.Use, err))
		}
	}
	return errs
}

// renderDescription replaces placeholders ${component} in Short and Long of a command
// to the determined component during runtime.
func renderDescription(command *cobra.Command) {
	command.Short = strings.ReplaceAll(command.Short, "${component}", runtime.Mode)
	command.Long = strings.ReplaceAll(command.Long, "${component}", runtime.Mode)
}
