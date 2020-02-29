package proxy

import (
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	k8sproxy "k8s.io/kubernetes/pkg/proxy"
	"k8s.io/kubernetes/pkg/util/async"

	"github.com/vmware-tanzu/antrea/pkg/agent/interfacestore"
	"github.com/vmware-tanzu/antrea/pkg/agent/openflow"
	binding "github.com/vmware-tanzu/antrea/pkg/ovs/openflow"
)

// internal struct for string service information
type ServiceInfo struct {
	*k8sproxy.BaseServiceInfo
}

// returns a new k8sproxy.ServicePort which abstracts a serviceInfo
func newServiceInfo(port *corev1.ServicePort, service *corev1.Service, baseInfo *k8sproxy.BaseServiceInfo) k8sproxy.ServicePort {
	info := &ServiceInfo{BaseServiceInfo: baseInfo}
	return info
}

// internal struct for endpoints information
type EndpointInfo struct {
	*k8sproxy.BaseEndpointInfo
}

// returns a new k8sproxy.Endpoint which abstracts a endpointsInfo
func newEndpointInfo(baseInfo *k8sproxy.BaseEndpointInfo) k8sproxy.Endpoint {
	return &EndpointInfo{BaseEndpointInfo: baseInfo}
}

type proxier struct {
	endpointsChanges *k8sproxy.EndpointChangeTracker
	serviceChanges   *k8sproxy.ServiceChangeTracker
	informerFactory  informers.SharedInformerFactory
	ifstore          interfacestore.InterfaceStore

	syncProxyRulesMutex sync.Mutex
	serviceMap          k8sproxy.ServiceMap
	endpointsMap        k8sproxy.EndpointsMap
	serviceSynced       bool
	endpointsSynced     bool
	initialized         int32

	groupMap       map[k8sproxy.ServicePortName]uint32
	groupIDCounter uint32

	runner   *async.BoundedFrequencyRunner
	stopChan <-chan struct{}
	ofClient openflow.Client
}

func (p *proxier) setInitialized(value bool) {
	var initialized int32
	if value {
		initialized = 1
	}
	atomic.StoreInt32(&p.initialized, initialized)
}

func (p *proxier) isInitialized() bool {
	return atomic.LoadInt32(&p.initialized) > 0
}

func (p *proxier) getGroupID(svcPortName k8sproxy.ServicePortName) uint32 {
	if id, ok := p.groupMap[svcPortName]; ok {
		return id
	} else {
		// TODO: wrap the counter?
		id := atomic.AddUint32(&p.groupIDCounter, 1)
		p.groupMap[svcPortName] = id
		return id
	}
}

func (p *proxier) syncProxyRules() {
	p.syncProxyRulesMutex.Lock()
	defer p.syncProxyRulesMutex.Unlock()

	// don't sync rules till we've received services and endpoints
	if !p.endpointsSynced || !p.serviceSynced {
		klog.V(2).Info("Not syncing rules until Services and Endpoints have been received")
		return
	}

	klog.Infoln("Syncing proxy rules")
	start := time.Now()
	defer func() {
		klog.V(2).Infof("syncProxyRules took %v", time.Since(start))
	}()

	serviceUpdateResult := k8sproxy.UpdateServiceMap(p.serviceMap, p.serviceChanges)
	endpointUpdateResult := p.endpointsMap.Update(p.endpointsChanges)
	klog.Infof("Service updates: %v, Map: %v", serviceUpdateResult, p.serviceMap)
	klog.Infof("Endpoint updates: %v, Map: %v", endpointUpdateResult, p.endpointsMap)
	for namespacedName, port := range serviceUpdateResult.HCServiceNodePorts {
		klog.Infof("Updating %s:%d", namespacedName.String(), port)
	}


	for _, svcPortName := range endpointUpdateResult.StaleServiceNames {
		klog.Infof("Removing Service %v", svcPortName)
		groupID := p.getGroupID(svcPortName)
		p.ofClient.UninstallServiceFlows(groupID)
	}

	for svcPortName, svcPort := range p.serviceMap {
		svcInfo, ok := svcPort.(*ServiceInfo)
		if !ok {
			klog.Errorf("Failed to cast serviceInfo %q", svcPortName.String())
			continue
		}
		if endpoints, ok := p.endpointsMap[svcPortName]; ok {
			groupID := p.getGroupID(svcPortName)
			// TODO: handle zero endpoints problem.
			var affinityTimeoutSeconds uint16
			if svcInfo.SessionAffinityType == corev1.ServiceAffinityClientIP {
				affinityTimeoutSeconds = uint16(svcInfo.StickyMaxAgeSeconds)
			}
			protocol := binding.ProtocolIP
			if svcInfo.GetProtocol() == corev1.ProtocolTCP {
				protocol = binding.ProtocolTCP
			} else if svcInfo.GetProtocol() == corev1.ProtocolUDP {
				protocol = binding.ProtocolUDP
			}
			if err := p.ofClient.UpdateEndpointsGroup(groupID, protocol, affinityTimeoutSeconds != 0, endpoints...); err != nil {
				klog.Errorf("Error when adding endpoints: %v", err)
				return
			}
			if err := p.ofClient.InstallServiceFlows(groupID, svcInfo.ClusterIP, uint16(svcInfo.Port), protocol, affinityTimeoutSeconds); err != nil {
				klog.Errorf("Error when adding service flows: %v", err)
				break
			}
		}
	}

	klog.Infoln("Finish updating services")

}

func (p *proxier) Sync() {
	p.runner.Run()
}

func (p *proxier) SyncLoop() {
	p.runner.Loop(p.stopChan)
}

func (p *proxier) OnEndpointsAdd(endpoints *corev1.Endpoints) {
	p.OnEndpointsUpdate(nil, endpoints)
}

func (p *proxier) OnEndpointsUpdate(oldEndpoints, endpoints *corev1.Endpoints) {
	if p.endpointsChanges.Update(oldEndpoints, endpoints) && p.isInitialized() {
		p.runner.Run()
	}
}

func (p *proxier) OnEndpointsDelete(endpoints *corev1.Endpoints) {
	p.OnEndpointsUpdate(endpoints, nil)
}

func (p *proxier) OnEndpointsSynced() {
	func() {
		p.syncProxyRulesMutex.Lock()
		defer p.syncProxyRulesMutex.Unlock()
		p.endpointsSynced = true
		p.setInitialized(p.serviceSynced && p.endpointsSynced)
	}()
	p.runner.Run()
}

func (p *proxier) OnServiceAdd(service *corev1.Service) {
	p.OnServiceUpdate(nil, service)
}

func (p *proxier) OnServiceUpdate(oldService, service *corev1.Service) {
	if p.serviceChanges.Update(oldService, service) && p.isInitialized() {
		p.runner.Run()
	}
}

func (p *proxier) OnServiceDelete(service *corev1.Service) {
	p.OnServiceUpdate(service, nil)
}

func (p *proxier) OnServiceSynced() {
	func() {
		p.syncProxyRulesMutex.Lock()
		defer p.syncProxyRulesMutex.Unlock()
		p.serviceSynced = true
		p.setInitialized(p.serviceSynced && p.endpointsSynced)
	}()
	p.runner.Run()
}

func newProxier(
	name string,
	hostname string,
	ifstore interfacestore.InterfaceStore,
	recorder record.EventRecorder,
	ofClient openflow.Client,
) *proxier {
	p := new(proxier)
	// TODO: support IPV6 one day.
	enableIPV6 := false
	p.ofClient = ofClient
	p.ifstore = ifstore
	p.endpointsChanges = k8sproxy.NewEndpointChangeTracker(hostname, newEndpointInfo, &enableIPV6, recorder)
	p.serviceChanges = k8sproxy.NewServiceChangeTracker(newServiceInfo, &enableIPV6, recorder)
	p.runner = async.NewBoundedFrequencyRunner(name, p.syncProxyRules, time.Nanosecond, 24*time.Hour, 2)
	p.endpointsMap = make(k8sproxy.EndpointsMap)
	p.serviceMap = make(k8sproxy.ServiceMap)
	p.groupMap = map[k8sproxy.ServicePortName]uint32{}
	return p
}
