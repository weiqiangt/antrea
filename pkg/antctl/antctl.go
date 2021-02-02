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

package antctl

import (
	"fmt"
	"reflect"

	"github.com/vmware-tanzu/antrea/pkg/agent/apiserver/handlers/agentinfo"
	"github.com/vmware-tanzu/antrea/pkg/agent/apiserver/handlers/ovsflows"
	"github.com/vmware-tanzu/antrea/pkg/agent/apiserver/handlers/ovstracing"
	"github.com/vmware-tanzu/antrea/pkg/agent/apiserver/handlers/podinterface"
	"github.com/vmware-tanzu/antrea/pkg/agent/openflow"
	"github.com/vmware-tanzu/antrea/pkg/antctl/framework"
	"github.com/vmware-tanzu/antrea/pkg/antctl/raw/proxy"
	"github.com/vmware-tanzu/antrea/pkg/antctl/raw/supportbundle"
	"github.com/vmware-tanzu/antrea/pkg/antctl/raw/traceflow"
	"github.com/vmware-tanzu/antrea/pkg/antctl/transform/addressgroup"
	"github.com/vmware-tanzu/antrea/pkg/antctl/transform/appliedtogroup"
	"github.com/vmware-tanzu/antrea/pkg/antctl/transform/controllerinfo"
	"github.com/vmware-tanzu/antrea/pkg/antctl/transform/formatter"
	"github.com/vmware-tanzu/antrea/pkg/antctl/transform/networkpolicy"
	"github.com/vmware-tanzu/antrea/pkg/antctl/transform/version"
	cpv1beta "github.com/vmware-tanzu/antrea/pkg/apis/controlplane/v1beta2"
	systemv1beta1 "github.com/vmware-tanzu/antrea/pkg/apis/system/v1beta1"
	controllerinforest "github.com/vmware-tanzu/antrea/pkg/apiserver/registry/system/controllerinfo"
	"github.com/vmware-tanzu/antrea/pkg/client/clientset/versioned/scheme"
	controllernetworkpolicy "github.com/vmware-tanzu/antrea/pkg/controller/networkpolicy"
)

var defaultFormatters = map[framework.FormatterType]framework.FormatterFunc{
	framework.JsonFormatter:  formatter.JSON,
	framework.YamlFormatter:  formatter.YAML,
	framework.TableFormatter: formatter.TableForGet,
}

// CommandList defines all commands that could be used in the antctl for both agents
// and controller. The unit test "TestCommandListValidation" ensures it to be valid.
var CommandList = &framework.CommandList{
	Definitions: []framework.CommandDefinition{
		{
			Use:          "version",
			Short:        "Print version information",
			Long:         "Print version information of antctl and ${component}",
			CommandGroup: framework.CommandGroupFlat,
			ControllerEndpoint: &framework.CommandEndpoint{
				ResourceEndpoint: &framework.ResourceEndpoint{
					ResourceName:         controllerinforest.ControllerInfoResourceName,
					GroupVersionResource: &systemv1beta1.ControllerInfoVersionResource,
				},
				Formatters:     defaultFormatters,
				AddonTransform: version.ControllerTransform,
			},
			AgentEndpoint: &framework.CommandEndpoint{
				NonResourceEndpoint: &framework.NonResourceCommandEndpoint{
					Path: "/version",
				},
				Formatters:     defaultFormatters,
				AddonTransform: version.AgentTransform,
			},

			TransformedResponse: reflect.TypeOf(version.Response{}),
		},
		{
			Use:   "log-level",
			Short: "Show or set log verbosity level",
			Long:  "Show or set the log verbosity level of ${component}",
			Example: `  Show the current log verbosity level
  $ antctl log-level
  Set the log verbosity level to 2
  $ antctl log-level 2`,
			CommandGroup: framework.CommandGroupFlat,
			ControllerEndpoint: &framework.CommandEndpoint{
				NonResourceEndpoint: &framework.NonResourceCommandEndpoint{
					Path: "/loglevel",
					Flags: []framework.FlagInfo{
						{
							Name:  "level",
							Usage: "The integer log verbosity level to set",
							Arg:   true,
						},
					},
					OutputType: framework.OutputTypeSingle,
				},
				Formatters: defaultFormatters,
			},
			AgentEndpoint: &framework.CommandEndpoint{
				NonResourceEndpoint: &framework.NonResourceCommandEndpoint{
					Path: "/loglevel",
					Flags: []framework.FlagInfo{
						{
							Name:  "level",
							Usage: "The integer log verbosity level to set",
							Arg:   true,
						},
					},
					OutputType: framework.OutputTypeSingle,
				},
				Formatters: defaultFormatters,
			},
			TransformedResponse: reflect.TypeOf(0),
		},
		{
			Use:     "networkpolicy",
			Aliases: []string{"networkpolicies", "netpol"},
			Short:   "Print control plane NetworkPolicies",
			Long:    "Print control plane NetworkPolicies in ${component}. 'namespace' is required if 'pod' is provided.",
			Example: `  Get a specific control plane NetworkPolicy
  $ antctl get networkpolicy 6001549b-ba63-4752-8267-30f52b4332db
  Get the list of all control plane NetworkPolicies
  $ antctl get networkpolicy
  Get the list of all control plane NetworkPolicies, sorted by the order in which the policies are evaluated.
  $ antctl get networkpolicy --sort-by=effectivePriority
  Get the control plane NetworkPolicy with a specific source (supported by agent only)
  $ antctl get networkpolicy -S allow-http -n ns1
  Get the list of control plane NetworkPolicies whose source NetworkPolicies are in a Namespace (supported by agent only)
  $ antctl get networkpolicy -n ns1
  Get the list of control plane NetworkPolicies with a specific source Type (supported by agent only)
  $ antctl get networkpolicy -T acnp
  Get the list of control plane NetworkPolicies applied to a Pod (supported by agent only)
  $ antctl get networkpolicy -p pod1 -n ns1`,
			CommandGroup: framework.CommandGroupGet,
			ControllerEndpoint: &framework.CommandEndpoint{
				ResourceEndpoint: &framework.ResourceEndpoint{
					GroupVersionResource: &cpv1beta.NetworkPolicyVersionResource,
					Flags: []framework.FlagInfo{
						{
							Name:            "sort-by",
							DefaultValue:    "",
							SupportedValues: []string{"effectivePriority"},
							Usage:           "Get NetworkPolicies in specific order. Current supported value is effectivePriority.",
						},
					},
				},
				Formatters:         defaultFormatters,
				PreferredFormatter: framework.TableFormatter,
				AddonTransform:     networkpolicy.Transform,
			},
			AgentEndpoint: &framework.CommandEndpoint{
				NonResourceEndpoint: &framework.NonResourceCommandEndpoint{
					Path: "/networkpolicies",
					Flags: []framework.FlagInfo{
						{
							Name:  "name",
							Usage: "Get NetworkPolicy by name.",
							Arg:   true,
						},
						{
							Name:      "source",
							Usage:     "Get NetworkPolicies for which the source has the provided name. The source of a control plane NetworkPolicy is the original policy resource (K8s NetworkPolicy or Antrea-native Policy) from which the control plane NetworkPolicy was derived.",
							Shorthand: "S",
						},
						{
							Name:      "namespace",
							Usage:     "Get Networkpolicies from specific Namespace.",
							Shorthand: "n",
						},
						{
							Name:      "pod",
							Usage:     "Get NetworkPolicies applied to the Pod. If present, Namespace must be provided.",
							Shorthand: "p",
						},
						{
							Name:      "type",
							Usage:     "Get NetworkPolicies with specific type. Type means the type of its source network policy: K8sNP, ACNP, ANP",
							Shorthand: "T",
						},
						{
							Name:            "sort-by",
							DefaultValue:    "",
							SupportedValues: []string{"effectivePriority"},
							Usage:           "Get NetworkPolicies in specific order. Current supported value is effectivePriority.",
						},
					},
					OutputType: framework.OutputTypeMultiple,
				},
				Formatters:         defaultFormatters,
				PreferredFormatter: framework.TableFormatter,
				AddonTransform:     networkpolicy.Transform,
			},
			TransformedResponse: reflect.TypeOf(networkpolicy.Response{}),
		},
		{
			Use:          "appliedtogroup",
			Aliases:      []string{"appliedtogroups", "atg"},
			Short:        "Print appliedto groups",
			Long:         "Print appliedto groups in ${component}",
			CommandGroup: framework.CommandGroupGet,
			ControllerEndpoint: &framework.CommandEndpoint{
				ResourceEndpoint: &framework.ResourceEndpoint{
					GroupVersionResource: &cpv1beta.AppliedToGroupVersionResource,
				},
				Formatters:         defaultFormatters,
				PreferredFormatter: framework.TableFormatter,
				AddonTransform:     appliedtogroup.Transform,
			},
			AgentEndpoint: &framework.CommandEndpoint{
				NonResourceEndpoint: &framework.NonResourceCommandEndpoint{
					Path: "/appliedtogroups",
					Flags: []framework.FlagInfo{
						{
							Usage: "Retrieve resource by name",
							Name:  "name",
							Arg:   true,
						},
					},
				},
				Formatters:         defaultFormatters,
				PreferredFormatter: framework.TableFormatter,
				AddonTransform:     appliedtogroup.Transform,
			},
			TransformedResponse: reflect.TypeOf(appliedtogroup.Response{}),
		},
		{
			Use:          "addressgroup",
			Aliases:      []string{"addressgroups", "ag"},
			Short:        "Print address groups",
			Long:         "Print address groups in ${component}",
			CommandGroup: framework.CommandGroupGet,
			ControllerEndpoint: &framework.CommandEndpoint{
				ResourceEndpoint: &framework.ResourceEndpoint{
					GroupVersionResource: &cpv1beta.AddressGroupVersionResource,
				},
				Formatters:         defaultFormatters,
				PreferredFormatter: framework.TableFormatter,
				AddonTransform:     addressgroup.Transform,
			},
			AgentEndpoint: &framework.CommandEndpoint{
				NonResourceEndpoint: &framework.NonResourceCommandEndpoint{
					Path: "/addressgroups",
					Flags: []framework.FlagInfo{
						{
							Usage: "Retrieve resource by name",
							Name:  "name",
							Arg:   true,
						},
					},
				},
				Formatters:         defaultFormatters,
				PreferredFormatter: framework.TableFormatter,
				AddonTransform:     addressgroup.Transform,
			},
			TransformedResponse: reflect.TypeOf(addressgroup.Response{}),
		},
		{
			Use:     "controllerinfo",
			Aliases: []string{"controllerinfos", "ci"},
			Short:   "Print Antrea controller's basic information",
			Long:    "Print Antrea controller's basic information including version, deployment, NetworkPolicy controller, ControllerConditions, etc.",
			ControllerEndpoint: &framework.CommandEndpoint{
				ResourceEndpoint: &framework.ResourceEndpoint{
					ResourceName:         controllerinforest.ControllerInfoResourceName,
					GroupVersionResource: &systemv1beta1.ControllerInfoVersionResource,
				},
				Formatters:         defaultFormatters,
				PreferredFormatter: framework.TableFormatter,
				AddonTransform:     controllerinfo.Transform,
			},
			CommandGroup:        framework.CommandGroupGet,
			TransformedResponse: reflect.TypeOf(controllerinfo.Response{}),
		},
		{
			Use:     "agentinfo",
			Aliases: []string{"agentinfos", "ai"},
			Short:   "Print agent's basic information",
			Long:    "Print agent's basic information including version, deployment, Node subnet, OVS info, AgentConditions, etc.",
			AgentEndpoint: &framework.CommandEndpoint{
				NonResourceEndpoint: &framework.NonResourceCommandEndpoint{
					Path:       "/agentinfo",
					OutputType: framework.OutputTypeSingle,
				},
				PreferredFormatter: framework.TableFormatter,
				Formatters:         defaultFormatters,
			},
			CommandGroup:        framework.CommandGroupGet,
			TransformedResponse: reflect.TypeOf(agentinfo.AntreaAgentInfoResponse{}),
		},
		{
			Use:     "podinterface",
			Aliases: []string{"podinterfaces", "pi"},
			Short:   "Print Pod's network interface information",
			Long:    "Print information about the network interface(s) created by the Antrea agent for the specified Pod.",
			Example: `  Get a pod-interface
  $ antctl get podinterface pod1 -n ns1
  Get the list of podinterfaces in a Namespace
  $ antctl get podinterface -n ns1
  Get the list of podinterfaces whose names match in all Namespaces
  $ antctl get podinterface pod1
  Get the list of podinterfaces in all Namespaces
  $ antctl get podinterface`,
			AgentEndpoint: &framework.CommandEndpoint{
				NonResourceEndpoint: &framework.NonResourceCommandEndpoint{
					Path: "/podinterfaces",
					Flags: []framework.FlagInfo{
						{
							Name:  "name",
							Usage: "Retrieve Pod interface by name. If present, Namespace must be provided.",
							Arg:   true,
						},
						{
							Name:      "namespace",
							Usage:     "Get Pod interfaces from specific Namespace",
							Shorthand: "n",
						},
					},
					OutputType: framework.OutputTypeMultiple,
				},
				Formatters: defaultFormatters,
			},
			CommandGroup:        framework.CommandGroupGet,
			TransformedResponse: reflect.TypeOf(podinterface.Response{}),
		},
		{
			Use:     "ovsflows",
			Aliases: []string{"of"},
			Short:   "Dump OVS flows",
			Long:    "Dump all the OVS flows or the flows installed for the specified entity.",
			Example: `  Dump all OVS flows
  $ antctl get ovsflows
  Dump OVS flows of a local Pod
  $ antctl get ovsflows -p pod1 -n ns1
  Dump OVS flows of a NetworkPolicy
  $ antctl get ovsflows --networkpolicy np1 -n ns1
  Dump OVS flows of a flow Table
  $ antctl get ovsflows -T IngressRule

  Antrea OVS Flow Tables:` + generateFlowTableHelpMsg(),
			AgentEndpoint: &framework.CommandEndpoint{
				NonResourceEndpoint: &framework.NonResourceCommandEndpoint{
					Path: "/ovsflows",
					Flags: []framework.FlagInfo{
						{
							Name:      "namespace",
							Usage:     "Namespace of the entity",
							Shorthand: "n",
						},
						{
							Name:      "pod",
							Usage:     "Name of a local Pod. If present, Namespace must be provided.",
							Shorthand: "p",
						},
						{
							Name:  "networkpolicy",
							Usage: "NetworkPolicy name. If present, Namespace must be provided.",
						},
						{
							Name:      "table",
							Usage:     "Comma separated Antrea OVS flow table names or numbers",
							Shorthand: "T",
						},
					},
					OutputType: framework.OutputTypeMultiple,
				},
				PreferredFormatter: framework.TableFormatter,
				Formatters:         defaultFormatters,
			},
			CommandGroup:        framework.CommandGroupGet,
			TransformedResponse: reflect.TypeOf(ovsflows.Response{}),
		},
		{
			Use:   "trace-packet",
			Short: "OVS packet tracing",
			Long:  "Trace the OVS flows the specified packet traverses, leveraging OVS 'ofproto/trace'. Check ovs-vswitchd(8) manpage for more information about 'ofproto/trace'.",
			Example: `  Trace an IP packet between two Pods
  $ antctl trace-packet -S ns1/pod1 -D ns2/pod2
  Trace a TCP packet from a local Pod to a Service
  $ antctl trace-packet -S ns1/pod1 -D ns2/srv2 -f tcp,tcp_dst=80
  Trace a UDP packet from a Pod to an IP address
  $ antctl trace-packet -S ns1/pod1 -D 10.1.2.3 -f udp,udp_dst=1234
  Trace an IP packet from a Pod to gateway port
  $ antctl trace-packet -S ns1/pod1 -D antrea-gw0
  Trace a UDP packet from an IP to a Pod
  $ antctl trace-packet -D ns1/pod1 -S 10.1.2.3 -f udp,udp_src=1234
  Trace an IP packet from OVS port using a specified source IP
  $ antctl trace-packet -p port1 -S 10.1.2.3 -D ns1/pod1
  Trace an ARP packet from a local Pod
  $ antctl trace-packet -p ns1/pod1 -f arp`,
			AgentEndpoint: &framework.CommandEndpoint{
				NonResourceEndpoint: &framework.NonResourceCommandEndpoint{
					Path: "/ovstracing",
					Flags: []framework.FlagInfo{
						{
							Name:      "port",
							Usage:     "OVS port to input the tracing packet. Can be an OVS port name, or a local Pod (specified by <Namespace>/<name>). If not specified, the input port will be automatically figured out based on the 'source', and the gateway port will be used if `source` is not specified either. If specified, the 'in_port' field should not be added in the 'flow' argument.",
							Shorthand: "p",
						},
						{
							Name:      "source",
							Usage:     "Source of the packet. Can be an OVS port name, or a (local or remote) Pod (specified by <Namespace>/<name>), or an IP address. If specified, the source's IP address will be used as the tracing packet's source IP address, and the 'nw_src'/'ipv6_src' field should not be added in the 'flow' argument.",
							Shorthand: "S",
						},
						{
							Name:      "destination",
							Usage:     "Destination of the packet. Can be an OVS port name, or a (local or remote) Pod or a Service (specified by <Namespace>/<name>). If there are both a Pod and a Service matching the destination name in a Namespace, the Pod will be set as the destination. It can also be an IP address. If specified, the destination's IP address (the ClusterIP for a Service) will be used as the tracing packet's destination IP address, and the 'nw_dst' field should not be added in the 'flow' argument.",
							Shorthand: "D",
						},
						{
							Name:      "flow",
							Usage:     "Specify the flow (packet headers) of the tracing packet. Check the flow syntax descriptions in ovs-ofctl(8) manpage.",
							Shorthand: "f",
						},
						{
							Name:      "addressFamily",
							Usage:     "Specify the address family fo the packet. Can be 4 (IPv4) or 6 (IPv6). If not specified, the addressFamily will be automatically figured out based on the 'flow'. If no IP address or address family is given in the 'flow', IPv4 is used by default.",
							Shorthand: "F",
						},
					},
					OutputType: framework.OutputTypeSingle,
				},
				Formatters: defaultFormatters,
			},
			CommandGroup:        framework.CommandGroupFlat,
			TransformedResponse: reflect.TypeOf(ovstracing.Response{}),
		},
		{ // TODO: implement as a "rawCommand" (see supportbundle) so that the command can be run out-of-cluster
			Use:     "endpoint",
			Aliases: []string{"endpoints"},
			Short:   "Filter network policies relevant to an endpoint.",
			Long:    "Filter network policies relevant to an endpoint into three categories: network policies which apply to the endpoint and policies which select the endpoint in an ingress and/or egress rule.",
			Example: `  Query network policies given Pod and Namespace
  $ antctl query endpoint -p pod1 -n ns1
`,
			CommandGroup: framework.CommandGroupQuery,
			ControllerEndpoint: &framework.CommandEndpoint{
				NonResourceEndpoint: &framework.NonResourceCommandEndpoint{
					Path: "/endpoint",
					Flags: []framework.FlagInfo{
						{
							Name:      "namespace",
							Usage:     "Namespace of the endpoint (defaults to 'default')",
							Shorthand: "n",
						},
						{
							Name:      "pod",
							Usage:     "Name of a Pod endpoint",
							Shorthand: "p",
						},
					},
					OutputType: framework.OutputTypeSingle,
				},
				Formatters: map[framework.FormatterType]framework.FormatterFunc{
					framework.JsonFormatter:  formatter.JSON,
					framework.YamlFormatter:  formatter.YAML,
					framework.TableFormatter: formatter.TableForQuery,
				},
			},
			TransformedResponse: reflect.TypeOf(controllernetworkpolicy.EndpointQueryResponse{}),
		},
	},
	RawCommands: []framework.RawCommand{
		{
			CobraCommand:      supportbundle.Command,
			SupportAgent:      true,
			SupportController: true,
		},
		{
			CobraCommand:      traceflow.Command,
			SupportAgent:      true,
			SupportController: true,
		},
		{
			CobraCommand:      proxy.Command,
			SupportAgent:      false,
			SupportController: true,
		},
	},
	Codec: scheme.Codecs,
}

func generateFlowTableHelpMsg() string {
	msg := ""
	for _, t := range openflow.FlowTables {
		msg += fmt.Sprintf("\n  %d\t%s", uint32(t.Number), t.Name)
	}
	return msg
}
