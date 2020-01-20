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

package addressgroup

import (
	"encoding/json"
	"io"
	"net"
	"reflect"

	networkingv1beta1 "github.com/vmware-tanzu/antrea/pkg/apis/networking/v1beta1"
)

type Response struct {
	Name        string   `json:"name" yaml:"name"`
	IPAddresses []string `json:"ipAddresses" yaml:"ipAddresses"`
}

func listTransform(groups *networkingv1beta1.AddressGroupList) []Response {
	result := []Response{}
	for _, item := range groups.Items {
		result = append(result, objectTransform(&item))
	}
	return result
}

func objectTransform(group *networkingv1beta1.AddressGroup) Response {
	ips := []string{}
	for _, ip := range group.IPAddresses {
		ips = append(ips, net.IP(ip).String())
	}
	return Response{Name: group.Name, IPAddresses: ips}
}

func Transform(reader io.Reader, single bool) (interface{}, error) {
	var refType reflect.Type
	if single {
		refType = reflect.TypeOf(networkingv1beta1.AddressGroup{})
	} else {
		refType = reflect.TypeOf(networkingv1beta1.AddressGroupList{})
	}
	refVal := reflect.New(refType)
	if err := json.NewDecoder(reader).Decode(refVal.Interface()); err != nil {
		return nil, err
	}
	if single {
		return objectTransform(refVal.Interface().(*networkingv1beta1.AddressGroup)), nil
	}
	return listTransform(refVal.Interface().(*networkingv1beta1.AddressGroupList)), nil
}
