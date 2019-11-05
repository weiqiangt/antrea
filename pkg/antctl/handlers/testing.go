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
	"k8s.io/api/core/v1"

	"github.com/vmware-tanzu/antrea/pkg/apis/clusterinformation/v1beta1"
	"github.com/vmware-tanzu/antrea/pkg/monitor"
)

var (
	_ monitor.AgentQuerier      = new(mockAgentQuerier)
	_ monitor.ControllerQuerier = new(mockControllerQuerier)
)

type mockAgentQuerier struct {
	version string
}

func (aq *mockAgentQuerier) GetSelfPod() v1.ObjectReference {
	panic("implement me")
}

func (aq *mockAgentQuerier) GetSelfNode() v1.ObjectReference {
	panic("implement me")
}

func (aq *mockAgentQuerier) GetNetworkPolicyControllerInfo() v1beta1.NetworkPolicyControllerInfo {
	panic("implement me")
}

func (aq *mockAgentQuerier) GetVersion() string {
	return aq.version
}

func (aq *mockAgentQuerier) GetOVSFlowTable() map[string]int32 {
	panic("implement me")
}

func (aq *mockAgentQuerier) GetLocalPodNum() int32 {
	panic("implement me")
}

type mockControllerQuerier struct {
	version string
}

func (cq *mockControllerQuerier) GetSelfPod() v1.ObjectReference {
	panic("implement me")
}

func (cq *mockControllerQuerier) GetSelfNode() v1.ObjectReference {
	panic("implement me")
}

func (cq *mockControllerQuerier) GetNetworkPolicyControllerInfo() v1beta1.NetworkPolicyControllerInfo {
	panic("implement me")
}

func (cq *mockControllerQuerier) GetVersion() string {
	return cq.version
}

func (cq *mockControllerQuerier) GetService() v1.ObjectReference {
	panic("implement me")
}
