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
	"fmt"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/vmware-tanzu/antrea/pkg/antctl/runtime"
	"github.com/vmware-tanzu/antrea/pkg/client/clientset/versioned/scheme"
)

type testResponse struct {
	Label string `json:"label" antctl:"key"`
	Value uint64 `json:"value"`
}

var testCommandList = &CommandList{
	Definitions: []CommandDefinition{
		{
			Use:                 "test",
			Short:               "test short description ${component}",
			Long:                "test description ${component}",
			TransformedResponse: reflect.TypeOf(testResponse{}),
		},
	},
	Codec: scheme.Codecs,
}

func TestCommandListApplyToCommand(t *testing.T) {
	testRoot := new(cobra.Command)
	testRoot.Short = "The component is ${component}"
	testRoot.Long = "The component is ${component}"
	testCommandList.ApplyToRootCommand(runtime.SetupAntreaKubeconfig, testRoot)
	// sub-commands should be attached
	assert.True(t, testRoot.HasSubCommands())
	// render should work as expected
	assert.Contains(t, testRoot.Short, fmt.Sprintf("The component is %s", runtime.Mode))
	assert.Contains(t, testRoot.Long, fmt.Sprintf("The component is %s", runtime.Mode))
}
