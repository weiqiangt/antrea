package proxy

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/proxy/config"
	"k8s.io/kubernetes/pkg/util/node"

	"github.com/vmware-tanzu/antrea/pkg/agent/interfacestore"
	"github.com/vmware-tanzu/antrea/pkg/agent/openflow"
	"github.com/vmware-tanzu/antrea/pkg/monitor"
)

const (
	resyncPeriod = 24 * time.Hour // We don't want to resync actually.
	name         = "antrea-agent-proxy"
)

type instance struct {
	serviceConfig   *config.ServiceConfig
	endpointsConfig *config.EndpointsConfig
	proxier         *proxier
}

type Instance interface {
	Run(stopCh <-chan struct{})
}

func (i *instance) Run(stopCh <-chan struct{}) {
	go i.serviceConfig.Run(stopCh)
	go i.endpointsConfig.Run(stopCh)
	i.proxier.stopChan = stopCh
	klog.Infof("Starting %s", name)
	i.proxier.SyncLoop()
}

func New(querier monitor.AgentQuerier, ifstore interfacestore.InterfaceStore, ofClient openflow.Client, informerFactory informers.SharedInformerFactory) (Instance, error) {
	hostname, err := node.GetHostname("localhost")
	if err != nil {
		return nil, err
	}
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(runtime.NewScheme(), corev1.EventSource{Component: name, Host: hostname})
	p := newProxier(name, hostname, ifstore, recorder, ofClient)
	serviceConfig := config.NewServiceConfig(informerFactory.Core().V1().Services(), resyncPeriod)
	serviceConfig.RegisterEventHandler(p)

	endpointsConfig := config.NewEndpointsConfig(informerFactory.Core().V1().Endpoints(), resyncPeriod)
	endpointsConfig.RegisterEventHandler(p)

	return &instance{
		endpointsConfig: endpointsConfig,
		serviceConfig:   serviceConfig,
		proxier:         p,
	}, nil
}
