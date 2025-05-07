// Package branches implements caching of branch data
package branches

import (
	"sync"
	"time"

	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
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

		// Cache of state readers. Single state (trie) reader for the branch/root. When accessed through the cache,
		// reading is highly optimized because each state reader keeps its trie cache, so consequent calls to
		// HasUTXO, GetUTXO and similar do not require database involvement during attachment and solidification
		// in the same slot. Inactive cached readers with their trie caches are constantly cleaned up
		stateReaders map[base.TransactionID]*cachedStateReader
	}

	cachedStateReader struct {
		multistate.IndexedStateReader
		lastActivity time.Time
	}
)

const (
	stateReaderTTLSlots        = 2
	branchDataCacheTTLSlots    = 12
	sharedStateReaderCacheSize = 3000
)

func New(env environment) *Branches {
	ret := &Branches{
		environment:      env,
		snapshotBranchID: multistate.FetchSnapshotBranchID(env.StateStore()),
		m:                make(map[base.TransactionID]*multistate.BranchData),
		stateReaders:     make(map[base.TransactionID]*cachedStateReader),
	}
	env.RepeatInBackground("branches_cleanup", 5*time.Second, func() bool {
		ret.mutex.Lock()
		defer ret.mutex.Unlock()

		ret._cleanupCachedStateReaders()
		ret._cleanupBranches()

		return true
	}, true)
	return ret
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
			b.Assertf(bd.LedgerCoverage > bd.CoverageDelta, "bd.LedgerCoverage > bd.CoverageDeltaRaw")
		}
		bd.LastActive = time.Now()
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
		bdRec.LastActive = time.Now()
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

func (b *Branches) _cleanupCachedStateReaders() (int, int) {
	ttl := stateReaderTTLSlots * ledger.SlotDuration()
	count := 0

	for txid, br := range b.stateReaders {
		if time.Since(br.lastActivity) > ttl {
			delete(b.stateReaders, txid)
			count++
		}
	}
	return count, len(b.stateReaders)
}

func (b *Branches) _cleanupBranches() (int, int) {
	ttl := branchDataCacheTTLSlots * ledger.SlotDuration()
	count := 0

	for txid, br := range b.m {
		if time.Since(br.LastActive) > ttl {
			delete(b.m, txid)
			count++
		}
	}
	return count, len(b.m)
}

// GetStateReaderForTheBranch returns a state reader for the branch or nil if the state does not exist.
// If the branch is before the snapshot and branch ID is known in the snapshot state, it returns the snapshot state (which always exists)
func (b *Branches) GetStateReaderForTheBranch(branchID base.TransactionID) multistate.IndexedStateReader {
	util.Assertf(branchID.IsBranchTransaction(), "GetStateReaderForTheBranchExt: branch tx expected. Got: %s", branchID.StringShort())

	snapID := b.SnapshotBranchID()
	switch {
	case branchID.Slot() < snapID.Slot():
		// recursive but won't deadlock because the snapshot state always exists
		snapRdr := b.GetStateReaderForTheBranch(snapID)
		if snapRdr.KnowsCommittedTransaction(branchID) {
			return snapRdr
		}
		return nil
	case branchID.Slot() == snapID.Slot() && branchID != snapID:
		return nil
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()

	ret := b.stateReaders[branchID]
	if ret != nil {
		ret.lastActivity = time.Now()
		return ret.IndexedStateReader
	}
	bd, found := b.getNoLock(branchID)
	if !found {
		return nil
	}
	b.stateReaders[branchID] = &cachedStateReader{
		IndexedStateReader: multistate.MustNewReadable(b.StateStore(), bd.Root, sharedStateReaderCacheSize),
		lastActivity:       time.Now(),
	}
	return b.stateReaders[branchID]
}

func (b *Branches) BranchKnowsTransaction(branchID, txid base.TransactionID) bool {
	util.Assertf(branchID.IsBranchTransaction(), "branch tx expected. Got: %s", branchID.StringShort)

	if branchID == txid {
		return true
	}
	if branchID.Slot() <= txid.Slot() {
		return false
	}
	return b.GetStateReaderForTheBranch(branchID).KnowsCommittedTransaction(txid)

}
