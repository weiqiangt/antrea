// Copyright 2020 Antrea Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1beta1 "github.com/vmware-tanzu/antrea/pkg/apis/clusterinformation/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeAntreaAgentInfos implements AntreaAgentInfoInterface
type FakeAntreaAgentInfos struct {
	Fake *FakeClusterinformationV1beta1
}

var antreaagentinfosResource = schema.GroupVersionResource{Group: "clusterinformation.antrea.tanzu.vmware.com", Version: "v1beta1", Resource: "antreaagentinfos"}

var antreaagentinfosKind = schema.GroupVersionKind{Group: "clusterinformation.antrea.tanzu.vmware.com", Version: "v1beta1", Kind: "AntreaAgentInfo"}

// Get takes name of the antreaAgentInfo, and returns the corresponding antreaAgentInfo object, and an error if there is any.
func (c *FakeAntreaAgentInfos) Get(name string, options v1.GetOptions) (result *v1beta1.AntreaAgentInfo, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(antreaagentinfosResource, name), &v1beta1.AntreaAgentInfo{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.AntreaAgentInfo), err
}

// List takes label and field selectors, and returns the list of AntreaAgentInfos that match those selectors.
func (c *FakeAntreaAgentInfos) List(opts v1.ListOptions) (result *v1beta1.AntreaAgentInfoList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(antreaagentinfosResource, antreaagentinfosKind, opts), &v1beta1.AntreaAgentInfoList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1beta1.AntreaAgentInfoList{ListMeta: obj.(*v1beta1.AntreaAgentInfoList).ListMeta}
	for _, item := range obj.(*v1beta1.AntreaAgentInfoList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested antreaAgentInfos.
func (c *FakeAntreaAgentInfos) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(antreaagentinfosResource, opts))
}

// Create takes the representation of a antreaAgentInfo and creates it.  Returns the server's representation of the antreaAgentInfo, and an error, if there is any.
func (c *FakeAntreaAgentInfos) Create(antreaAgentInfo *v1beta1.AntreaAgentInfo) (result *v1beta1.AntreaAgentInfo, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(antreaagentinfosResource, antreaAgentInfo), &v1beta1.AntreaAgentInfo{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.AntreaAgentInfo), err
}

// Update takes the representation of a antreaAgentInfo and updates it. Returns the server's representation of the antreaAgentInfo, and an error, if there is any.
func (c *FakeAntreaAgentInfos) Update(antreaAgentInfo *v1beta1.AntreaAgentInfo) (result *v1beta1.AntreaAgentInfo, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(antreaagentinfosResource, antreaAgentInfo), &v1beta1.AntreaAgentInfo{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.AntreaAgentInfo), err
}

// Delete takes name of the antreaAgentInfo and deletes it. Returns an error if one occurs.
func (c *FakeAntreaAgentInfos) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteAction(antreaagentinfosResource, name), &v1beta1.AntreaAgentInfo{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeAntreaAgentInfos) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(antreaagentinfosResource, listOptions)

	_, err := c.Fake.Invokes(action, &v1beta1.AntreaAgentInfoList{})
	return err
}

// Patch applies the patch and returns the patched antreaAgentInfo.
func (c *FakeAntreaAgentInfos) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.AntreaAgentInfo, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(antreaagentinfosResource, name, pt, data, subresources...), &v1beta1.AntreaAgentInfo{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.AntreaAgentInfo), err
}
