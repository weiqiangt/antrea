package groupmember

import (
	"net"

	cpv1beta "github.com/vmware-tanzu/antrea/pkg/apis/controlplane/v1beta2"
)

type Response struct {
	Pod *cpv1beta.PodReference `json:"pod,omitempty"`
	// IP maintains the IPAddresses associated with the Pod.
	IP string `json:"ip,omitempty"`
	// Ports maintain the named port mapping of this Pod.
	Ports []cpv1beta.NamedPort `json:"ports,omitempty"`
}

func ObjectToResponse(member cpv1beta.GroupMember) Response {
	var ipStr string
	for i, ip := range member.IPs {
		if i != 0 {
			ipStr += ", "
		}
		ipStr += net.IP(ip).String()
	}
	return Response{Pod: member.Pod, IP: ipStr, Ports: member.Ports}
}
