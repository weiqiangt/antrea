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
	"net/http"

	"github.com/vmware-tanzu/antrea/pkg/monitor"
)

// Factory is the interface to generate handlers. The handlers will be used to
// serve requests from the antctl binary which runs against agents.
type Factory interface {
	// Handler returns a net/http.HandlerFunc which will be used to handle
	// requests issued by commands from the agent antctl client.
	// If the command has no AddonTransform, the HandlerFunc need to write the
	// data to the response body in JSON format.
	Handler(aq monitor.AgentQuerier) http.HandlerFunc
}
