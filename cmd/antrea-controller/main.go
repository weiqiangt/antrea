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

// Package main under directory cmd parses and validates user input,
// instantiates and initializes objects imported from pkg, and runs
// the process.
package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"k8s.io/component-base/logs"
	"k8s.io/klog"

	"github.com/vmware-tanzu/antrea/pkg/version"
)

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()

	http.Handle("/metrics", promhttp.Handler())

	go func() {
		err := http.ListenAndServe("0.0.0.0:2112", nil)
		if err != nil {
			log.Fatalf("Error when starting metric and pprof server: %v", err)
		}
	}()

	command := newControllerCommand()

	if err := command.Execute(); err != nil {
		logs.FlushLogs()
		os.Exit(1)
	}
}

func newControllerCommand() *cobra.Command {
	opts := newOptions()

	cmd := &cobra.Command{
		Use:  "antrea-controller",
		Long: "The Antrea Controller.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := opts.complete(args); err != nil {
				klog.Fatalf("Failed to complete: %v", err)
			}
			if err := opts.validate(args); err != nil {
				klog.Fatalf("Failed to validate: %v", err)
			}
			if err := run(opts); err != nil {
				klog.Fatalf("Error running controller: %v", err)
			}
		},
		Version: version.GetFullVersionWithRuntimeInfo(),
	}

	flags := cmd.Flags()
	opts.addFlags(flags)
	// Install log flags
	flags.AddGoFlagSet(flag.CommandLine)
	return cmd
}
