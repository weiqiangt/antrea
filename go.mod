module github.com/vmware-tanzu/antrea

go 1.12

require (
	github.com/TomCodeLV/OVSDB-golang-lib v0.0.0-20190103132138-cf96a9e61bd1
	github.com/containernetworking/cni v0.7.1
	github.com/containernetworking/plugins v0.8.2-0.20190724153215-ded2f1757770
	github.com/coreos/go-iptables v0.4.1
	github.com/elazarl/goproxy v0.0.0-20190911111923-ecfe977594f1 // indirect
	github.com/evanphx/json-patch v4.5.0+incompatible // indirect
	github.com/gogo/protobuf v1.2.2-0.20190723190241-65acae22fc9d
	github.com/golang/mock v1.3.1
	github.com/golang/protobuf v1.3.2
	github.com/google/uuid v1.1.1
	github.com/googleapis/gnostic v0.3.1 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/j-keck/arping v1.0.0
	github.com/kevinburke/ssh_config v0.0.0-20190725054713-01f96b0aa0cd
	github.com/satori/go.uuid v1.2.0
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.4.0
	github.com/vishvananda/netlink v1.0.0
	github.com/vmware/octant v0.8.0
	golang.org/x/crypto v0.0.0-20190820162420-60c769a6c586
	golang.org/x/sys v0.0.0-20190826190057-c7b8b68b1456
	google.golang.org/grpc v1.23.1
	gopkg.in/yaml.v2 v2.2.4
	k8s.io/api v0.0.0-20191121015604-11707872ac1c
	k8s.io/apimachinery v0.0.0-20191121015412-41065c7a8c2a
	k8s.io/apiserver v0.0.0-20191121020624-6eed2f5a3289
	k8s.io/client-go v0.0.0-20191121015835-571c0ef67034
	k8s.io/component-base v0.0.0-20191121020327-771114ba3383
	k8s.io/klog v1.0.0
)

// Octant is renamed from vmware/octant to vmware-tanzu/octant since v0.9.0.
// However, Octant v0.9.0 K8s API is not compatible with Antrea K8s API version.
// Furthermore, octant v0.8 and v0.9 do not check-in some generated code required for testing
// (mocks), which breaks "go mod". This has been fixed in master.
// Will remove this and upgrade Octant version after finding another compatible Octant release.
replace github.com/vmware/octant => github.com/antoninbas/octant v0.8.1-0.20191116223915-811df1acc59f
