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

package openflow

import (
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"time"

	"k8s.io/klog"

	"github.com/vmware-tanzu/antrea/pkg/agent/openflow/cookie"
	"github.com/vmware-tanzu/antrea/pkg/agent/types"
	binding "github.com/vmware-tanzu/antrea/pkg/ovs/openflow"
	"github.com/vmware-tanzu/antrea/pkg/ovs/ovsconfig"
)

const maxRetryForOFSwitch = 5

//go:generate mockgen -copyright_file ../../../hack/boilerplate/license_header.raw.txt -destination testing/mock_client.go -package=testing github.com/vmware-tanzu/antrea/pkg/agent/openflow Client

// Client is the interface to program OVS flows for entity connectivity of Antrea.
// TODO: flow sync (e.g. at agent restart), retry at failure, garbage collection mechanisms
type Client interface {
	// Initialize sets up all basic flows on the specific OVS bridge.
	Initialize(brClient ovsconfig.OVSBridgeClient) error

	// InstallGatewayFlows sets up flows related to an OVS gateway port, the gateway must exist.
	InstallGatewayFlows(gatewayAddr net.IP, gatewayMAC net.HardwareAddr, gatewayOFPort uint32) error

	// InstallClusterServiceCIDRFlows sets up the appropriate flows so that traffic can reach
	// the different Services running in the Cluster. This method needs to be invoked once with
	// the Cluster Service CIDR as a parameter.
	InstallClusterServiceCIDRFlows(serviceNet *net.IPNet, gatewayOFPort uint32) error

	// InstallTunnelFlows sets up flows related to an OVS tunnel port, the tunnel port must exist.
	InstallTunnelFlows(tunnelOFPort uint32) error

	// InstallNodeFlows should be invoked when a connection to a remote Node is going to be set
	// up. The hostname is used to identify the added flows. Calls to InstallNodeFlows are
	// idempotent. Concurrent calls to InstallNodeFlows and / or UninstallNodeFlows are
	// supported as long as they are all for different hostnames.
	InstallNodeFlows(hostname string, localGatewayMAC net.HardwareAddr, peerGatewayIP net.IP, peerPodCIDR net.IPNet, tunnelPeerAddr net.IP) error

	// UninstallNodeFlows removes the connection to the remote Node specified with the
	// hostname. UninstallNodeFlows will do nothing if no connection to the host was established.
	UninstallNodeFlows(hostname string) error

	// InstallPodFlows should be invoked when a connection to a Pod on current Node. The
	// containerID is used to identify the added flows. Calls to InstallPodFlows are
	// idempotent. Concurrent calls to InstallPodFlows and / or UninstallPodFlows are
	// supported as long as they are all for different containerIDs.
	InstallPodFlows(containerID string, podInterfaceIP net.IP, podInterfaceMAC, gatewayMAC net.HardwareAddr, ofPort uint32) error

	// UninstallPodFlows removes the connection to the local Pod specified with the
	// containerID. UninstallPodFlows will do nothing if no connection to the Pod was established.
	UninstallPodFlows(containerID string) error

	// GetFlowTableStatus should return an array of flow table status, all existing flow tables should be included in the list.
	GetFlowTableStatus() []binding.TableStatus

	// InstallPolicyRuleFlows installs flows for a new NetworkPolicy rule. Rule should include all fields in the
	// NetworkPolicy rule. Each ingress/egress policy rule installs Openflow entries on two tables, one for
	// ruleTable and the other for dropTable. If a packet does not pass the ruleTable, it will be dropped by the
	// dropTable.
	InstallPolicyRuleFlows(rule *types.PolicyRule) error

	// UninstallPolicyRuleFlows removes the Openflow entry relevant to the specified NetworkPolicy rule.
	// UninstallPolicyRuleFlows will do nothing if no Openflow entry for the rule is installed.
	UninstallPolicyRuleFlows(ruleID uint32) error

	// AddPolicyRuleAddress adds one or multiple addresses to the specified NetworkPolicy rule. If addrType is true, the
	// addresses are added to PolicyRule.From, else to PolicyRule.To.
	AddPolicyRuleAddress(ruleID uint32, addrType types.AddressType, addresses []types.Address) error

	// DeletePolicyRuleAddress removes addresses from the specified NetworkPolicy rule. If addrType is srcAddress, the addresses
	// are removed from PolicyRule.From, else from PolicyRule.To.
	DeletePolicyRuleAddress(ruleID uint32, addrType types.AddressType, addresses []types.Address) error

	// Disconnect disconnects the connection between client and OFSwitch.
	Disconnect() error
}

// GetFlowTableStatus returns an array of flow table status.
func (c *client) GetFlowTableStatus() []binding.TableStatus {
	return c.bridge.DumpTableStatus()
}

// addMissingFlows adds any flow from flows which is not currently in the flow cache. The function
// returns immediately in case of error when adding a flow. If a flow is added successfully, it is
// added to the flow cache. If the flow cache has not been initialized yet (i.e. there is no
// flowCacheKey key in the cache map), we create it first.
func (c *client) addMissingFlows(cache *flowCategoryCache, flowCacheKey string, flows []binding.Flow) error {
	// initialize flow cache if needed
	fCacheI, _ := cache.LoadOrStore(flowCacheKey, flowCache{})
	fCache := fCacheI.(flowCache)

	for _, flow := range flows {
		flowKey := flow.MatchString()
		if _, ok := fCache[flowKey]; ok {
			continue
		}
		if err := c.flowOperations.Add(flow); err != nil {
			return err
		}
		fCache[flow.MatchString()] = flow
	}
	return nil
}

// deleteFlows deletes all the flows in the flow cache indexed by the provided flowCacheKey.
func (c *client) deleteFlows(cache *flowCategoryCache, flowCacheKey string) error {
	fCacheI, ok := cache.Load(flowCacheKey)
	if !ok {
		// no matching flows found in the cache
		return nil
	}
	fCache := fCacheI.(flowCache)

	// delete flowCache from the top-level cache if all flows were successfully deleted
	defer func() {
		if len(fCache) == 0 {
			cache.Delete(flowCacheKey)
		}
	}()

	for flowKey, flow := range fCache {
		if err := c.flowOperations.Delete(flow); err != nil {
			return err
		}
		delete(fCache, flowKey)
	}
	return nil
}

func (c *client) InstallNodeFlows(hostname string, localGatewayMAC net.HardwareAddr, peerGatewayIP net.IP, peerPodCIDR net.IPNet, tunnelPeerAddr net.IP) error {
	flows := []binding.Flow{
		c.arpResponderFlow(peerGatewayIP, cookie.Node),
		c.l3FwdFlowToRemote(localGatewayMAC, peerPodCIDR, tunnelPeerAddr, cookie.Node),
	}

	return c.addMissingFlows(c.nodeFlowCache, hostname, flows)
}

func (c *client) UninstallNodeFlows(hostname string) error {
	return c.deleteFlows(c.nodeFlowCache, hostname)
}

func (c *client) InstallPodFlows(containerID string, podInterfaceIP net.IP, podInterfaceMAC, gatewayMAC net.HardwareAddr, ofPort uint32) error {
	flows := []binding.Flow{
		c.podClassifierFlow(ofPort, cookie.Pod),
		c.podIPSpoofGuardFlow(podInterfaceIP, podInterfaceMAC, ofPort, cookie.Pod),
		c.arpSpoofGuardFlow(podInterfaceIP, podInterfaceMAC, ofPort, cookie.Pod),
		c.l2ForwardCalcFlow(podInterfaceMAC, ofPort, cookie.Pod),
		c.l3FlowsToPod(gatewayMAC, podInterfaceIP, podInterfaceMAC, cookie.Pod),
	}

	return c.addMissingFlows(c.podFlowCache, containerID, flows)
}

func (c *client) UninstallPodFlows(containerID string) error {
	return c.deleteFlows(c.podFlowCache, containerID)
}

func (c *client) InstallClusterServiceCIDRFlows(serviceNet *net.IPNet, gatewayOFPort uint32) error {
	return c.flowOperations.Add(c.serviceCIDRDNATFlow(serviceNet, gatewayOFPort, cookie.Service))
}

func (c *client) InstallGatewayFlows(gatewayAddr net.IP, gatewayMAC net.HardwareAddr, gatewayOFPort uint32) error {
	if err := c.flowOperations.Add(c.gatewayClassifierFlow(gatewayOFPort, cookie.Default)); err != nil {
		return err
	} else if err := c.flowOperations.Add(c.gatewayIPSpoofGuardFlow(gatewayOFPort, cookie.Default)); err != nil {
		return err
	} else if err := c.flowOperations.Add(c.gatewayARPSpoofGuardFlow(gatewayOFPort, cookie.Default)); err != nil {
		return err
	} else if err := c.flowOperations.Add(c.l3ToGatewayFlow(gatewayAddr, gatewayMAC, cookie.Default)); err != nil {
		return err
	} else if err := c.flowOperations.Add(c.l2ForwardCalcFlow(gatewayMAC, gatewayOFPort, cookie.Default)); err != nil {
		return err
	} else if err := c.flowOperations.Add(c.localProbeFlow(gatewayAddr, cookie.Default)); err != nil {
		return err
	}
	return nil
}

func (c *client) InstallTunnelFlows(tunnelOFPort uint32) error {
	if err := c.flowOperations.Add(c.tunnelClassifierFlow(tunnelOFPort, cookie.Default)); err != nil {
		return err
	}
	if err := c.flowOperations.Add(c.l2ForwardCalcFlow(globalVirtualMAC, tunnelOFPort, 0)); err != nil {
		return err
	}
	return nil
}

func (c *client) Initialize(brClient ovsconfig.OVSBridgeClient) error {
	// Initiate connections to target OFswitch, and create tables on the switch.
	if err := c.bridge.Connect(maxRetryForOFSwitch, make(chan struct{})); err != nil {
		return err
	}

	c.cookieAllocator = cookie.NewAllocator(roundNum(brClient))
	for _, flow := range c.defaultFlows() {
		if err := c.flowOperations.Add(flow); err != nil {
			return fmt.Errorf("failed to install default flows: %v", err)
		}
	}
	if err := c.flowOperations.Add(c.arpNormalFlow(cookie.Default)); err != nil {
		return fmt.Errorf("failed to install arp normal flow: %v", err)
	}
	if err := c.flowOperations.Add(c.l2ForwardOutputFlow(cookie.Default)); err != nil {
		return fmt.Errorf("failed to install l2 forward output flows: %v", err)
	}
	for _, flow := range c.connectionTrackFlows(cookie.Default) {
		if err := c.flowOperations.Add(flow); err != nil {
			return fmt.Errorf("failed to install connection track flows: %v", err)
		}
	}
	for _, flow := range c.establishedConnectionFlows(cookie.Default) {
		if err := flow.Add(); err != nil {
			return fmt.Errorf("failed to install flows to skip established connections: %v", err)
		}
	}
	return nil
}

func roundNum(bridgeClient ovsconfig.OVSBridgeClient) uint64 {
	var num uint64
	rand.Seed(time.Now().UnixNano())
	// read the round number
	extIDs, ovsErr := bridgeClient.GetExternalIDs()
	if ovsErr != nil {
		klog.Warningf("Getting last round number failed: %s", ovsErr.Error())
		num = rand.Uint64()
	} else if roundNumValue, ok := extIDs[roundNumKey]; ok {
		var err error
		num, err = strconv.ParseUint(roundNumValue, 10, 64)
		if err != nil {
			klog.Warningf("Parsing last round number failed: %v", err)
			num = rand.Uint64()
		}
		if num == (1<<cookie.BitwidthRound)-1 {
			num = 0
		} else {
			num += 1
		}
	} else {
		klog.Infoln("No round number found in OVSDB, using a random value")
	}
	num = rand.Uint64() % (1 << cookie.BitwidthRound)
	klog.Infof("Using round number %d", num)
	// write back
	extIDs[roundNumKey] = fmt.Sprint(num)
	updatedExtIDs := make(map[string]interface{})
	for k, v := range extIDs {
		extIDs[k] = v
	}
	if err := bridgeClient.SetExternalIDs(updatedExtIDs); err != nil {
		klog.Errorf("Writing round number failed: %v", err)
	}

	return num
}
