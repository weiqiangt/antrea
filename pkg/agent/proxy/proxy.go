package proxy

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/proxy"

	"github.com/vmware-tanzu/antrea/pkg/agent/openflow"
)

type Proxier struct {
	endpointsChanges *proxy.EndpointChangeTracker
	serviceChanges   *proxy.ServiceChangeTracker
	ofClient         openflow.Client
}

func (p *Proxier) OnEndpointsAdd(endpoints *corev1.Endpoints) {
	p.ofClient.InstallPodFlows()
	panic("implement me")
}

func (p *Proxier) OnEndpointsUpdate(oldEndpoints, endpoints *corev1.Endpoints) {
	panic("implement me")
}

func (p *Proxier) OnEndpointsDelete(endpoints *corev1.Endpoints) {
	panic("implement me")
}

func (p *Proxier) OnEndpointsSynced() {
	panic("implement me")
}

func (p *Proxier) OnServiceAdd(service *corev1.Service) {
	panic("implement me")
}

func (p *Proxier) OnServiceUpdate(oldService, service *corev1.Service) {
	panic("implement me")
}

func (p *Proxier) OnServiceDelete(service *corev1.Service) {
	panic("implement me")
}

func (p *Proxier) OnServiceSynced() {
	panic("implement me")
}

func NewProxier() *Proxier {
	return &Proxier{
		endpointsChanges: nil,
		serviceChanges:   nil,
	}
}
