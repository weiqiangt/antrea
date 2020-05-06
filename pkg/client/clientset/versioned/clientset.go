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

package versioned

import (
	clusterinformationv1beta1 "github.com/vmware-tanzu/antrea/pkg/client/clientset/versioned/typed/clusterinformation/v1beta1"
	networkingv1beta1 "github.com/vmware-tanzu/antrea/pkg/client/clientset/versioned/typed/networking/v1beta1"
	systemv1beta1 "github.com/vmware-tanzu/antrea/pkg/client/clientset/versioned/typed/system/v1beta1"
	discovery "k8s.io/client-go/discovery"
	rest "k8s.io/client-go/rest"
	flowcontrol "k8s.io/client-go/util/flowcontrol"
)

type Interface interface {
	Discovery() discovery.DiscoveryInterface
	ClusterinformationV1beta1() clusterinformationv1beta1.ClusterinformationV1beta1Interface
	NetworkingV1beta1() networkingv1beta1.NetworkingV1beta1Interface
	SystemV1beta1() systemv1beta1.SystemV1beta1Interface
}

// Clientset contains the clients for groups. Each group has exactly one
// version included in a Clientset.
type Clientset struct {
	*discovery.DiscoveryClient
	clusterinformationV1beta1 *clusterinformationv1beta1.ClusterinformationV1beta1Client
	networkingV1beta1         *networkingv1beta1.NetworkingV1beta1Client
	systemV1beta1             *systemv1beta1.SystemV1beta1Client
}

// ClusterinformationV1beta1 retrieves the ClusterinformationV1beta1Client
func (c *Clientset) ClusterinformationV1beta1() clusterinformationv1beta1.ClusterinformationV1beta1Interface {
	return c.clusterinformationV1beta1
}

// NetworkingV1beta1 retrieves the NetworkingV1beta1Client
func (c *Clientset) NetworkingV1beta1() networkingv1beta1.NetworkingV1beta1Interface {
	return c.networkingV1beta1
}

// SystemV1beta1 retrieves the SystemV1beta1Client
func (c *Clientset) SystemV1beta1() systemv1beta1.SystemV1beta1Interface {
	return c.systemV1beta1
}

// Discovery retrieves the DiscoveryClient
func (c *Clientset) Discovery() discovery.DiscoveryInterface {
	if c == nil {
		return nil
	}
	return c.DiscoveryClient
}

// NewForConfig creates a new Clientset for the given config.
func NewForConfig(c *rest.Config) (*Clientset, error) {
	configShallowCopy := *c
	if configShallowCopy.RateLimiter == nil && configShallowCopy.QPS > 0 {
		configShallowCopy.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(configShallowCopy.QPS, configShallowCopy.Burst)
	}
	var cs Clientset
	var err error
	cs.clusterinformationV1beta1, err = clusterinformationv1beta1.NewForConfig(&configShallowCopy)
	if err != nil {
		return nil, err
	}
	cs.networkingV1beta1, err = networkingv1beta1.NewForConfig(&configShallowCopy)
	if err != nil {
		return nil, err
	}
	cs.systemV1beta1, err = systemv1beta1.NewForConfig(&configShallowCopy)
	if err != nil {
		return nil, err
	}

	cs.DiscoveryClient, err = discovery.NewDiscoveryClientForConfig(&configShallowCopy)
	if err != nil {
		return nil, err
	}
	return &cs, nil
}

// NewForConfigOrDie creates a new Clientset for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *Clientset {
	var cs Clientset
	cs.clusterinformationV1beta1 = clusterinformationv1beta1.NewForConfigOrDie(c)
	cs.networkingV1beta1 = networkingv1beta1.NewForConfigOrDie(c)
	cs.systemV1beta1 = systemv1beta1.NewForConfigOrDie(c)

	cs.DiscoveryClient = discovery.NewDiscoveryClientForConfigOrDie(c)
	return &cs
}

// New creates a new Clientset for the given RESTClient.
func New(c rest.Interface) *Clientset {
	var cs Clientset
	cs.clusterinformationV1beta1 = clusterinformationv1beta1.New(c)
	cs.networkingV1beta1 = networkingv1beta1.New(c)
	cs.systemV1beta1 = systemv1beta1.New(c)

	cs.DiscoveryClient = discovery.NewDiscoveryClient(c)
	return &cs
}
