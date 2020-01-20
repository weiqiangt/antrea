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

package flowtable

// FlowTableResponse defines the response data struct of the flow-table command.
type Response struct {
	TableID   string `json:"tableID" yaml:"tableID" antctl:"key,The table id of the flow table"`
	FlowCount uint   `json:"flowCount" yaml:"flowCount"`
}
