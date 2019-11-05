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
	"path"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

// RequestOption describes options to issue requests to the antctl server.
type RequestOption struct {
	// Kubeconfig is the path to the config file for kubectl.
	Kubeconfig string
	// Name is the Use of the command going to be requested.
	Name string
	// Args are the parameters of the ongoing request.
	Args map[string]string
	// Timeout specifies a time limit for requests made by the client. The timeout
	// includes connection time, any redirects, and reading the response body.
	TimeOut time.Duration
}

// URI constructs the request URI of the request option. It expands Args to query
// parameters in URL. It returns the URI in the format: "/apis/(API Group)/(API Version)/(command Use)[?parameters]".
func (ro *RequestOption) URI(version *schema.GroupVersion) string {
	u := new(url.URL)
	u.Path = path.Join("/apis", version.Group, version.Version, ro.Name)

	q := u.Query()
	for k, v := range ro.Args {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	return u.RequestURI()
}

// client issues requests to an antctl server and gets the response.
type client struct {
	// inPod tells the running mode of the client.
	inPod bool
	// groupVersion describes the group version the corresponding antctl server be installed.
	groupVersion *schema.GroupVersion
	// codec is the CodecFactory for this command, it is needed for remote accessing.
	codec serializer.CodecFactory
}

// resolveKubeconfig tries to load the kubeconfig specified in the RequestOption.
// It will return error if the stating of the file failed or the kubeconfig is malformed.
// It will not try to look up InCluster configuration. If the kubeconfig is loaded,
// The GroupVersion and the Codec in the RequestOption will be populated into the
// kubeconfig object.
func (c *client) resolveKubeconfig(opt *RequestOption) (*rest.Config, error) {
	kubeconfig, err := clientcmd.BuildConfigFromFlags("", opt.Kubeconfig)
	if err != nil {
		return nil, err
	}
	kubeconfig.GroupVersion = c.groupVersion
	kubeconfig.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: c.codec}
	return kubeconfig, nil
}

// localRequest issues the local request according to the RequestOption. It only cares
// the groupVersion of the RequestOption which will be used to construct the request
// URI. localRequest is basically a raw http request, none authentication and authorization
// will be done during the request. For safety concerns, it communicates with the
// antctl server by a predefined unix domain socket. If the request succeeded, it
// will returns an io.Reader which contains the response data.
func (c *client) localRequest(opt *RequestOption) (io.Reader, error) {
	uri := opt.URI(c.groupVersion)
	klog.Infof("Requesting URI %s", uri)
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (conn net.Conn, err error) {
				var cancel context.CancelFunc
				if opt.TimeOut != 0 {
					ctx, cancel = context.WithTimeout(ctx, opt.TimeOut)
					defer cancel()
				}
				return new(net.Dialer).DialContext(ctx, "unix", unixDomainSockAddr)
			},
		},
	}
	resp, err := client.Get("http://antctl" + uri)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Since we are going to close the connection, copying the response.
	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	return &buf, err
}

// Request issues the appropriate request to the antctl server according to the
// request options. If the inPod filed of the client is true, the client will do
// a local request by invoking localRequest. Otherwise, it will check the kubeconfig
// and request the antctl server by the delegation of the kubernetes API server.
// If the request succeeded, it will returns an io.Reader which contains the response
// data.
func (c *client) Request(opt *RequestOption) (io.Reader, error) {
	if c.inPod {
		klog.Infoln("Antctl runs as local mode")
		return c.localRequest(opt)
	}
	kubeconfig, err := c.resolveKubeconfig(opt)
	if err != nil {
		return nil, err
	}
	restClient, err := rest.RESTClientFor(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create rest client: %w", err)
	}
	// If timeout is not set, no timeout.
	restClient.Client.Timeout = opt.TimeOut
	uri := opt.URI(c.groupVersion)
	result := restClient.Get().RequestURI(uri).Do()
	if result.Error() != nil {
		return nil, fmt.Errorf("error when requesting URI %s: %w", uri, result.Error())
	}
	raw, err := result.Raw()
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(raw), nil
}
