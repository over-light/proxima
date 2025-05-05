// Package branches implements caching of branch data
package branches

import (
	"sync"

	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/util"
)

type (
	environment interface {
		global.NodeGlobal
		StateStore() multistate.StateStore
	}

	Branches struct {
		environment
		mutex            sync.Mutex
		snapshotBranchID base.TransactionID
		m                map[base.TransactionID]*multistate.BranchData
	}
)

func New(env environment) *Branches {
	return &Branches{
		environment:      env,
		snapshotBranchID: multistate.FetchSnapshotBranchID(env.StateStore()),
		m:                make(map[base.TransactionID]*multistate.BranchData),
	}
}

func (b *Branches) Get(branchTxID base.TransactionID) (multistate.BranchData, bool) {
	util.Assertf(branchTxID.IsBranchTransaction(), "branch transaction ID expected. Got %s", branchTxID.StringShort)

	b.mutex.Lock()
	defer b.mutex.Unlock()

	if ret, ok := b.getNoLock(branchTxID); ok {
		return *ret, ok
	}
	return multistate.BranchData{}, false
}

func (b *Branches) SnapshotBranchID() base.TransactionID {
	return b.snapshotBranchID
}

func (b *Branches) SnapshotSlot() base.Slot {
	return b.snapshotBranchID.Slot()
}

func (b *Branches) getNoLock(branchID base.TransactionID) (*multistate.BranchData, bool) {
	if bd, ok := b.m[branchID]; ok {
		if branchID.Slot() > 0 {
			// branch record is in the cache. Ledger coverage (calculated) must always be bigger than coverage delta
			b.Assertf(bd.LedgerCoverage > bd.CoverageDelta, "bd.LedgerCoverage > bd.CoverageDelta")
		}
		return bd, true
	}

	if branchID.Slot() < b.snapshotBranchID.Slot() ||
		(branchID.Slot() == b.snapshotBranchID.Slot() && branchID != b.snapshotBranchID) {
		// the branch is impossible assuming the snapshot baseline
		return nil, false
	}

	// fetch branch from the database and calculate ledger coverage
	if rd, found := multistate.FetchRootRecord(b.StateStore(), branchID); found {
		bdRec := multistate.FetchBranchDataByRoot(b.StateStore(), rd)
		b.calcAndCacheLedgerCoverage(branchID.Slot(), &bdRec)
		b.m[branchID] = &bdRec
		return &bdRec, true
	}
	return nil, false
}

// calcAndCacheLedgerCoverage traverses branches back and calculate full coverage
func (b *Branches) calcAndCacheLedgerCoverage(branchSlot base.Slot, bdRec *multistate.BranchData) {
	bdRec.LedgerCoverage = bdRec.CoverageDelta
	contrib := bdRec.CoverageDelta
	for rec := b.predBranchRecord(bdRec); rec != nil && contrib > 0; rec = b.predBranchRecord(rec) {
		b.Assertf(rec.Stem.ID.Slot() < branchSlot, "")
		contrib = rec.CoverageDelta >> (branchSlot - rec.Stem.ID.Slot())
		bdRec.LedgerCoverage += contrib
	}
}

func (b *Branches) predBranchRecord(br *multistate.BranchData) *multistate.BranchData {
	if ret, ok := b.getNoLock(br.StemPredecessorBranchID()); ok {
		return ret
	}
	return nil
}

// LedgerCoverage strictly speaking, is non-deterministic if the snapshot is after genesis
// However:
//   - if branchID is far enough (63 slots), it is guaranteed to be the real value and therefore deterministic
//   - if the snapshot is N slots behind the branchID, it is guaranteed that the returned value differs from
//     the real value no more than by 1/2^N
func (b *Branches) LedgerCoverage(branchID base.TransactionID) uint64 {
	util.Assertf(branchID.IsBranchTransaction(), "branch transaction ID expected. Got %s", branchID.StringShort)

	b.mutex.Lock()
	defer b.mutex.Unlock()

	if bd, ok := b.getNoLock(branchID); ok {
		return bd.LedgerCoverage
	}
	return 0
}

func (b *Branches) Supply(branchID base.TransactionID) uint64 {
	util.Assertf(branchID.IsBranchTransaction(), "branch transaction ID expected. Got %s", branchID.StringShort)

	b.mutex.Lock()
	defer b.mutex.Unlock()

	if bd, ok := b.getNoLock(branchID); ok {
		return bd.Supply
	}
	return 0
}
