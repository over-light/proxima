package memdag

import (
	"bytes"
	"sort"
	"time"

	"github.com/lunfardo314/proxima/core/vertex"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/lines"
	"github.com/lunfardo314/proxima/util/set"
)

func (d *MemDAG) Info(verbose ...bool) string {
	return d.InfoLines(verbose...).String()
}

func (d *MemDAG) InfoLines(verbose ...bool) *lines.Lines {
	ln := lines.New()

	slots := d._timeSlotsOrdered()

	d.mutex.RLock()
	ln.Add("MemDAG:: vertices: %d, stateReaders: %d, slots: %d",
		len(d.vertices), len(d.stateReaders), len(slots))
	d.mutex.RUnlock()

	if len(verbose) > 0 && verbose[0] {
		verticesWithFlags := d.VerticesWitExpirationFlag()
		ln.Add("---- all vertices (verbose)")
		vertices := util.KeysSorted(verticesWithFlags, func(vid1, vid2 *vertex.WrappedTx) bool {
			return vid1.SlotWhenAdded < vid2.SlotWhenAdded
		})
		for _, vid := range vertices {
			ln.Add("    %s -- expired: %v", vid.ShortString(), verticesWithFlags[vid])
		}

		ln.Add("---- cached state readers (verbose)")
		func() {
			d.stateReadersMutex.RLock()
			defer d.stateReadersMutex.RUnlock()

			branches := util.KeysSorted(d.stateReaders, func(id1, id2 base.TransactionID) bool {
				return bytes.Compare(id1[:], id2[:]) < 0
			})
			for _, br := range branches {
				rdrData := d.stateReaders[br]
				ln.Add("    %s, last activity %v", br.StringShort(), time.Since(rdrData.lastActivity))
			}
		}()
	}
	return ln
}

func (d *MemDAG) InfoRefLines(prefix ...string) *lines.Lines {
	ln := lines.New(prefix...)
	vert := d.Vertices()
	sort.Slice(vert, func(i, j int) bool {
		return vert[i].Timestamp().Before(vert[j].Timestamp())
	})
	for _, vid := range vert {
		past := vid.FindPastReferencesSuchAs(func(vid *vertex.WrappedTx) bool {
			return true // vid.Slot() <= 1
		})
		if len(past) == 0 {
			ln.Add(" %s -> no past references", vid.ShortString())
		} else {
			ln.Add(" %s -> %d past references", vid.ShortString(), len(past))
			for _, ref := range past {
				ln.Add("    --> %s", ref.ShortString())
			}
		}
	}
	return ln
}

func (d *MemDAG) VerticesInSlotAndAfter(slot base.Slot) []*vertex.WrappedTx {
	ret := d.VerticesFiltered(func(txid base.TransactionID) bool {
		return txid.Slot() >= slot
	})
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Timestamp().Before(ret[j].Timestamp())
	})
	return ret
}

func (d *MemDAG) LinesVerticesInSlotAndAfter(slot base.Slot) *lines.Lines {
	return vertex.VerticesLines(d.VerticesInSlotAndAfter(slot))
}

func (d *MemDAG) _timeSlotsOrdered(descOrder ...bool) []base.Slot {
	desc := false
	if len(descOrder) > 0 {
		desc = descOrder[0]
	}
	slots := set.New[base.Slot]()
	for br := range d.stateReaders {
		slots.Insert(br.Slot())
	}
	if desc {
		return util.KeysSorted(slots, func(e1, e2 base.Slot) bool {
			return e1 > e2
		})
	}
	return util.KeysSorted(slots, func(e1, e2 base.Slot) bool {
		return e1 < e2
	})
}

func (d *MemDAG) FetchSummarySupplyAndInflation(nBack int) *multistate.SummarySupplyAndInflation {
	return multistate.FetchSummarySupply(d.StateStore(), nBack)
}

//
//func (ut *MemDAG) MustAccountInfoOfHeaviestBranch() *multistate.AccountInfo {
//	return multistate.MustCollectAccountInfo(ut.stateStore, ut.HeaviestStateRootForLatestTimeSlot())
//}
