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

package framework

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/rest"

	"github.com/vmware-tanzu/antrea/pkg/antctl/runtime"
)

// requestOption describes options to issue requests.
type requestOption struct {
	commandDefinition *CommandDefinition
	// kubeconfig is the path to the config file for kubectl.
	kubeconfig string
	// args are the parameters of the ongoing resourceRequest.
	args map[string]string
	// timeout specifies a time limit for requests made by the client. The timeout
	// duration includes connection setup, all redirects, and reading of the
	// response body.
	timeout time.Duration
	// server is the address and port of the APIServer specified by user explicitly.
	// If not set, antctl will connect to 127.0.0.1:10350 in agent mode, and will
	// connect to the server set in kubeconfig in controller mode.
	// It set, it takes precedence over the above default endpoints.
	server string
}

type configWrapperType func(kubeconfig *rest.Config, codec serializer.CodecFactory) (*rest.Config, error)

// client issues requests to endpoints.
type client struct {
	configWrapper configWrapperType
	// codec is the CodecFactory for this command, it is needed for remote accessing.
	codec serializer.CodecFactory
}

func (c *client) request(opt *requestOption) (io.Reader, error) {
	var e *CommandEndpoint
	if runtime.Mode == runtime.ModeAgent {
		e = opt.commandDefinition.AgentEndpoint
	} else {
		e = opt.commandDefinition.ControllerEndpoint
	}
	if e.ResourceEndpoint != nil {
		return c.resourceRequest(e.ResourceEndpoint, opt)
	}
	return c.nonResourceRequest(e.NonResourceEndpoint, opt)
}

func (c *client) nonResourceRequest(e *NonResourceCommandEndpoint, opt *requestOption) (io.Reader, error) {
	kubeconfig, err := runtime.ResolveKubeconfig(opt.kubeconfig)
	if err != nil {
		return nil, err
	}
	kubeconfig, err = c.configWrapper(kubeconfig, c.codec)
	if err != nil {
		return nil, err
	}

	if opt.server != "" {
		kubeconfig.Host = opt.server
	}
	restClient, err := rest.UnversionedRESTClientFor(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create rest client: %w", err)
	}
	u := url.URL{Path: e.Path}
	q := u.Query()
	for k, v := range opt.args {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	getter := restClient.Get().RequestURI(u.RequestURI()).Timeout(opt.timeout)
	result, err := getter.DoRaw(context.TODO())
	if err != nil {
		statusErr, ok := err.(*errors.StatusError)
		if !ok {
			return nil, err
		}
		return nil, generateMessageForStatusErr(opt.commandDefinition, opt.args, statusErr)
	}
	return bytes.NewReader(result), nil
}

func (c *client) resourceRequest(e *ResourceEndpoint, opt *requestOption) (io.Reader, error) {
	kubeconfig, err := runtime.ResolveKubeconfig(opt.kubeconfig)
	if err != nil {
		return nil, err
	}
	kubeconfig, err = c.configWrapper(kubeconfig, c.codec)
	if err != nil {
		return nil, err
	}
	if opt.server != "" {
		kubeconfig.Host = opt.server
	}
	gv := e.GroupVersionResource.GroupVersion()
	kubeconfig.GroupVersion = &gv
	kubeconfig.APIPath = genericapiserver.APIGroupPrefix

	restClient, err := rest.RESTClientFor(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create rest client: %w", err)
	}
	// If timeout is zero, there will be no timeout.
	restClient.Client.Timeout = opt.timeout

	resGetter := restClient.Get().
		NamespaceIfScoped(opt.args["namespace"], e.Namespaced).
		Resource(e.GroupVersionResource.Resource)

	if len(e.ResourceName) != 0 {
		resGetter = resGetter.Name(e.ResourceName)
	} else if name, ok := opt.args["name"]; ok {
		resGetter = resGetter.Name(name)
	}

	for arg, val := range opt.args {
		if arg != "name" && arg != "namespace" {
			resGetter = resGetter.Param(arg, val)
		}
	}
	result := resGetter.Do(context.TODO())
	if result.Error() != nil {
		return nil, generateMessage(opt, result)
	}
	raw, err := result.Raw()
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(raw), nil
}
