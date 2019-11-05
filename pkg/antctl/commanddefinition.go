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
	"io/ioutil"
	"os"
	"reflect"
	"strings"

	"github.com/fatih/structtag"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
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
//  antctl <commandGroup> <command>
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
	// TagKey is the tag name of the antctl specific annotation.
	// For example:
	// 	type FooResponse struct {
	// 		Bar BarType `antctl:"key"`
	// 	}
	// If the field is annotated with antctl:"key", the framework assumes this field
	// could be used to retrieve a unique Response, thus the framework will generate
	// corresponding arg options to the cobra.Command.
	TagKey = "antctl"
	// TagOptionKeyArg is the option for antctl annotation. It tells the antctl
	// the field is a primary key.
	TagOptionKeyArg = "key"
)

// ArgOption describes one argument which can be used in a RequestOption.
type ArgOption struct {
	Name      string
	FieldName string
	Usage     string
	Key       bool
}

// commandDefinition defines options to create a cobra.Command for a antctl client.
type commandDefinition struct {
	// Cobra related
	Use     string
	Short   string
	Long    string
	Example string // It will be filled with generated examples if it is not provided.
	// commandGroup represents the group of the command.
	CommandGroup commandGroup
	// Controller should be true if this command works for antrea-controller.
	Controller bool
	// Agent should be true if this command works for antrea-agent.
	Agent bool
	// Singleton should be true if the handler always returns single object. The
	// antctl assumes the response data as a slice of the objects by default.
	Singleton bool
	// antctl will not add formatter option for this command if it is true.
	PlainText bool
	// The handler factory of the command.
	HandlerFactory handlers.Factory
	// ResponseStruct is the response object data struct of the command. This struct
	// is no need to be used for the response struct of the handler if a AddonTransform
	// is set.
	ResponseStruct interface{}
	// AddonTransform is used to transform or update the response data received
	// from the handler.
	AddonTransform func(reader io.Reader, single bool) (interface{}, error)
}

// applySubCommandToRoot applies the commandDefinition to a cobra.Command with
// the client. It populates basic fields of a cobra.Command and creates specific
// the RunE function for it according the commandDefinition.
func (cd *commandDefinition) applySubCommandToRoot(root *cobra.Command, client *client, isAgent bool) {
	cmd := &cobra.Command{
		Use:   cd.Use,
		Short: cd.Short,
		Long:  cd.Long,
	}
	renderDescription(cmd, isAgent)

	cd.applyFlagsToCommand(cmd)
	var keyArgOption *ArgOption
	argOpts := cd.ArgOptions()
	for i := range argOpts {
		a := argOpts[i]
		if a.Key {
			cmd.Use += fmt.Sprintf(" [%s]", a.Name)
			cmd.Long += "\n\nArgs:\n" + fmt.Sprintf("  %s\t%s", a.Name, a.Usage)
			// save the key arg option
			keyArgOption = a
		}
	}

	if groupCommand, ok := groupCommands[cd.CommandGroup]; ok {
		groupCommand.AddCommand(cmd)
	} else {
		root.AddCommand(cmd)
	}
	cd.applyExampleToCommand(cmd, keyArgOption)

	// Set key arg length validator to the command.
	if keyArgOption != nil {
		cmd.Args = cobra.MaximumNArgs(1)
	} else {
		cmd.Args = cobra.NoArgs
	}

	cmd.RunE = cd.newCommandRunE(keyArgOption, client)
}

// validate checks if the commandDefinition is valid.
func (cd *commandDefinition) validate() []error {
	var errs []error
	if len(cd.Use) == 0 { // must have a use string
		errs = append(errs, fmt.Errorf("the command does not have name"))
	}
	if cd.HandlerFactory == nil { // must have a handler
		errs = append(errs, fmt.Errorf("%s: no handler specified", cd.Use))
	}
	if cd.ResponseStruct == nil && !cd.PlainText { // must define its response struct
		errs = append(errs, fmt.Errorf("%s: no response struct", cd.Use))
	}
	// Only one key arg is allowed.
	var hasKey bool
	for _, arg := range cd.ArgOptions() {
		if arg.Key && hasKey {
			errs = append(errs, fmt.Errorf("%s: has more than one key field", cd.Use))
			break
		} else if arg.Key {
			hasKey = true
		}
	}
	return errs
}

// ArgOptions returns the list of arguments that could be used in a commandDefinition.
func (cd *commandDefinition) ArgOptions() []*ArgOption {
	var ret []*ArgOption
	if cd.ResponseStruct == nil {
		return ret
	}
	t := reflect.Indirect(reflect.ValueOf(cd.ResponseStruct)).Type()

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		argOpt := &ArgOption{FieldName: f.Name}

		tags, err := structtag.Parse(string(f.Tag))
		if err != nil { // Broken cli tags, skip this field
			continue
		}

		jsonTag, err := tags.Get("json")
		if err != nil {
			argOpt.Name = strings.ToLower(f.Name)
		} else {
			argOpt.Name = jsonTag.Name
		}

		cliTag, err := tags.Get(TagKey)
		if err == nil {
			argOpt.Key = cliTag.Name == TagOptionKeyArg
			argOpt.Usage = strings.Join(cliTag.Options, ", ")
		}

		ret = append(ret, argOpt)
	}
	return ret
}

// decode parses the data in reader to the ResponseStruct. If the single is false,
// the return type is []ResponseStruct. Otherwise, the response struct is ResponseStruct.
func (cd *commandDefinition) decode(r io.Reader, single bool) (interface{}, error) {
	var result interface{}
	t := reflect.Indirect(reflect.ValueOf(cd.ResponseStruct)).Type()

	if single || cd.Singleton {
		ref := reflect.New(t)
		err := json.NewDecoder(r).Decode(ref.Interface())
		if err != nil {
			return nil, err
		}
		result = ref.Interface()
	} else {
		ref := reflect.New(reflect.SliceOf(t))
		err := json.NewDecoder(r).Decode(ref.Interface())
		if err != nil {
			return nil, err
		}
		result = reflect.Indirect(ref).Interface()
	}

	return result, nil
}

// format transforms the data in resp and outputs to the writer. If the AddonTransform
// is set, it will use the function to transform the data first. If the commandDefinition
// specified PlainText, it will output to writer without using any formatter.
func (cd *commandDefinition) format(resp io.Reader, writer io.Writer, ft formatterType, single bool) (err error) {
	var obj interface{}
	if cd.AddonTransform != nil { // Use AddonTransform to decode the data, if it exists.
		obj, err = cd.AddonTransform(resp, single)
		klog.Infof("After transforming %v", obj)
	}
	if err != nil {
		return fmt.Errorf("error when doing local transform: %w", err)
	}

	if cd.PlainText { // Output PlainText directly
		if cd.AddonTransform == nil {
			data, err := ioutil.ReadAll(resp)
			if err != nil {
				return fmt.Errorf("error when reading plain test from response: %w", err)
			}
			_, err = fmt.Fprintln(writer, string(data))
			if err != nil {
				return fmt.Errorf("error when outputing plain text in local: %w", err)
			}
			return nil
		}
		_, err := fmt.Fprintln(writer, obj)
		if err != nil {
			return fmt.Errorf("error when outputing plain text: %w", err)
		}
		return err
	}
	if cd.AddonTransform == nil { // Decode the data if there is no AddonTransform.
		obj, err = cd.decode(resp, single)
	}
	if err != nil {
		return fmt.Errorf("error when decoding response: %w", err)
	}
	// Output structure data in format
	switch ft {
	case jsonFormatter:
		var output bytes.Buffer
		err := json.NewEncoder(&output).Encode(obj)
		if err != nil {
			return fmt.Errorf("error when encoding data in json: %w", err)
		}
		var prettifiedBuf bytes.Buffer
		err = json.Indent(&prettifiedBuf, output.Bytes(), "", "  ")
		if err != nil {
			return fmt.Errorf("error when formatting outputing in json: %w", err)
		}
		_, err = io.Copy(writer, &prettifiedBuf)
		if err != nil {
			return fmt.Errorf("error when outputing in json format: %w", err)
		}
	case yamlFormatter:
		err := yaml.NewEncoder(writer).Encode(obj)
		if err != nil {
			return fmt.Errorf("error when outputing in yaml format: %w", err)
		}
	case tableFormatter: // TODO: Add table formatter
		panic("Implement it")
	default:
		return fmt.Errorf("unsupport format type: %v", ft)
	}
	return nil
}

// newCommandRunE creates the RunE function for the command. The RunE function
// checks the args according to ArgOption and flags.
func (cd *commandDefinition) newCommandRunE(key *ArgOption, c *client) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		argMap := make(map[string]string)
		if len(args) > 0 {
			argMap[key.Name] = args[0]
		}
		kubeconfigPath, _ := cmd.Flags().GetString("kubeconfig")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		resp, err := c.Request(&RequestOption{
			Kubeconfig: kubeconfigPath,
			Name:       cmd.Name(),
			Args:       argMap,
			TimeOut:    timeout,
		})
		if err != nil {
			return err
		}
		single := len(argMap) != 0
		outputFormat, err := cmd.Flags().GetString("output")
		if err != nil {
			return err
		}
		return cd.format(resp, os.Stdout, formatterType(outputFormat), single)
	}
}

// applyFlagsToCommand sets up flags for the command.
func (cd *commandDefinition) applyFlagsToCommand(cmd *cobra.Command) {
	if !cd.PlainText {
		cmd.Flags().StringP("output", "o", "json", "output format: json|yaml|table")
	}
}

// applyExampleToCommand generates examples according to the commandDefinition.
// It only creates for commands which specified ResponseStruct. If the Singleton
// is specified, it only creates one example to retrieve the single object. Otherwise,
// it will generates examples about retrieving single object according to the key
// ArgOption and retrieving the object list.
func (cd *commandDefinition) applyExampleToCommand(cmd *cobra.Command, key *ArgOption) {
	if len(cd.Example) != 0 {
		cmd.Example = cd.Example
		return
	}
	if cd.ResponseStruct == nil {
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
	typeName := reflect.Indirect(reflect.ValueOf(cd.ResponseStruct)).Type().Name()
	dataName := strings.ToLower(strings.TrimSuffix(typeName, "Response"))

	if cd.Singleton {
		fmt.Fprintf(&buf, "  Get the %s\n", dataName)
		fmt.Fprintf(&buf, "  $ %s\n", strings.Join(commands, " "))
	} else {
		if key != nil {
			fmt.Fprintf(&buf, "  Get a %s\n", dataName)
			fmt.Fprintf(&buf, "  $ %s [%s]\n", strings.Join(commands, " "), key.Name)
		}
		fmt.Fprintf(&buf, "  Get the list of %s\n", dataName)
		fmt.Fprintf(&buf, "  $ %s\n", strings.Join(commands, " "))
	}

	cmd.Example = buf.String()
}
