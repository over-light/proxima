package multistate

import (
	"sync"

	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/unitrie/common"
)

type (
	Branches struct {
		mutex sync.Mutex
		store StateStoreReader
		m     map[base.TransactionID]*BranchData
	}
)

func NewBranches(store StateStoreReader) *Branches {
	return &Branches{
		store: store,
		m:     make(map[base.TransactionID]*BranchData),
	}
}

func (b *Branches) Get(branchTxID base.TransactionID) (BranchData, bool) {
	util.Assertf(branchTxID.IsBranchTransaction(), "branch transaction ID expected. Got %s", branchTxID.StringShort)

	b.mutex.Lock()
	defer b.mutex.Unlock()

	ret, ok := b.getNoLock(branchTxID)
	return *ret, ok
}

func (b *Branches) getNoLock(branchTxID base.TransactionID) (*BranchData, bool) {
	if bd, ok := b.m[branchTxID]; ok {
		return bd, true
	}
	if rd, found := FetchRootRecord(b.store, branchTxID); found {
		bdRec := FetchBranchDataByRoot(b.store, rd)
		bdRec.LedgerCoverage = bdRec.CoverageDelta + b.calcLedgerCoveragePast(bdRec.TxID(), bdRec.StemPredecessorBranchID())
		b.m[branchTxID] = &bdRec
	}
	return nil, false
}

func (b *Branches) calcLedgerCoveragePast(branchID, predBranchID base.TransactionID) uint64 {
	slot := branchID.Slot()
	slotPred := predBranchID.Slot()
	util.Assertf(slotPred < slot, "slotPred < slot")

	shift := slot - slotPred
	if shift >= 64 {
		return 0
	}
	bdPred, ok := b.getNoLock(predBranchID)
	if !ok {
		return 0
	}
	return bdPred.LedgerCoverage >> shift

}

// FetchBranchData returns branch data by the branch transaction id
// Deprecated: use Get instead
func FetchBranchData(store common.KVReader, branchTxID base.TransactionID) (BranchData, bool) {
	if rd, found := FetchRootRecord(store, branchTxID); found {
		return FetchBranchDataByRoot(store, rd), true
	}
	return BranchData{}, false
}
