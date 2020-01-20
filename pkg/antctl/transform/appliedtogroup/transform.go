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

package appliedtogroup

import (
	"encoding/json"
	"io"
	"reflect"

	networkingv1beta1 "github.com/vmware-tanzu/antrea/pkg/apis/networking/v1beta1"
)

type Response struct {
	Name string                           `json:"name" yaml:"name"`
	Pods []networkingv1beta1.PodReference `json:"pods" yaml:"pods"`
}

func transformList(groups *networkingv1beta1.AppliedToGroupList) []Response {
	result := []Response{}
	for _, group := range groups.Items {
		result = append(result, transformObject(&group))
	}
	return result
}

func transformObject(group *networkingv1beta1.AppliedToGroup) Response {
	if group.Pods == nil {
		group.Pods = []networkingv1beta1.PodReference{}
	}
	return Response{
		Name: group.Name,
		Pods: group.Pods,
	}
}

func Transform(reader io.Reader, single bool) (interface{}, error) {
	var refType reflect.Type
	if single {
		refType = reflect.TypeOf(networkingv1beta1.AppliedToGroup{})
	} else {
		refType = reflect.TypeOf(networkingv1beta1.AppliedToGroupList{})
	}
	refVal := reflect.New(refType)
	if err := json.NewDecoder(reader).Decode(refVal.Interface()); err != nil {
		return nil, err
	}
	if single {
		return transformObject(refVal.Interface().(*networkingv1beta1.AppliedToGroup)), nil
	}
	return transformList(refVal.Interface().(*networkingv1beta1.AppliedToGroupList)), nil
}
