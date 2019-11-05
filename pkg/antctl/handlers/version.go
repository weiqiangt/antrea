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

package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/vmware-tanzu/antrea/pkg/monitor"
)

var _ Factory = new(Version)

// ComponentVersionResponse describes the internal response struct of version command.
// It only contains the version of the component the antctl server be installed
// in, either the agent or the controller.
// This struct is not the response struct of the version command. The version command
// definition has the AddonTransform field which will populate the antctl version
// to the final response.
type ComponentVersionResponse struct {
	AgentVersion      string `json:"agentVersion,omitempty" yaml:"agentVersion,omitempty"`
	ControllerVersion string `json:"controllerVersion,omitempty" yaml:"controllerVersion,omitempty"`
}

// Version is the implementation of Factory for version command.
type Version struct{}

// Handler returns the function which can handle queries issued by version commands,
// the handler function populate component's version to the ComponentVersionResponse.
func (v *Version) Handler(aq monitor.AgentQuerier, cq monitor.ControllerQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var m ComponentVersionResponse

		if aq != nil {
			m.AgentVersion = aq.GetVersion()
		} else if cq != nil {
			m.ControllerVersion = cq.GetVersion()
		}

		err := json.NewEncoder(w).Encode(m)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}
