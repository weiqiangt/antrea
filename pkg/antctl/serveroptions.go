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
	"context"
	"net"
	"net/http"
	"os"

	"k8s.io/apiserver/pkg/server/mux"
	"k8s.io/klog"

	"github.com/vmware-tanzu/antrea/pkg/monitor"
)

const unixDomainSockAddr = "/var/run/antctl.sock"

// ServerOptions defines all options of a cli server.
type ServerOptions struct {
	CommandBundle *commandList
	Listener      net.Listener
}

// NewLocalServerOption creates an options of a server accepts only local access.
func NewLocalServerOption() (*ServerOptions, error) {
	os.Remove(unixDomainSockAddr)
	listener, err := net.Listen("unix", unixDomainSockAddr)
	if err != nil {
		return nil, err
	}
	o := &ServerOptions{
		CommandBundle: Manifest,
		Listener:      listener,
	}
	return o, nil
}

// StartServer starts a CLI server configured by the options.
// It returns a channel which tells invoker whether the server stopped.
func StartServer(opts *ServerOptions, aq monitor.AgentQuerier, cq monitor.ControllerQuerier, stopCh <-chan struct{}) <-chan struct{} {
	antctlMux := mux.NewPathRecorderMux("antctl")
	opts.CommandBundle.ApplyToMux(antctlMux, aq, cq)
	server := &http.Server{Handler: antctlMux}
	stoppedCh := make(chan struct{})

	// Request http server graceful stop
	go func() {
		<-stopCh
		server.Shutdown(context.Background())
	}()

	// Run the http server
	go func() {
		klog.Info("Starting antctl server")
		err := server.Serve(opts.Listener)
		if err != nil {
			klog.Errorf("antctl server stopped with error: %v", err)
		}
		close(stoppedCh)
	}()

	return stoppedCh
}
