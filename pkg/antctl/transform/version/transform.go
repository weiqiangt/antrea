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

package version

import (
	"encoding/json"
	"io"
	"io/ioutil"

	"k8s.io/klog"

	"github.com/vmware-tanzu/antrea/pkg/antctl/handlers"
	clusterinfov1beta1 "github.com/vmware-tanzu/antrea/pkg/apis/clusterinformation/v1beta1"
	pversion "github.com/vmware-tanzu/antrea/pkg/version"
)

type Response struct {
	AgentVersion      string `json:"agentVersion,omitempty" yaml:"agentVersion,omitempty"`
	ControllerVersion string `json:"controllerVersion,omitempty" yaml:"controllerVersion,omitempty"`
	AntctlVersion     string `json:"antctlVersion,omitempty" yaml:"antctlVersion,omitempty"`
}

// AgentVersion is the AddonTransform for the version command. This function
// will try to parse the response as a AgentVersionResponse and then populate
// it with the version of antctl to a transformedVersionResponse object.
func AgentTransform(reader io.Reader, _ bool) (interface{}, error) {
	b, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	klog.Infof("version transform received: %s", string(b))
	cv := new(handlers.AgentVersionResponse)
	err = json.Unmarshal(b, cv)
	if err != nil {
		return nil, err
	}
	resp := &Response{
		AgentVersion:  cv.AgentVersion,
		AntctlVersion: pversion.GetFullVersion(),
	}
	return resp, nil
}

func ControllerTransform(reader io.Reader, _ bool) (interface{}, error) {
	b, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	klog.Infof("version transform received: %s", string(b))
	controllerInfo := new(clusterinfov1beta1.AntreaControllerInfo)
	err = json.Unmarshal(b, controllerInfo)
	if err != nil {
		return nil, err
	}
	resp := &Response{
		ControllerVersion: controllerInfo.Version,
		AntctlVersion:     pversion.GetFullVersion(),
	}
	return resp, nil
}
