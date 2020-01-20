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

package networkpolicy

import (
	"encoding/json"
	"io"
	"net"
	"reflect"

	networkingv1beta1 "github.com/vmware-tanzu/antrea/pkg/apis/networking/v1beta1"
)

type ipBlock struct {
	CIDR   string   `json:"cidr" yaml:"cidr"`
	Except []string `json:"except,omitempty" yaml:"except,omitempty"`
}

type networkPolicyPeer struct {
	AddressGroups []string  `json:"addressGroups,omitempty" yaml:"addressGroups,omitempty"`
	IPBlocks      []ipBlock `json:"ipBlocks,omitempty" json:"ipBlocks,omitempty"`
}

type networkPolicyRule struct {
	Direction networkingv1beta1.Direction `json:"direction,omitempty" yaml:"direction,omitempty"`
	From      networkPolicyPeer           `json:"from,omitempty" yaml:"from,omitempty"`
	To        networkPolicyPeer           `json:"to,omitempty" yaml:"to,omitempty"`
	Services  []networkingv1beta1.Service `json:"services,omitempty" yaml:"services,omitempty"`
}

type Response struct {
	Name            string              `json:"name" yaml:"name"`
	Rules           []networkPolicyRule `json:"rules" yaml:"rules"`
	AppliedToGroups []string            `json:"appliedToGroups" yaml:"appliedToGroups"`
}

func transformIPNet(ipNet networkingv1beta1.IPNet) *net.IPNet {
	ip := net.IP(ipNet.IP)
	var bits int
	if ip.To4() != nil {
		bits = net.IPv4len * 8
	} else {
		bits = net.IPv6len * 8
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(int(ipNet.PrefixLength), bits)}
}

func transformIPBlock(block networkingv1beta1.IPBlock) ipBlock {
	except := []string{}
	for i := range block.Except {
		except = append(except, transformIPNet(block.Except[i]).String())
	}

	return ipBlock{
		CIDR:   transformIPNet(block.CIDR).String(),
		Except: except,
	}
}

func transformNetworkPolicyPeer(peer networkingv1beta1.NetworkPolicyPeer) networkPolicyPeer {
	blocks := []ipBlock{}
	for _, originBlock := range peer.IPBlocks {
		blocks = append(blocks, transformIPBlock(originBlock))
	}
	return networkPolicyPeer{AddressGroups: peer.AddressGroups, IPBlocks: blocks}
}

func transformObject(policy *networkingv1beta1.NetworkPolicy) *Response {
	rules := []networkPolicyRule{}
	for _, originRule := range policy.Rules {
		rules = append(rules, networkPolicyRule{
			Direction: originRule.Direction,
			From:      transformNetworkPolicyPeer(originRule.From),
			To:        transformNetworkPolicyPeer(originRule.To),
			Services:  originRule.Services,
		})
	}
	if policy.AppliedToGroups == nil {
		policy.AppliedToGroups = []string{}
	}
	return &Response{
		Name:            policy.Name,
		Rules:           rules,
		AppliedToGroups: policy.AppliedToGroups,
	}
}

func transformList(policyList *networkingv1beta1.NetworkPolicyList) []Response {
	result := []Response{}
	for _, item := range policyList.Items {
		result = append(result, *transformObject(&item))
	}
	return result
}

func Transform(reader io.Reader, single bool) (interface{}, error) {
	var refType reflect.Type
	if single {
		refType = reflect.TypeOf(networkingv1beta1.NetworkPolicy{})
	} else {
		refType = reflect.TypeOf(networkingv1beta1.NetworkPolicyList{})
	}
	refVal := reflect.New(refType)
	if err := json.NewDecoder(reader).Decode(refVal.Interface()); err != nil {
		return nil, err
	}
	if single {
		return transformObject(refVal.Interface().(*networkingv1beta1.NetworkPolicy)), nil
	}
	return transformList(refVal.Interface().(*networkingv1beta1.NetworkPolicyList)), nil
}
