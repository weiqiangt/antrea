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

package cookie

import (
	"fmt"
)

const (
	BitwidthRound    = 16
	BitwidthCategory = 8
	BitwidthReserved = 64 - BitwidthCategory - BitwidthRound
)

// Category represents the flow entry category.
type Category uint64

const (
	Default Category = iota
	Gateway
	Node
	Pod
	Service
	Policy
)

func (c Category) String() string {
	switch c {
	case Default:
		return "Default"
	case Gateway:
		return "Gateway"
	case Node:
		return "Node"
	case Pod:
		return "Pod"
	case Service:
		return "Service"
	case Policy:
		return "Policy"
	default:
		return "Invalid"
	}
}

// ID defines segments a cookie ID contains. An ID is composed like:
//
// |------------------------- ID --------------------------|
//
// |- round 16bits -|- category 8bits -|- reserved 40bits -|
// The round segment represents the round id.
// The category segment represents the category of flow this ID belongs.

type ID uint64

func newID(round uint64, cat Category) ID {
	r := uint64(0)
	r |= round << (64 - BitwidthRound)
	r |= (uint64(cat) << (BitwidthReserved + BitwidthRound)) >> (BitwidthRound)
	return ID(r)
}

// Raw returns the unit64 type value of the ID
func (i ID) Raw() uint64 {
	return uint64(i)
}

// Round returns the round number of the ID
func (i ID) Round() uint64 {
	return uint64(i) >> (64 - BitwidthRound)
}

// Category returns the category of the ID
func (i ID) Category() Category {
	return Category((uint64(i) << BitwidthRound) >> (64 - BitwidthCategory))
}

// Category returns the string representation of the ID
func (i ID) String() string {
	return fmt.Sprintf("<round:%d,category:%s>", i.Round(), i.Category().String())
}

// Allocator defines operations of a cookie ID allocator.
type Allocator interface {
	// Request cookie IDs of flow categories.
	Request(cat Category) ID
}

type allocator struct {
	round uint64
}

// Mask returns a ID with the given category.
func (a *allocator) Request(cat Category) ID {
	return newID(a.round, cat)
}

// Mask returns a mask to match specific category of flows.
func (a *allocator) Mask(cat Category) uint64 {
	return newID(a.round, cat).Raw()
}

// NewAllocator creates a cookie ID allocator by using the given round number. Only last 16 bits of the round number would be used.
func NewAllocator(round uint64) Allocator {
	return &allocator{round: round}
}
