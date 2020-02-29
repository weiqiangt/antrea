package openflow

import (
	"fmt"

	"github.com/contiv/libOpenflow/openflow13"

	"github.com/contiv/ofnet/ofctrl"
)

type ofGroup struct {
	ofctrl  *ofctrl.Group
	bridge  *OFBridge
	builder *ofGroup
}

func (g *ofGroup) Reset() {
	g.ofctrl.Switch = g.bridge.ofSwitch
}

func (g *ofGroup) Add() error {
	return g.ofctrl.Install()
}

func (g *ofGroup) Modify() error {
	return g.ofctrl.Install()
}

func (g *ofGroup) Delete() error {
	return g.ofctrl.Delete()
}

func (g *ofGroup) Type() EntryType {
	return GroupEntry
}

func (g *ofGroup) KeyString() string {
	return fmt.Sprintf("group_id:%d,bucket_num:%d", g.ofctrl.ID, len(g.ofctrl.Buckets))
}

func (g *ofGroup) Bucket() BucketBuilder {
	return &bucketBuilder{
		group:        g,
		bucket:       openflow13.NewBucket(),
	}
}

func (g *ofGroup) ResetBucket() Group {
	g.ofctrl.Buckets = nil
	return g
}

type bucketBuilder struct {
	group        *ofGroup
	bucket       *openflow13.Bucket
}

func (b *bucketBuilder) LoadReg(regID int, data uint32) BucketBuilder {
	return b.LoadRegRange(regID, data, Range{0, 31})
}

func (b *bucketBuilder) LoadRegRange(regID int, data uint32, rng Range) BucketBuilder {
	reg := fmt.Sprintf("%s%d", NxmFieldReg, regID)
	regField, _ := openflow13.FindFieldHeaderByName(reg, true)
	b.bucket.AddAction(openflow13.NewNXActionRegLoad(rng.ToNXRange().ToOfsBits(), regField, uint64(data)))
	return b
}

func (b *bucketBuilder) ResubmitToTable(tableID TableIDType) BucketBuilder {
	b.bucket.AddAction(openflow13.NewNXActionResubmitTableAction(openflow13.OFPP_IN_PORT, uint8(tableID)))
	return b
}

func (b *bucketBuilder) Weight(val uint16) BucketBuilder {
	b.bucket.Weight = val
	return b
}

func (b *bucketBuilder) Done() Group {
	b.group.ofctrl.AddBuckets(b.bucket)
	return b.group
}
