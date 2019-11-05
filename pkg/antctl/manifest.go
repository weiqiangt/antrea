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
	"io"
	"io/ioutil"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog"

	"github.com/vmware-tanzu/antrea/pkg/antctl/handlers"
	"github.com/vmware-tanzu/antrea/pkg/client/clientset/versioned/scheme"
	"github.com/vmware-tanzu/antrea/pkg/version"
)

type VersionResponse struct {
	handlers.ComponentVersionResponse `json:",inline" yaml:",inline"`
	AntctlVersion                     string `json:"antctlVersion" yaml:"antctlVersion"`
}

func versionTransform(reader io.Reader, _ bool) (interface{}, error) {
	b, err := ioutil.ReadAll(reader)
	klog.Infof("version transform received: %s", string(b))
	if err != nil {
		return nil, err
	}
	var cv handlers.ComponentVersionResponse
	err = json.Unmarshal(b, &cv)
	if err != nil {
		return nil, err
	}
	return &VersionResponse{
		ComponentVersionResponse: cv,
		AntctlVersion:            version.GetFullVersion(),
	}, nil
}

// CommandList defines all commands could be used for the antctl for both agent and
// controller. The unit test TestManifestValidation ensures it to be valid.
var CommandList = &commandList{
	definitions: []commandDefinition{
		{
			Use:            "version",
			Short:          "Print version information",
			Long:           "Print version information of the antctl and the ${component}",
			HandlerFactory: new(handlers.Version),
			ResponseStruct: new(VersionResponse),
			Agent:          true,
			Controller:     true,
			Singleton:      true,
			CommandGroup:   flat,
			AddonTransform: versionTransform,
		},
	},
	groupVersion: &schema.GroupVersion{Group: "clusterinfo.antrea.tanzu.vmware.com", Version: "v1"},
	codec:        scheme.Codecs,
}
