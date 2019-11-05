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
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/server/mux"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog"

	"github.com/vmware-tanzu/antrea/pkg/monitor"
)

// commandList organizes definitions.
// It is the protocol for a pair of antctl client and server.
type commandList struct {
	definitions  []commandDefinition
	groupVersion *schema.GroupVersion
	codec        serializer.CodecFactory
}

// APIPrefix returns the API prefix of the antctl service.
func (cl *commandList) APIPrefix() string {
	return path.Join("/apis", cl.groupVersion.Group, cl.groupVersion.Version)
}

// ApplyToMux adds the handler function of each commandDefinition in the
// commandList to the mux (<api prefix>/<cmd>), it also adds a dummy discovery
// handler(path <api prefix>). Since the antctl service contains only non-resource
// data, we need setup the discovery handler here to satisfy the apiserver.
func (cl *commandList) ApplyToMux(mux *mux.PathRecorderMux, aq monitor.AgentQuerier, cq monitor.ControllerQuerier) {
	for _, cmdOpt := range cl.definitions {
		reqPath := path.Join(cl.APIPrefix(), strings.ToLower(cmdOpt.Use))
		klog.Infof("Adding cli handler %s", reqPath)
		handler := cmdOpt.HandlerFactory.Handler(aq, cq)
		mux.HandleFunc(reqPath, handler)
	}
	// Set up the discovery handler.
	mux.HandleFunc(cl.APIPrefix(), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		l := metav1.APIResourceList{
			TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList", APIVersion: metav1.SchemeGroupVersion.Version},
			GroupVersion: cl.groupVersion.String(),
		}
		if err := json.NewEncoder(w).Encode(l); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
}

func (cl *commandList) applyFlagsToRootCommand(root *cobra.Command) {
	defaultKubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	root.PersistentFlags().BoolP("verbose", "v", false, "enable verbose output")
	root.PersistentFlags().StringP("kubeconfig", "k", defaultKubeconfig, "absolute path to the kubeconfig file")
	root.PersistentFlags().DurationP("timeout", "t", 0, "time limit of the execution of the command")
}

// ApplyToRootCommand applies the commandList to the root cobra applySubCommandToRoot, it applies
// each commandDefinition of it to the root applySubCommandToRoot as a sub-applySubCommandToRoot.
func (cl *commandList) ApplyToRootCommand(root *cobra.Command, isAgent bool, inPod bool) {
	client := &client{
		inPod:        inPod,
		groupVersion: cl.groupVersion,
		codec:        cl.codec,
	}
	for _, groupCommand := range groupCommands {
		root.AddCommand(groupCommand)
	}
	for i := range cl.definitions {
		cmdOpt := cl.definitions[i]
		if (cmdOpt.Agent != isAgent) && (cmdOpt.Controller != !isAgent) {
			continue
		}
		cmdOpt.applySubCommandToRoot(root, client, isAgent)
	}
	cl.applyFlagsToRootCommand(root)
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		enableVerbose, err := root.PersistentFlags().GetBool("verbose")
		if err != nil {
			return err
		}
		err = flag.Set("logtostderr", fmt.Sprint(enableVerbose))
		if err != nil {
			return err
		}
		err = flag.Set("alsologtostderr", fmt.Sprint(enableVerbose))
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
	renderDescription(root, isAgent)
}

// validate if the commandList is correct.
func (cl *commandList) validate() []error {
	var errs []error
	if len(cl.definitions) == 0 { // must has at least one applySubCommandToRoot
		return []error{fmt.Errorf("no applySubCommandToRoot in applySubCommandToRoot bundle")}
	}
	for i, c := range cl.definitions { // each commandDefinition must be valid
		for _, err := range c.validate() {
			errs = append(errs, fmt.Errorf("#%d applySubCommandToRoot<%s>: %w", i, c.Use, err))
		}
	}
	return errs
}

func renderDescription(command *cobra.Command, isAgent bool) {
	var componentName string
	if isAgent {
		componentName = "agent"
	} else {
		componentName = "controller"
	}
	command.Short = strings.ReplaceAll(command.Short, "${component}", componentName)
	command.Long = strings.ReplaceAll(command.Long, "${component}", componentName)
}
