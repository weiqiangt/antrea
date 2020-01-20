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

package antctl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"strings"
	"text/tabwriter"

	"github.com/fatih/structtag"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog"

	"github.com/vmware-tanzu/antrea/pkg/antctl/handlers"
)

type formatterType string

const (
	jsonFormatter  formatterType = "json"
	yamlFormatter  formatterType = "yaml"
	tableFormatter formatterType = "table"
)

// commandGroup is used to group commands, it could be specified in commandDefinition.
// The default commandGroup of a commandDefinition is `flat` which means the command
// is a direct sub-command of the root command. For any other commandGroup, the
// antctl framework will generate a same name sub-command of the root command for
// each of them, any commands specified as one of these group will need to be invoked
// as:
//   antctl <commandGroup> <command>
type commandGroup uint

const (
	flat commandGroup = iota
	get
)

var groupCommands = map[commandGroup]*cobra.Command{
	get: {
		Use:   "get",
		Short: "Get the status or resource of a topic",
		Long:  "Get the status or resource of a topic",
	},
}

const (
	// tagKey is the tag name of the antctl specific annotation.
	// For example:
	// 	type FooResponse struct {
	// 		Bar BarType `antctl:"key"`
	// 	}
	// If the field is annotated with antctl:"key", the framework assumes this field
	// could be used to retrieve a unique Response, thus the framework will generate
	// corresponding arg options to the cobra.Command.
	tagKey = "antctl"
	// tagOptionKeyArg is the option for antctl annotation. It tells the antctl
	// the field is a primary key.
	tagOptionKeyArg = "key"
)

// argOption describes one argument which can be used in a requestOption.
type argOption struct {
	key       string
	usage     string
	optionals map[string]string
}

// controllerEndpoint is used to specified the API for an antctl running against antrea-controller.
type controllerEndpoint struct {
	groupVersionResource *schema.GroupVersionResource
	resourceName         string
	namespaced           bool
	// AddonTransform is used to transform or update the response data received
	// from the handler, it must returns an interface which has same type as
	// TransformedResponse.
	addonTransform func(reader io.Reader, single bool) (interface{}, error)
}

// agentEndpoint is used to handle requests from an antctl running against antrea-agent.
type agentEndpoint struct {
	// The handler factory of the command.
	HandlerFactory handlers.Factory
	// AddonTransform is used to transform or update the response data received
	// from the handler, it must returns an interface which has same type as
	// TransformedResponse.
	addonTransform func(reader io.Reader, single bool) (interface{}, error)
}

// commandDefinition defines options to create a cobra.Command for an antctl client.
type commandDefinition struct {
	// Cobra related
	use     string
	short   string
	long    string
	example string // It will be filled with generated examples if it is not provided.
	// commandGroup represents the group of the command.
	commandGroup       commandGroup
	controllerEndpoint *controllerEndpoint
	agentEndpoint      *agentEndpoint
	singleObject       bool
	// transformedResponse is the final response struct of the command. If the
	// AddonTransform is set, TransformedResponse is not needed to be used as the
	// response struct of the handler, but it is still needed to guide the formatter.
	// It should always be filled.
	transformedResponse reflect.Type
}

// applySubCommandToRoot applies the commandDefinition to a cobra.Command with
// the client. It populates basic fields of a cobra.Command and creates the
// appropriate RunE function for it according to the commandDefinition.
func (cd *commandDefinition) applySubCommandToRoot(root *cobra.Command, client *client) {
	cmd := &cobra.Command{
		Use:   cd.use,
		Short: cd.short,
		Long:  cd.long,
	}
	renderDescription(cmd)
	cd.applyFlagsToCommand(cmd)

	if groupCommand, ok := groupCommands[cd.commandGroup]; ok {
		groupCommand.AddCommand(cmd)
	} else {
		root.AddCommand(cmd)
	}
	cd.applyExampleToCommand(cmd)

	cmd.RunE = cd.newCommandRunE(client)
}

// validate checks if the commandDefinition is valid.
func (cd *commandDefinition) validate() []error {
	var errs []error
	if len(cd.use) == 0 {
		errs = append(errs, fmt.Errorf("the command does not have name"))
	}
	if cd.transformedResponse == nil {
		errs = append(errs, fmt.Errorf("%s: command does not define output struct", cd.use))
	}
	if cd.agentEndpoint == nil && cd.controllerEndpoint == nil {
		errs = append(errs, fmt.Errorf("%s: command does not define any supported component", cd.use))
	}
	if cd.agentEndpoint != nil && cd.agentEndpoint.HandlerFactory == nil {
		errs = append(errs, fmt.Errorf("%s: command for agent must define a handler as command endpoint", cd.use))
	}
	if cd.controllerEndpoint != nil && cd.controllerEndpoint.groupVersionResource == nil {
		errs = append(errs, fmt.Errorf("%s: command for controller must define an api resource as endpoint", cd.use))
	}
	return errs
}

// argOptions returns the key argument of the commandDefinition. It traverses all
// exported field and return an argOption based on the first field annotated with
// antctl.
func (cd *commandDefinition) argOption() *argOption {
	argOpt := &argOption{optionals: map[string]string{}}
	for i := 0; i < cd.transformedResponse.NumField(); i++ {
		f := cd.transformedResponse.Field(i)
		tags, err := structtag.Parse(string(f.Tag))
		if err != nil { // Broken cli tags, skip this field
			continue
		}
		var name string
		jsonTag, err := tags.Get("json")
		if err != nil {
			name = strings.ToLower(f.Name)
		} else {
			name = jsonTag.Name
		}

		cliTag, err := tags.Get(tagKey)
		if err != nil { // Broken cli tags, skip this field
			continue
		}
		if cliTag.Name != tagOptionKeyArg {
			argOpt.optionals[name] = strings.Join(cliTag.Options, ", ")
		} else {
			argOpt.key = strings.ToLower(f.Name)
			argOpt.usage = strings.Join(cliTag.Options, ", ")
		}
	}
	return argOpt
}

// decode parses the data in reader and converts it to one or more
// TransformedResponse objects. If single is false, the return type is
// []TransformedResponse. Otherwise, the return type is TransformedResponse.
func (cd *commandDefinition) decode(r io.Reader, single bool) (interface{}, error) {
	var refType reflect.Type
	if single {
		refType = cd.transformedResponse
	} else {
		refType = reflect.SliceOf(cd.transformedResponse)
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

func jsonEncode(obj interface{}, output *bytes.Buffer) error {
	if err := json.NewEncoder(output).Encode(obj); err != nil {
		return fmt.Errorf("error when encoding data in json: %w", err)
	}
	return nil
}

func (cd *commandDefinition) jsonOutput(obj interface{}, writer io.Writer) error {
	var output bytes.Buffer
	if err := jsonEncode(obj, &output); err != nil {
		return fmt.Errorf("error when encoding data in json: %w", err)
	}

	var prettifiedBuf bytes.Buffer
	err := json.Indent(&prettifiedBuf, output.Bytes(), "", "  ")
	if err != nil {
		return fmt.Errorf("error when formatting outputing in json: %w", err)
	}
	_, err = io.Copy(writer, &prettifiedBuf)
	if err != nil {
		return fmt.Errorf("error when outputing in json format: %w", err)
	}
	return nil
}

func (cd *commandDefinition) yamlOutput(obj interface{}, writer io.Writer) error {
	err := yaml.NewEncoder(writer).Encode(obj)
	if err != nil {
		return fmt.Errorf("error when outputing in yaml format: %w", err)
	}
	return nil
}

// respTransformer collects output fields in original transformedResponse
// and flattens them. respTransformer realizes this by turning obj into
// JSON and unmarshalling it.
// E.g. agent's transformedVersionResponse will only have two fields after
// transforming: agentVersion and antctlVersion.
func respTransformer(obj interface{}) (interface{}, error) {
	var jsonObj bytes.Buffer
	if err := json.NewEncoder(&jsonObj).Encode(obj); err != nil {
		return nil, fmt.Errorf("error when encoding data in json: %w", err)
	}
	jsonStr := jsonObj.String()

	var target interface{}
	if err := json.Unmarshal([]byte(jsonStr), &target); err != nil {
		return nil, fmt.Errorf("error when unmarshalling data in json: %w", err)
	}
	return target, nil
}

func (cd *commandDefinition) tableOutput(obj interface{}, writer io.Writer) error {
	target, err := respTransformer(obj)
	if err != nil {
		return fmt.Errorf("error when transforming obj: %w", err)
	}

	list, multiple := target.([]interface{})
	var args []string
	if multiple {
		for _, el := range list {
			m := el.(map[string]interface{})
			for k := range m {
				args = append(args, k)
			}
			break
		}
	} else {
		m, _ := target.(map[string]interface{})
		for k := range m {
			args = append(args, k)
		}
	}

	var buffer bytes.Buffer
	for _, arg := range args {
		buffer.WriteString(arg)
		buffer.WriteString("\t")
	}
	attrLine := buffer.String()

	var valLines []string
	if multiple {
		for _, el := range list {
			m := el.(map[string]interface{})
			buffer.Reset()
			for _, k := range args {
				var output bytes.Buffer
				if err = jsonEncode(m[k], &output); err != nil {
					return fmt.Errorf("error when encoding data in json: %w", err)
				}
				buffer.WriteString(strings.Trim(output.String(), "\"\n"))
				buffer.WriteString("\t")
			}
			valLines = append(valLines, buffer.String())
		}
	} else {
		buffer.Reset()
		m, _ := target.(map[string]interface{})
		for _, k := range args {
			var output bytes.Buffer
			if err = jsonEncode(m[k], &output); err != nil {
				return fmt.Errorf("error when encoding : %w", err)
			}
			buffer.WriteString(strings.Trim(output.String(), "\"\n"))
			buffer.WriteString("\t")
		}
		valLines = append(valLines, buffer.String())
	}

	var b bytes.Buffer
	w := tabwriter.NewWriter(&b, 15, 0, 1, ' ', 0)
	fmt.Fprintln(w, attrLine)
	for _, line := range valLines {
		fmt.Fprintln(w, line)
	}
	w.Flush()

	if _, err = io.Copy(writer, &b); err != nil {
		return fmt.Errorf("error when copy output into writer: %w", err)
	}

	return nil
}

// output reads bytes from the resp and outputs the data to the writer in desired
// format. If the AddonTransform is set, it will use the function to transform
// the data first. It will try to output the resp in the format ft specified after
// doing transform.
func (cd *commandDefinition) output(resp io.Reader, writer io.Writer, ft formatterType, single bool) (err error) {
	var obj interface{}

	var addonTransform func(reader io.Reader, single bool) (interface{}, error)
	if runtimeComponent == componentController && cd.controllerEndpoint.addonTransform != nil {
		addonTransform = cd.controllerEndpoint.addonTransform
	} else if runtimeComponent == componentAgent && cd.agentEndpoint.addonTransform != nil {
		addonTransform = cd.agentEndpoint.addonTransform
	}

	if addonTransform == nil { // Decode the data if there is no AddonTransform.
		obj, err = cd.decode(resp, single)
		if err != nil {
			return fmt.Errorf("error when decoding response: %w", err)
		}
	} else {
		obj, err = addonTransform(resp, single)
		if err != nil {
			return fmt.Errorf("error when doing local transform: %w", err)
		}
		klog.Infof("After transforming %v", obj)
	}

	// Output structure data in format
	switch ft {
	case jsonFormatter:
		return cd.jsonOutput(obj, writer)
	case yamlFormatter:
		return cd.yamlOutput(obj, writer)
	case tableFormatter:
		return cd.tableOutput(obj, writer)
	default:
		return fmt.Errorf("unsupport format type: %v", ft)
	}
}

// newCommandRunE creates the RunE function for the command. The RunE function
// checks the args according to argOption and flags.
func (cd *commandDefinition) newCommandRunE(c *client) func(*cobra.Command, []string) error {
	argOpt := cd.argOption()
	return func(cmd *cobra.Command, args []string) error {
		argMap := make(map[string]string)
		if len(args) > 0 {
			argMap[argOpt.key] = args[0]
		}
		for flag := range argOpt.optionals {
			val, err := cmd.Flags().GetString(flag)
			if err == nil && len(val) != 0 {
				argMap[flag] = val
			}
		}
		if cd.controllerEndpoint != nil && cd.controllerEndpoint.namespaced {
			argMap["namespace"], _ = cmd.Flags().GetString("namespace")
		}
		kubeconfigPath, _ := cmd.Flags().GetString("kubeconfig")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		resp, err := c.Request(&requestOption{
			commandDefinition: cd,
			kubeconfig:        kubeconfigPath,
			args:              argMap,
			timeout:           timeout,
		})
		if err != nil {
			return err
		}
		single := len(args) != 0 || (cd.controllerEndpoint != nil && len(cd.controllerEndpoint.resourceName) != 0)
		outputFormat, err := cmd.Flags().GetString("output")
		if err != nil {
			return err
		}
		return cd.output(resp, os.Stdout, formatterType(outputFormat), single)
	}
}

// applyFlagsToCommand sets up args and flags for the command.
func (cd *commandDefinition) applyFlagsToCommand(cmd *cobra.Command) {
	argOpt := cd.argOption()
	if len(argOpt.key) != 0 {
		cmd.Args = cobra.MaximumNArgs(1)
		cmd.Use += fmt.Sprintf(" [%s]", argOpt.key)
		cmd.Long += "\n\nArgs:\n" + fmt.Sprintf("  %s\t%s", argOpt.key, argOpt.usage)
	} else {
		cmd.Args = cobra.NoArgs
	}
	for arg, usage := range argOpt.optionals {
		cmd.Flags().String(arg, "", usage)
	}

	cmd.Flags().StringP("output", "o", "json", "output format: json|yaml|table")
	if cd.controllerEndpoint != nil && cd.controllerEndpoint.namespaced {
		cmd.Flags().StringP("namespace", "n", "default", "specify the namespace of the resource")
	}
}

// applyExampleToCommand generates examples according to the commandDefinition.
// It only creates for commands which specified TransformedResponse. If the singleObject
// is specified, it only creates one example to retrieve the single object. Otherwise,
// it will generates examples about retrieving single object according to the key
// argOption and retrieving the object list.
func (cd *commandDefinition) applyExampleToCommand(cmd *cobra.Command) {
	if len(cd.example) != 0 {
		cmd.Example = cd.example
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
	dataName := strings.ToLower(cd.use)

	if cd.singleObject {
		fmt.Fprintf(&buf, "  Get the %s\n", dataName)
		fmt.Fprintf(&buf, "  $ %s\n", strings.Join(commands, " "))
	} else {
		argOpt := cd.argOption()
		if len(argOpt.key) != 0 {
			fmt.Fprintf(&buf, "  Get a %s\n", dataName)
			fmt.Fprintf(&buf, "  $ %s [%s]\n", strings.Join(commands, " "), argOpt.key)
		}
		fmt.Fprintf(&buf, "  Get the list of %s\n", dataName)
		fmt.Fprintf(&buf, "  $ %s\n", strings.Join(commands, " "))
	}

	cmd.Example = buf.String()
}

func (cd *commandDefinition) agentRequestPath() string {
	if cd.agentEndpoint == nil {
		return ""
	}
	return path.Join("/antctl", cd.use)
}
