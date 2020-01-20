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
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"

	"k8s.io/apiserver/pkg/server/mux"
	"k8s.io/klog"

	"github.com/vmware-tanzu/antrea/pkg/monitor"
)

// AgentServer is the antctl server running in antrea agents serves in-pod antctl
// requests.
type AgentServer struct {
	// startOnce ensures the server could only be started once.
	startOnce sync.Once
	listener  net.Listener
}

// Start starts the AgentServer. It guarantees the server could only be started
// once. The server will do graceful stop whenever it receives from the stopCh.
func (s *AgentServer) Start(aq monitor.AgentQuerier, stopCh <-chan struct{}) {
	s.startOnce.Do(func() {
		antctlMux := mux.NewPathRecorderMux("antctl-server")
		CommandList.applyToMux(antctlMux, aq)
		server := &http.Server{Handler: antctlMux}
		// Graceful stop goroutine.
		go func() {
			<-stopCh
			err := server.Shutdown(context.Background())
			if err != nil {
				klog.Errorf("Antctl server stopped with error: %v", err)
			} else {
				klog.Info("Antctl server stopped")
			}
		}()
		// Start the http server.
		go func() {
			klog.Info("Starting antctl server")
			err := server.Serve(s.listener)
			if !errors.Is(err, http.ErrServerClosed) {
				klog.Fatalf("Antctl server stopped with error: %v", err)
			}
		}()
	})
}

// NewAgentServer creates an antctl server. For safety concerns, it creates the
// antctl server which listens on a predefined unix domain socket.
func NewAgentServer() (*AgentServer, error) {
	os.Remove(unixDomainSockAddr)
	ln, err := net.Listen("unix", unixDomainSockAddr)
	if err != nil {
		return nil, fmt.Errorf("error when creating antctl local server: %w", err)
	}
	return &AgentServer{listener: ln}, nil
}
