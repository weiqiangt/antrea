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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog"

	"github.com/vmware-tanzu/antrea/pkg/antctl/runtime"
)

type FormatterType string
type FormatterFunc func(obj interface{}, writer io.Writer) error

const (
	JsonFormatter  FormatterType = "json"
	YamlFormatter  FormatterType = "yaml"
	TableFormatter FormatterType = "table"
)

// CommandGroup is used to group commands, it could be specified in CommandDefinition.
// The default CommandGroup of a CommandDefinition is `flat` which means the command
// is a direct sub-command of the root command. For any other CommandGroup, the
// antctl framework will generate a same name sub-command of the root command for
// each of them, any commands specified as one of these group will need to be invoked
// as:
//   antctl <CommandGroup> <command>
type CommandGroup uint
type OutputType uint

// There are two output types: single item or list and the actual type is decided by
// OutputType value here and command's arguments.
const (
	// OutputTypeDefault represents the output type is single item if there is an argument
	// and its value is provided. If not, the output type is list.
	OutputTypeDefault OutputType = iota
	// OutputTypeSingle represents the output type is always single item.
	OutputTypeSingle
	// OutputTypeMultiple represents the output type is always list.
	OutputTypeMultiple
)

const (
	CommandGroupFlat CommandGroup = iota
	CommandGroupGet
	CommandGroupQuery
)

var GroupCommands = map[CommandGroup]*cobra.Command{
	CommandGroupGet: {
		Use:   "get",
		Short: "Get the status or resource of a topic",
		Long:  "Get the status or resource of a topic",
	},
	CommandGroupQuery: {
		Use:   "query",
		Short: "Execute a user-provided query",
		Long:  "Execute a user-provided query",
	},
}

type endpointResponder interface {
	GetOutputType() OutputType
	GetFlags() []FlagInfo
}

type ResourceEndpoint struct {
	GroupVersionResource *schema.GroupVersionResource
	ResourceName         string
	Namespaced           bool
	Flags                []FlagInfo
}

func (e *ResourceEndpoint) GetOutputType() OutputType {
	if len(e.ResourceName) != 0 {
		return OutputTypeSingle
	}
	return OutputTypeDefault
}

func (e *ResourceEndpoint) GetFlags() []FlagInfo {
	var flags []FlagInfo
	if len(e.ResourceName) == 0 {
		flags = append(flags, FlagInfo{
			Name:         "name",
			DefaultValue: "",
			Arg:          true,
			Usage:        "Retrieve the resource by name",
		})
	}
	if e.Namespaced {
		flags = append(flags, FlagInfo{
			Name:         "namespace",
			Shorthand:    "n",
			DefaultValue: metav1.NamespaceAll,
			Usage:        "Filter the resource by namespace",
		})
	}
	flags = append(flags, e.Flags...)
	return flags
}

type NonResourceCommandEndpoint struct {
	Path       string
	Flags      []FlagInfo
	OutputType OutputType
}

func (e *NonResourceCommandEndpoint) GetFlags() []FlagInfo {
	return e.Flags
}

func (e *NonResourceCommandEndpoint) GetOutputType() OutputType {
	return e.OutputType
}

// CommandEndpoint is used to specified the API for an antctl running against antrea-controller.
type CommandEndpoint struct {
	ResourceEndpoint    *ResourceEndpoint
	NonResourceEndpoint *NonResourceCommandEndpoint
	Formatters          map[FormatterType]FormatterFunc
	PreferredFormatter  FormatterType
	// AddonTransform is used to transform or update the response data received
	// from the handler, it must returns an interface which has same type as
	// TransformedResponse.
	AddonTransform func(reader io.Reader, single bool, opts map[string]string) (interface{}, error)
}

// FlagInfo represents a command-line flag that can be provided when invoking an antctl command.
type FlagInfo struct {
	Name            string
	Shorthand       string
	DefaultValue    string
	SupportedValues []string
	Arg             bool
	Usage           string
}

// RawCommand defines a full function cobra.Command which lets developers
// write complex client-side tasks. Only the global GetFlags of the antctl framework will
// be passed to the cobra.Command.
type RawCommand struct {
	CobraCommand      *cobra.Command
	SupportAgent      bool
	SupportController bool
}

// CommandDefinition defines options to create a cobra.Command for an antctl client.
type CommandDefinition struct {
	// Cobra related
	Use     string
	Aliases []string
	Short   string
	Long    string
	Example string // It will be filled with generated examples if it is not provided.
	// CommandGroup represents the group of the command.
	CommandGroup       CommandGroup
	ControllerEndpoint *CommandEndpoint
	AgentEndpoint      *CommandEndpoint
	// TransformedResponse is the final response struct of the command. If the
	// AddonTransform is set, TransformedResponse is not needed to be used as the
	// response struct of the handler, but it is still needed to guide the formatter.
	// It should always be filled.
	TransformedResponse reflect.Type
}

func (cd *CommandDefinition) namespaced() bool {
	if runtime.Mode == runtime.ModeAgent {
		return cd.AgentEndpoint != nil && cd.AgentEndpoint.ResourceEndpoint != nil && cd.AgentEndpoint.ResourceEndpoint.Namespaced
	} else if runtime.Mode == runtime.ModeController {
		return cd.ControllerEndpoint != nil && cd.ControllerEndpoint.ResourceEndpoint != nil && cd.ControllerEndpoint.ResourceEndpoint.Namespaced
	}
	return false
}

func (cd *CommandDefinition) getAddonTransform() func(reader io.Reader, single bool, opts map[string]string) (interface{}, error) {
	if runtime.Mode == runtime.ModeAgent && cd.AgentEndpoint != nil {
		return cd.AgentEndpoint.AddonTransform
	} else if runtime.Mode == runtime.ModeController && cd.ControllerEndpoint != nil {
		return cd.ControllerEndpoint.AddonTransform
	}
	return nil
}

func (cd *CommandDefinition) getAllFormatters() map[FormatterType]FormatterFunc {
	if runtime.Mode == runtime.ModeAgent {
		return cd.AgentEndpoint.Formatters
	} else if runtime.Mode == runtime.ModeController {
		return cd.ControllerEndpoint.Formatters
	}
	return nil
}

func (cd *CommandDefinition) getFormatter(formatter FormatterType) FormatterFunc {
	return cd.getAllFormatters()[formatter]
}

func (cd *CommandDefinition) getPreferredFormatterType() FormatterType {
	var f FormatterType
	if runtime.Mode == runtime.ModeAgent {
		f = cd.AgentEndpoint.PreferredFormatter
	} else {
		f = cd.ControllerEndpoint.PreferredFormatter
	}
	if len(f) == 0 {
		f = YamlFormatter
	}
	return f
}

func (cd *CommandDefinition) getEndpoint() endpointResponder {
	if runtime.Mode == runtime.ModeAgent {
		if cd.AgentEndpoint != nil {
			if cd.AgentEndpoint.ResourceEndpoint != nil {
				return cd.AgentEndpoint.ResourceEndpoint
			}
			return cd.AgentEndpoint.NonResourceEndpoint
		}
	} else if runtime.Mode == runtime.ModeController {
		if cd.ControllerEndpoint != nil {
			if cd.ControllerEndpoint.ResourceEndpoint != nil {
				return cd.ControllerEndpoint.ResourceEndpoint
			}
			return cd.ControllerEndpoint.NonResourceEndpoint
		}
	}
	return nil
}

// applySubCommandToRoot applies the CommandDefinition to a cobra.Command with
// the client. It populates basic fields of a cobra.Command and creates the
// appropriate RunE function for it according to the CommandDefinition.
func (cd *CommandDefinition) applySubCommandToRoot(root *cobra.Command, client *client) {
	cmd := &cobra.Command{
		Use:     cd.Use,
		Aliases: cd.Aliases,
		Short:   cd.Short,
		Long:    cd.Long,
	}
	renderDescription(cmd)
	cd.applyFlagsToCommand(cmd)

	if groupCommand, ok := GroupCommands[cd.CommandGroup]; ok {
		groupCommand.AddCommand(cmd)
	} else {
		root.AddCommand(cmd)
	}
	cd.applyExampleToCommand(cmd)

	cmd.RunE = cd.newCommandRunE(client)
}

// validate checks if the CommandDefinition is valid.
func (cd *CommandDefinition) validate() []error {
	var errs []error
	if len(cd.Use) == 0 {
		errs = append(errs, fmt.Errorf("the command does not have name"))
	}
	existingAliases := make(map[string]bool)
	for _, a := range cd.Aliases {
		if a == cd.Use {
			errs = append(errs, fmt.Errorf("%s: command alias is the same with use of the command", cd.Use))
		}
		if _, ok := existingAliases[a]; ok {
			errs = append(errs, fmt.Errorf("%s: command alias is provided twice: %s", cd.Use, a))
		}
		existingAliases[a] = true
	}
	if cd.TransformedResponse == nil {
		errs = append(errs, fmt.Errorf("%s: command does not define output struct", cd.Use))
	}
	if cd.AgentEndpoint == nil && cd.ControllerEndpoint == nil {
		errs = append(errs, fmt.Errorf("%s: command does not define any supported component", cd.Use))
	}
	if cd.AgentEndpoint != nil && cd.AgentEndpoint.NonResourceEndpoint != nil && cd.AgentEndpoint.ResourceEndpoint != nil {
		errs = append(errs, fmt.Errorf("%s: command for agent can only define one endpoint", cd.Use))
	}
	if cd.AgentEndpoint != nil && cd.AgentEndpoint.NonResourceEndpoint == nil && cd.AgentEndpoint.ResourceEndpoint == nil {
		errs = append(errs, fmt.Errorf("%s: command for agent must define one endpoint", cd.Use))
	}
	if cd.ControllerEndpoint != nil && cd.ControllerEndpoint.NonResourceEndpoint != nil && cd.ControllerEndpoint.ResourceEndpoint != nil {
		errs = append(errs, fmt.Errorf("%s: command for controller can only define one endpoint", cd.Use))
	}
	if cd.ControllerEndpoint != nil && cd.ControllerEndpoint.NonResourceEndpoint == nil && cd.ControllerEndpoint.ResourceEndpoint == nil {
		errs = append(errs, fmt.Errorf("%s: command for controller must define one endpoint", cd.Use))
	}
	if cd.AgentEndpoint != nil {
		if len(cd.AgentEndpoint.Formatters) == 0 {
			errs = append(errs, fmt.Errorf("%s: command for Agent is not specified formatters", cd.Use))
		}
		preferredFormatter := cd.AgentEndpoint.PreferredFormatter
		if len(preferredFormatter) == 0 {
			preferredFormatter = YamlFormatter
		}
		if _, ok := cd.AgentEndpoint.Formatters[preferredFormatter]; !ok {
			errs = append(errs, fmt.Errorf("%s: command for Agent does not have the preferred formatter %s", cd.Use, preferredFormatter))
		}
	}
	if cd.ControllerEndpoint != nil {
		if len(cd.AgentEndpoint.Formatters) == 0 {
			errs = append(errs, fmt.Errorf("%s: command for Controller is not specified formatters", cd.Use))
		}
		preferredFormatter := cd.ControllerEndpoint.PreferredFormatter
		if len(preferredFormatter) == 0 {
			preferredFormatter = YamlFormatter
		}
		if _, ok := cd.ControllerEndpoint.Formatters[preferredFormatter]; !ok {
			errs = append(errs, fmt.Errorf("%s: command for Controller does not have the preferred formatter %s", cd.Use, preferredFormatter))
		}
	}
	empty := struct{}{}
	existingFlags := map[string]struct{}{"output": empty, "help": empty, "kubeconfig": empty, "timeout": empty, "verbose": empty}
	if endpoint := cd.getEndpoint(); endpoint != nil {
		for _, f := range endpoint.GetFlags() {
			if len(f.Name) == 0 {
				errs = append(errs, fmt.Errorf("%s: flag name cannot be empty", cd.Use))
			} else {
				if _, ok := existingFlags[f.Name]; ok {
					errs = append(errs, fmt.Errorf("%s: flag redefined: %s", cd.Use, f.Name))
				}
				existingFlags[f.Name] = empty
			}
			if len(f.Shorthand) > 1 {
				errs = append(errs, fmt.Errorf("%s: length of a flag shorthand cannot be larger than 1: %s", cd.Use, f.Shorthand))
			}
		}
	}
	return errs
}

// decode parses the data in reader and converts it to one or more
// TransformedResponse objects. If single is false, the return type is
// []TransformedResponse. Otherwise, the return type is TransformedResponse.
func (cd *CommandDefinition) decode(r io.Reader, single bool) (interface{}, error) {
	var refType reflect.Type
	if single {
		refType = cd.TransformedResponse
	} else {
		refType = reflect.SliceOf(cd.TransformedResponse)
	}
	ref := reflect.New(refType)
	err := json.NewDecoder(r).Decode(ref.Interface())
	if err != nil {
		return nil, err
	}
	if single {
		return ref.Interface(), nil
	}
	return reflect.Indirect(ref).Interface(), nil
}

// output reads bytes from the resp and outputs the data to the writer in desired
// format. If the AddonTransform is set, it will Use the function to transform
// the data first. It will try to output the resp in the format ft specified after
// doing transform.
func (cd *CommandDefinition) output(resp io.Reader, writer io.Writer, ft FormatterFunc, single bool, args map[string]string) (err error) {
	var obj interface{}
	addonTransform := cd.getAddonTransform()

	if addonTransform == nil { // Decode the data if there is no AddonTransform.
		obj, err = cd.decode(resp, single)
		if err == io.EOF {
			// No response returned.
			return nil
		}
		if err != nil {
			return fmt.Errorf("error when decoding response %v: %w", resp, err)
		}
	} else {
		obj, err = addonTransform(resp, single, args)
		if err != nil {
			return fmt.Errorf("error when doing local transform: %w", err)
		}
		klog.Infof("After transforming %v", obj)
	}

	return ft(obj, writer)
}

func (cd *CommandDefinition) collectFlags(cmd *cobra.Command, args []string) (map[string]string, error) {
	argMap := make(map[string]string)
	if endpoint := cd.getEndpoint(); endpoint != nil {
		for _, f := range endpoint.GetFlags() {
			if f.Arg {
				if len(args) > 0 {
					argMap[f.Name] = args[0]
				}
			} else {
				vs, err := cmd.Flags().GetString(f.Name)
				if err == nil && len(vs) != 0 {
					if f.SupportedValues != nil && !cd.validateFlagValue(vs, f.SupportedValues) {
						return nil, fmt.Errorf("unsupported value %s for flag %s", vs, f.Name)
					}
					argMap[f.Name] = vs
					continue
				}
			}
		}
	}
	if cd.namespaced() {
		argMap["namespace"], _ = cmd.Flags().GetString("namespace")
	}
	return argMap, nil
}

func (cd *CommandDefinition) validateFlagValue(val string, supportedValues []string) bool {
	for _, s := range supportedValues {
		if s == val {
			return true
		}
	}
	return false
}

// newCommandRunE creates the RunE function for the command. The RunE function
// checks the args according to argOption and GetFlags.
func (cd *CommandDefinition) newCommandRunE(c *client) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		argMap, err := cd.collectFlags(cmd, args)
		if err != nil {
			return err
		}
		klog.Infof("Args: %v", argMap)
		var argGet bool
		for _, flag := range cd.getEndpoint().GetFlags() {
			if _, ok := argMap[flag.Name]; ok && flag.Arg == true {
				argGet = true
				break
			}
		}
		kubeconfigPath, _ := cmd.Flags().GetString("kubeconfig")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		server, _ := cmd.Flags().GetString("server")
		resp, err := c.request(&requestOption{
			commandDefinition: cd,
			kubeconfig:        kubeconfigPath,
			args:              argMap,
			timeout:           timeout,
			server:            server,
		})
		if err != nil {
			return err
		}
		outputFormat, err := cmd.Flags().GetString("output")
		if err != nil {
			return err
		}
		formatter := cd.getFormatter(FormatterType(outputFormat))
		if formatter == nil {
			return fmt.Errorf("%s is not a valid format", outputFormat)
		}
		isSingle := cd.getEndpoint().GetOutputType() != OutputTypeMultiple && (cd.getEndpoint().GetOutputType() == OutputTypeSingle || argGet)
		return cd.output(resp, os.Stdout, formatter, isSingle, argMap)
	}
}

// applyFlagsToCommand sets up args and GetFlags for the command.
func (cd *CommandDefinition) applyFlagsToCommand(cmd *cobra.Command) {
	var hasFlag bool
	for _, flag := range cd.getEndpoint().GetFlags() {
		if flag.Arg {
			cmd.Args = cobra.MaximumNArgs(1)
			cmd.Use += fmt.Sprintf(" [%s]", flag.Name)
			cmd.Long += fmt.Sprintf("\n\nArgs:\n  %s\t%s", flag.Name, flag.Usage)
			hasFlag = true
		} else {
			cmd.Flags().StringP(flag.Name, flag.Shorthand, flag.DefaultValue, flag.Usage)
		}
	}
	if !hasFlag {
		cmd.Args = cobra.NoArgs
	}
	var formats []string
	for formatType := range cd.getAllFormatters() {
		formats = append(formats, string(formatType))
	}
	cmd.Flags().StringP("output", "o", string(cd.getPreferredFormatterType()), fmt.Sprintf("output format: %s", strings.Join(formats, "|")))
}

// applyExampleToCommand generates examples according to the CommandDefinition.
// It only creates for commands which specified TransformedResponse. If the singleObject
// is specified, it only creates one Example to retrieve the single object. Otherwise,
// it will generates examples about retrieving single object according to the key
// argOption and retrieving the object list.
func (cd *CommandDefinition) applyExampleToCommand(cmd *cobra.Command) {
	if len(cd.Example) != 0 {
		cmd.Example = cd.Example
		return
	}
	var commands []string
	for iter := cmd; iter != nil; iter = iter.Parent() {
		commands = append(commands, iter.Name())
	}
	for i := 0; i < len(commands)/2; i++ {
		commands[i], commands[len(commands)-1-i] = commands[len(commands)-1-i], commands[i]
	}

	var buf bytes.Buffer
	dataName := strings.ToLower(cd.Use)

	if cd.getEndpoint().GetOutputType() == OutputTypeSingle {
		fmt.Fprintf(&buf, "  Get the %s\n", dataName)
		fmt.Fprintf(&buf, "  $ %s\n", strings.Join(commands, " "))
	} else {
		fmt.Fprintf(&buf, "  Get a %s\n", dataName)
		fmt.Fprintf(&buf, "  $ %s [name]\n", strings.Join(commands, " "))
		fmt.Fprintf(&buf, "  Get the list of %s\n", dataName)
		fmt.Fprintf(&buf, "  $ %s\n", strings.Join(commands, " "))
	}

	cmd.Example = buf.String()
}
