package branches

import (
	"sync"

	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/unitrie/common"
)

type (
	environment interface {
		global.NodeGlobal
		StateStore() multistate.StateStore
	}

	Branches struct {
		environment
		mutex sync.Mutex
		m     map[base.TransactionID]*multistate.BranchData
	}
)

func New(env environment) *Branches {
	return &Branches{
		environment: env,
		m:           make(map[base.TransactionID]*multistate.BranchData),
	}
}

func (b *Branches) Get(branchTxID base.TransactionID) (multistate.BranchData, bool) {
	util.Assertf(branchTxID.IsBranchTransaction(), "branch transaction ID expected. Got %s", branchTxID.StringShort)

	b.mutex.Lock()
	defer b.mutex.Unlock()

	ret, ok := b.getNoLock(branchTxID)
	return *ret, ok
}

func (b *Branches) getNoLock(branchTxID base.TransactionID) (*multistate.BranchData, bool) {
	if bd, ok := b.m[branchTxID]; ok {
		return bd, true
	}
	if rd, found := multistate.FetchRootRecord(b.StateStore(), branchTxID); found {
		bdRec := multistate.FetchBranchDataByRoot(b.StateStore(), rd)
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
func FetchBranchData(store common.KVReader, branchTxID base.TransactionID) (multistate.BranchData, bool) {
	if rd, found := multistate.FetchRootRecord(store, branchTxID); found {
		return multistate.FetchBranchDataByRoot(store, rd), true
	}
	return multistate.BranchData{}, false
}
