// Copyright 2020 Antrea Authors
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

package types

import (
	"context"
	"fmt"

	"k8s.io/klog"
)

type Repeat struct {
	times    int
	name     string
	testCase TestCase
}

func (r *Repeat) Name() string {
	return fmt.Sprintf("Repeat: %s(x%d)", r.testCase.Name(), r.times)
}

func (r *Repeat) Includes(testCases ...TestCase) TestCase {
	r.testCase = testCases[0]
	return r
}

func (r *Repeat) Run(ctx context.Context, testData TestData) error {
	for i := 0; i < r.times; i++ {
		err := func() error {
			name := fmt.Sprintf("Repeat: %s-%d", r.testCase.Name(), i)
			ctx := wrapWithBreadcrumb(ctx, name)
			klog.Infof("Begin: %s", ctx.Value(CtxBreadcrumbs).(string))
			defer klog.Infof("Finish: %s", ctx.Value(CtxBreadcrumbs).(string))
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return r.testCase.Run(ctx, testData)
			}
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

func NewRepeat(name string, times int) *Repeat {
	return &Repeat{
		times: times,
		name:  name,
	}
}
