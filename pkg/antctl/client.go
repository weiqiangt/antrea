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
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

// requestOption describes options to issue requests to the antctl server.
type requestOption struct {
	commandDefinition *commandDefinition
	// kubeconfig is the path to the config file for kubectl.
	kubeconfig string
	// args are the parameters of the ongoing request.
	args map[string]string
	// timeout specifies a time limit for requests made by the client. The timeout
	// duration includes connection setup, all redirects, and reading of the
	// response body.
	timeout time.Duration
}

// client issues requests to an antctl server and gets the response.
type client struct {
	// inPod indicate whether the client is running in a pod or not.
	inPod bool
	// codec is the CodecFactory for this command, it is needed for remote accessing.
	codec serializer.CodecFactory
}

// resolveKubeconfig tries to load the kubeconfig specified in the requestOption.
// It will return error if the stating of the file failed or the kubeconfig is malformed.
// It will not try to look up InCluster configuration. If the kubeconfig is loaded,
// the groupVersion and the codec in the requestOption will be populated into the
// kubeconfig object.
func (c *client) resolveKubeconfig(opt *requestOption) (*rest.Config, error) {
	kubeconfig, err := clientcmd.BuildConfigFromFlags("", opt.kubeconfig)
	if err != nil {
		return nil, err
	}
	gv := opt.commandDefinition.controllerEndpoint.groupVersionResource.GroupVersion()
	kubeconfig.GroupVersion = &gv
	kubeconfig.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: c.codec}
	kubeconfig.APIPath = "/apis"
	return kubeconfig, nil
}

// agentRequest issues the local request according to the requestOption.
// localRequest is basically a raw http request, no authentication and authorization
// will be done during the request. If the request succeeds, it will return an
// io.Reader which contains the response data.
func (c *client) agentRequest(opt *requestOption) (io.Reader, error) {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (conn net.Conn, err error) {
				if opt.timeout != 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, opt.timeout)
					defer cancel()
				}
				return new(net.Dialer).DialContext(ctx, "unix", unixDomainSockAddr)
			},
		},
	}
	u, _ := url.ParseRequestURI(opt.commandDefinition.agentRequestPath())
	q := u.Query()
	for k, v := range opt.args {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	u.Scheme = "http"
	u.Host = "antctl"
	klog.Infoln("Issuing request to antrea agent:", u.String())
	resp, err := client.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Since we are going to close the connection, copying the response.
	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	return &buf, err
}

// controllerRequest issues request for the antctl running against antctl-controller.
// This function leverages k8s RestClient to do the request.
func (c *client) controllerRequest(opt *requestOption) (io.Reader, error) {
	klog.Infoln("Issuing request to antrea controller")
	kubeconfig, err := c.resolveKubeconfig(opt)
	if err != nil {
		return nil, err
	}
	restClient, err := rest.RESTClientFor(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create rest client: %w", err)
	}
	// If timeout is zero, there will be no timeout.
	restClient.Client.Timeout = opt.timeout
	resGetter := restClient.Get().
		NamespaceIfScoped(opt.args["namespace"], opt.commandDefinition.controllerEndpoint.namespaced).
		Resource(opt.commandDefinition.controllerEndpoint.groupVersionResource.Resource)

	if len(opt.commandDefinition.controllerEndpoint.resourceName) != 0 {
		resGetter = resGetter.Name(opt.commandDefinition.controllerEndpoint.resourceName)
	} else if name, ok := opt.args["name"]; ok {
		resGetter = resGetter.Name(name)
	}

	result := resGetter.Do()
	if result.Error() != nil {
		return nil, fmt.Errorf("error when requesting %s: %w", resGetter.URL(), result.Error())
	}
	raw, err := result.Raw()
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(raw), nil
}

// Request issues the appropriate request to the antctl server according to the
// request options.
// If the request succeeds, it will return an io.Reader which contains the response
// data.
func (c *client) Request(opt *requestOption) (io.Reader, error) {
	if runtimeComponent == componentAgent {
		return c.agentRequest(opt)
	}
	return c.controllerRequest(opt)
}
