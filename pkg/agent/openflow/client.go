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
	"encoding/binary"
	"fmt"
	"net"

	"k8s.io/klog"
	k8sproxy "k8s.io/kubernetes/pkg/proxy"

	"github.com/vmware-tanzu/antrea/pkg/agent/config"
	"github.com/vmware-tanzu/antrea/pkg/agent/openflow/cookie"
	"github.com/vmware-tanzu/antrea/pkg/agent/types"
	binding "github.com/vmware-tanzu/antrea/pkg/ovs/openflow"
)

const maxRetryForOFSwitch = 5

// Client is the interface to program OVS flows for entity connectivity of Antrea.
type Client interface {
	// Initialize sets up all basic flows on the specific OVS bridge. It returns a channel which
	// is used to notify the caller in case of a reconnection, in which case ReplayFlows should
	// be called to ensure that the set of OVS flows is correct. All flows programmed in the
	// switch which match the current round number will be deleted before any new flow is
	// installed.
	Initialize(roundInfo types.RoundInfo, config *config.NodeConfig, encapMode config.TrafficEncapModeType) (<-chan struct{}, error)

	// InstallGatewayFlows sets up flows related to an OVS gateway port, the gateway must exist.
	InstallGatewayFlows(gatewayAddr net.IP, gatewayMAC net.HardwareAddr, gatewayOFPort uint32) error

	// InstallClusterServiceCIDRFlows sets up the appropriate flows so that traffic can reach
	// the different Services running in the Cluster. This method needs to be invoked once with
	// the Cluster Service CIDR as a parameter.
	InstallClusterServiceCIDRFlows() error

	// InstallDefaultTunnelFlows sets up the classification flow for the default (flow based) tunnel.
	InstallDefaultTunnelFlows(tunnelOFPort uint32) error

	// InstallNodeFlows should be invoked when a connection to a remote Node is going to be set
	// up. The hostname is used to identify the added flows. When IPSec tunnel is enabled,
	// ipsecTunOFPort must be set to the OFPort number of the IPSec tunnel port to the remote Node;
	// otherwise ipsecTunOFPort must be set to 0.
	// InstallNodeFlows has all-or-nothing semantics(call succeeds if all the flows are installed
	// successfully, otherwise no flows will be installed). Calls to InstallNodeFlows are idempotent.
	// Concurrent calls to InstallNodeFlows and / or UninstallNodeFlows are supported as long as they
	// are all for different hostnames.
	InstallNodeFlows(
		hostname string,
		localGatewayMAC net.HardwareAddr,
		peerPodCIDR net.IPNet,
		peerGatewayIP, tunnelPeerIP net.IP,
		tunOFPort, ipsecTunOFPort uint32) error

	// UninstallNodeFlows removes the connection to the remote Node specified with the
	// hostname. UninstallNodeFlows will do nothing if no connection to the host was established.
	UninstallNodeFlows(hostname string) error

	// InstallPodFlows should be invoked when a connection to a Pod on current Node. The
	// containerID is used to identify the added flows. InstallPodFlows has all-or-nothing
	// semantics(call succeeds if all the flows are installed successfully, otherwise no
	// flows will be installed). Calls to InstallPodFlows are idempotent. Concurrent calls
	// to InstallPodFlows and / or UninstallPodFlows are supported as long as they are all
	// for different containerIDs.
	InstallPodFlows(containerID string, podInterfaceIP net.IP, podInterfaceMAC, gatewayMAC net.HardwareAddr, ofPort uint32) error

	// UninstallPodFlows removes the connection to the local Pod specified with the
	// containerID. UninstallPodFlows will do nothing if no connection to the Pod was established.
	UninstallPodFlows(containerID string) error

	UpdateEndpointsGroup(groupID uint32, protocol binding.Protocol, withSessionAffinity bool, endpoints ...k8sproxy.Endpoint) error

	InstallServiceFlows(groupID uint32, svcIP net.IP, svcPort uint16, protocol binding.Protocol, affinityTimeout uint16) error
	UninstallServiceFlows(groupID uint32) error

	// GetFlowTableStatus should return an array of flow table status, all existing flow tables should be included in the list.
	GetFlowTableStatus() []binding.TableStatus

	// InstallPolicyRuleFlows installs flows for a new NetworkPolicy rule. Rule should include all fields in the
	// NetworkPolicy rule. Each ingress/egress policy rule installs Openflow entries on two tables, one for
	// ruleTable and the other for dropTable. If a packet does not pass the ruleTable, it will be dropped by the
	// dropTable.
	InstallPolicyRuleFlows(ruleID uint32, rule *types.PolicyRule) error

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

	// IsConnected returns the connection status between client and OFSwitch. The return value is true if the OFSwitch is connected.
	IsConnected() bool

	// ReplayFlows should be called when a spurious disconnection occurs. After we reconnect to
	// the OFSwitch, we need to replay all the flows cached by the client. ReplayFlows will try
	// to replay as many flows as possible, and will log an error when a flow cannot be
	// installed.
	ReplayFlows()

	// DeleteStaleFlows deletes all flows from the previous round which are no longer needed. It
	// should be called by the agent after all required flows have been installed / updated with
	// the new round number.
	DeleteStaleFlows() error
}

// GetFlowTableStatus returns an array of flow table status.
func (c *client) GetFlowTableStatus() []binding.TableStatus {
	return c.bridge.DumpTableStatus()
}

// IsConnected returns the connection status between client and OFSwitch.
func (c *client) IsConnected() bool {
	return c.bridge.IsConnected()
}

// addEntries installs the flows on the OVS bridge and then add them into the flow cache. If the flow cache exists,
// it will return immediately, otherwise it will use Bundle to add all flows, and then add them into the flow cache.
// If it fails to add the flows with Bundle, it will return the error and no flow cache is created.
func (c *client) addEntries(cache *entryCategoryCache, cacheKey string, entries []binding.Entry) error {
	_, ok := cache.Load(cacheKey)
	// If a entry cache entry already exists for the key, return immediately. Otherwise, add the entries to the switch
	// and populate the cache with them.
	if ok {
		klog.V(2).Infof("Flows with cache key %s are already installed", cacheKey)
		return nil
	}
	err := c.operations.AddAll(entries)
	if err != nil {
		return err
	}
	eCache := entryCache{}
	// Add the successfully installed entries into the entry cache.
	for _, entry := range entries {
		eCache[entry.KeyString()] = entry
	}
	cache.Store(cacheKey, eCache)
	return nil
}

// deleteEntries deletes all the flows in the flow cache indexed by the provided flowCacheKey.
func (c *client) deleteEntries(cache *entryCategoryCache, cacheKey string) error {
	eCacheI, ok := cache.Load(cacheKey)
	if !ok {
		// no matching flows found in the cache
		return nil
	}
	eCache := eCacheI.(entryCache)
	// Delete flows from OVS.
	var delEntries []binding.Entry
	for _, entry := range eCache {
		delEntries = append(delEntries, entry)
	}
	if err := c.operations.DeleteAll(delEntries); err != nil {
		return err
	}
	cache.Delete(cacheKey)
	return nil
}

func (c *client) InstallNodeFlows(hostname string,
	localGatewayMAC net.HardwareAddr,
	peerPodCIDR net.IPNet,
	peerGatewayIP, tunnelPeerIP net.IP,
	tunOFPort, ipsecTunOFPort uint32) error {
	c.replayMutex.RLock()
	defer c.replayMutex.RUnlock()

	flows := []binding.Entry{
		c.arpResponderFlow(peerGatewayIP, cookie.Node),
	}
	if c.encapMode.NeedsEncapToPeer(tunnelPeerIP, c.nodeConfig.NodeIPAddr) {
		flows = append(flows, c.l3FwdFlowToRemote(localGatewayMAC, peerPodCIDR, tunnelPeerIP, tunOFPort, cookie.Node))
	} else {
		flows = append(flows, c.l3FwdFlowToRemoteViaGw(localGatewayMAC, peerPodCIDR, cookie.Node))
	}
	if ipsecTunOFPort != 0 {
		// When IPSec tunnel is enabled, packets received from the remote Node are
		// input from the Node's IPSec tunnel port, not the default tunnel port. So,
		// add a separate tunnelClassifierFlow for the IPSec tunnel port.
		flows = append(flows, c.tunnelClassifierFlow(ipsecTunOFPort, cookie.Node))
	}

	return c.addEntries(c.nodeFlowCache, hostname, flows)
}

func (c *client) UninstallNodeFlows(hostname string) error {
	c.replayMutex.RLock()
	defer c.replayMutex.RUnlock()
	return c.deleteEntries(c.nodeFlowCache, hostname)
}

func (c *client) InstallPodFlows(containerID string, podInterfaceIP net.IP, podInterfaceMAC, gatewayMAC net.HardwareAddr, ofPort uint32) error {
	c.replayMutex.RLock()
	defer c.replayMutex.RUnlock()
	flows := []binding.Entry{
		c.podClassifierFlow(ofPort, cookie.Pod),
		c.podIPSpoofGuardFlow(podInterfaceIP, podInterfaceMAC, ofPort, cookie.Pod),
		c.arpSpoofGuardFlow(podInterfaceIP, podInterfaceMAC, ofPort, cookie.Pod),
		c.l2ForwardCalcFlow(podInterfaceMAC, ofPort, cookie.Pod),
	}

	// NoEncap mode has no tunnel.
	if c.encapMode.SupportsEncap() {
		flows = append(flows, c.l3FlowsToPod(gatewayMAC, podInterfaceIP, podInterfaceMAC, cookie.Pod))
		flow := c.pipeline[serviceForwardingTable].BuildFlow(priorityNormal).MatchProtocol(binding.ProtocolIP).
			MatchCTMark(0xfff1).
			MatchDstIP(podInterfaceIP).
			Action().SetDstMAC(podInterfaceMAC).
			Action().DecTTL().
			Action().ResubmitToTable(l2ForwardingCalcTable).
			Cookie(c.cookieAllocator.Request(cookie.Pod).Raw()).
			Done()
		flows = append(flows, flow)
	}
	return c.addEntries(c.podFlowCache, containerID, flows)
}

func (c *client) UninstallPodFlows(containerID string) error {
	c.replayMutex.RLock()
	defer c.replayMutex.RUnlock()
	return c.deleteEntries(c.podFlowCache, containerID)
}

func (c *client) InstallServiceFlows(groupID uint32, svcIP net.IP, svcPort uint16, protocol binding.Protocol, affinityTimeout uint16) error {
	var entries []binding.Entry
	entries = append(entries, c.serviceLBFlow(groupID, svcIP, svcPort, protocol))
	if affinityTimeout != 0 {
		entries = append(entries, c.serviceLearnFlow(svcIP, svcPort, protocol, affinityTimeout))
	}
	flowKey := fmt.Sprintf("%s:%d:%s", svcIP, svcPort, protocol)
	return c.addEntries(c.serviceFlowCache, flowKey, entries)
}

func (c *client) UpdateEndpointsGroup(groupID uint32, protocol binding.Protocol, withSessionAffinity bool, endpoints ...k8sproxy.Endpoint) error {
	var entries []binding.Entry
	groupInterface, _ := c.groupCache.LoadOrStore(groupID, c.bridge.BuildGroup(groupID))
	group := groupInterface.(binding.Group)
	group.ResetBucket()
	var resubmitTableID binding.TableIDType
	var lbResultMark uint32
	if withSessionAffinity {
		resubmitTableID = serviceLBTable
		lbResultMark = serviceNeedLearnMark
	} else {
		resubmitTableID = endpointNATTable
		lbResultMark = serviceNoLearnMark
	}

	for _, endpoint := range endpoints {
		endpointPort, _ := endpoint.Port()
		endpointIP := net.ParseIP(endpoint.IP()).To4()
		ipVal := binary.BigEndian.Uint32(endpointIP)
		portVal := uint16(endpointPort)

		// TODO: add group to entries.
		group = group.Bucket().Weight(100).
			LoadReg(int(serviceIPReg), ipVal).
			LoadReg(int(servicePortReg), uint32(portVal)).
			LoadReg(int(serviceLearnReg), lbResultMark).
			ResubmitToTable(resubmitTableID).
			Done()

		entries = append(entries, c.endpointNATFlow(endpointIP, portVal, protocol))
	}
	if err := group.Add(); err != nil {
		return fmt.Errorf("error when installing endpoint: %w", err)
	}
	cacheKey := fmt.Sprintf("Endpoints:%d", groupID)
	return c.addEntries(c.serviceFlowCache, cacheKey, entries)
}

func (c *client) UninstallServiceFlows(groupID uint32) error {
	if obj, _ := c.groupCache.Load(groupID); obj != nil {
		if err := obj.(binding.Group).Delete(); err != nil {
			return err
		}
		cacheKey := fmt.Sprintf("Endpoints:%d", groupID)
		if err := c.deleteEntries(c.serviceFlowCache, cacheKey); err != nil {
			return err
		}
	}
	return nil
}

func (c *client) InstallClusterServiceCIDRFlows() error {
	flows := c.serviceCIDRDNATFlow()
	if err := c.operations.AddAll(flows); err != nil {
		return err
	}
	c.clusterServiceCIDRFlows = flows
	return nil
}

func (c *client) InstallGatewayFlows(gatewayAddr net.IP, gatewayMAC net.HardwareAddr, gatewayOFPort uint32) error {
	flows := []binding.Entry{
		c.gatewayClassifierFlow(gatewayOFPort, cookie.Default),
		c.gatewayIPSpoofGuardFlow(gatewayOFPort, cookie.Default),
		c.gatewayARPSpoofGuardFlow(gatewayOFPort, gatewayAddr, gatewayMAC, cookie.Default),
		c.ctRewriteDstMACFlow(gatewayMAC, cookie.Default),
		c.l2ForwardCalcFlow(gatewayMAC, gatewayOFPort, cookie.Default),
		c.localProbeFlow(gatewayAddr, cookie.Default),
	}
	// In NoEncap , no traffic from tunnel port
	if c.encapMode.SupportsEncap() {
		flows = append(flows, c.l3ToGatewayFlow(gatewayAddr, gatewayMAC, cookie.Default))
	}

	if err := c.operations.AddAll(flows); err != nil {
		return err
	}
	c.gatewayFlows = flows
	return nil
}

func (c *client) InstallDefaultTunnelFlows(tunnelOFPort uint32) error {
	flow := c.tunnelClassifierFlow(tunnelOFPort, cookie.Default)
	if err := c.operations.Add(flow); err != nil {
		return err
	}
	c.defaultTunnelFlows = []binding.Entry{flow}
	return nil
}

func (c *client) initialize() error {
	var entries []binding.Entry
	for _, flow := range c.defaultFlows() {
		entries = append(entries, flow)
	}
	if err := c.operations.AddAll(entries); err != nil {
		return fmt.Errorf("failed to install default flows: %v", err)
	}
	if err := c.operations.Add(c.arpNormalFlow(cookie.Default)); err != nil {
		return fmt.Errorf("failed to install arp normal flow: %v", err)
	}
	if err := c.operations.Add(c.l2ForwardOutputFlow(cookie.Default)); err != nil {
		return fmt.Errorf("failed to install L2 forward output flows: %v", err)
	}
	entries = []binding.Entry{}
	for _, flow := range c.connectionTrackFlows(cookie.Default) {
		entries = append(entries, flow)
	}
	if err := c.operations.AddAll(entries); err != nil {
		return fmt.Errorf("failed to install connection track flows: %v", err)
	}
	entries = []binding.Entry{}
	for _, flow := range c.establishedConnectionFlows(cookie.Default) {
		entries = append(entries, flow)
	}
	if err := c.operations.AddAll(entries); err != nil {
		return fmt.Errorf("failed to install flows to skip established connections: %v", err)
	}

	if c.encapMode.SupportsNoEncap() {
		if err := c.operations.Add(c.l2ForwardOutputInPortFlow(config.HostGatewayOFPort, cookie.Default)); err != nil {
			return fmt.Errorf("failed to install L2 forward same in-port and out-port flow: %v", err)
		}
	}
	return nil
}

func (c *client) Initialize(roundInfo types.RoundInfo, config *config.NodeConfig, encapMode config.TrafficEncapModeType) (<-chan struct{}, error) {
	c.nodeConfig = config
	c.encapMode = encapMode
	// Initiate connections to target OFswitch, and create tables on the switch.
	connCh := make(chan struct{})
	if err := c.bridge.Connect(maxRetryForOFSwitch, connCh); err != nil {
		return nil, err
	}

	// Ignore first notification, it is not a "reconnection".
	<-connCh

	c.roundInfo = roundInfo
	c.cookieAllocator = cookie.NewAllocator(roundInfo.RoundNum)

	// In the normal case, there should be no existing flows with the current round number. This
	// is needed in case the agent was restarted before we had a chance to increment the round
	// number (incrementing the round number happens once we are satisfied that stale flows from
	// the previous round have been deleted).
	if err := c.deleteFlowsByRoundNum(roundInfo.RoundNum); err != nil {
		return nil, fmt.Errorf("error when deleting exiting flows for current round number: %v", err)
	}

	return connCh, c.initialize()
}

func (c *client) ReplayFlows() {
	c.replayMutex.Lock()
	defer c.replayMutex.Unlock()

	if err := c.initialize(); err != nil {
		klog.Errorf("Error during flow replay: %v", err)
	}

	addFixedFlows := func(entries []binding.Entry) {
		for _, flow := range entries {
			flow.Reset()
		}
		if err := c.operations.AddAll(entries); err != nil {
			klog.Errorf("Error when replaying fixed flows: %v", err)
		}

	}

	addFixedFlows(c.gatewayFlows)
	addFixedFlows(c.clusterServiceCIDRFlows)
	addFixedFlows(c.defaultTunnelFlows)

	installCachedFlows := func(key, value interface{}) bool {
		fCache := value.(entryCache)
		cachedFlows := make([]binding.Entry, 0)

		for _, flow := range fCache {
			flow.Reset()
			cachedFlows = append(cachedFlows, flow)
		}

		if err := c.operations.AddAll(cachedFlows); err != nil {
			klog.Errorf("Error when replaying cached flows: %v", err)
		}
		return true
	}

	c.nodeFlowCache.Range(installCachedFlows)
	c.podFlowCache.Range(installCachedFlows)
	c.serviceFlowCache.Range(installCachedFlows)

	c.replayPolicyFlows()
}

func (c *client) deleteFlowsByRoundNum(roundNum uint64) error {
	cookieID, cookieMask := cookie.CookieMaskForRound(roundNum)
	return c.bridge.DeleteFlowsByCookie(cookieID, cookieMask)
}

func (c *client) DeleteStaleFlows() error {
	if c.roundInfo.PrevRoundNum == nil {
		klog.V(2).Info("Previous round number is unset, no flows to delete")
		return nil
	}
	return c.deleteFlowsByRoundNum(*c.roundInfo.PrevRoundNum)
}
