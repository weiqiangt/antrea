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

	"golang.org/x/sync/errgroup"
	"k8s.io/klog"
)

func NewFan(name string) *Fan {
	return &Fan{
		name: name,
	}
}

type Fan struct {
	name      string
	testCases []TestCase
}

func (f *Fan) Name() string {
	return f.name
}

func (f *Fan) Includes(testCases ...TestCase) TestCase {
	f.testCases = append(f.testCases, testCases...)
	return f
}

func (f *Fan) Run(ctx context.Context, testData TestData) error {
	ctx = wrapWithBreadcrumb(ctx, f.name)
	klog.Infof("Begin: %s", ctx.Value(CtxBreadcrumbs).(string))
	defer klog.Infof("Finish: %s", ctx.Value(CtxBreadcrumbs).(string))

	gErr, ctx := errgroup.WithContext(ctx)
	for i := range f.testCases {
		testCase := f.testCases[i]
		gErr.Go(func() error { return testCase.Run(ctx, testData) })
	}
	return gErr.Wait()
}
