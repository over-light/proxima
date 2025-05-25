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

	branchDataWithLedgerCoverage struct {
		*multistate.BranchData
		ledgerCoverage uint64
		lastActive     time.Time
	}
	Branches struct {
		environment
		mutex            sync.Mutex
		snapshotBranchID base.TransactionID
		m                map[base.TransactionID]branchDataWithLedgerCoverage

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
		m:                make(map[base.TransactionID]branchDataWithLedgerCoverage),
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

func (b *Branches) Get(branchTxID base.TransactionID) *multistate.BranchData {
	util.Assertf(branchTxID.IsBranchTransaction(), "branch transaction ID expected. Got %s", branchTxID.StringShort)

	b.mutex.Lock()
	defer b.mutex.Unlock()

	if ret, ok := b._getAndCacheNoLock(branchTxID); ok {
		return ret.BranchData
	}
	return nil
}

func (b *Branches) SnapshotBranchID() base.TransactionID {
	return b.snapshotBranchID
}

func (b *Branches) SnapshotSlot() base.Slot {
	return b.snapshotBranchID.Slot()
}

func (b *Branches) _getAndCacheNoLock(branchID base.TransactionID) (branchDataWithLedgerCoverage, bool) {
	bd, ok := b.m[branchID]
	if ok {
		if branchID.Slot() > 0 {
			b.Assertf(bd.ledgerCoverage == 0 || bd.ledgerCoverage >= bd.CoverageDelta, "bd.ledgerCoverage == 0 || bd.LedgerCoverage(%s) >= bd.CoverageDeltaRaw(%s) for %s",
				util.Th(bd.ledgerCoverage), util.Th(bd.CoverageDelta), branchID.StringShort)
		}
		bd.lastActive = time.Now()
		b.m[branchID] = bd
		return bd, true
	}

	if branchID.Slot() < b.snapshotBranchID.Slot() ||
		(branchID.Slot() == b.snapshotBranchID.Slot() && branchID != b.snapshotBranchID) {
		// the branch is impossible assuming the snapshot baseline
		return branchDataWithLedgerCoverage{}, false
	}

	// fetch branch from the database
	if rd, found := multistate.FetchRootRecord(b.StateStore(), branchID); found {
		bdRec := multistate.FetchBranchDataByRoot(b.StateStore(), rd)
		bd = branchDataWithLedgerCoverage{
			BranchData:     &bdRec,
			ledgerCoverage: 0, // will be lazy-calculated when needed
			lastActive:     time.Now(),
		}
		b.m[branchID] = bd
		return bd, true
	}
	return branchDataWithLedgerCoverage{}, false
}

// _ledgerCoverage traverses branches back up to 64 slots and calculates full coverage
func (b *Branches) _ledgerCoverage(brOrig branchDataWithLedgerCoverage) (ret uint64) {
	var slotsBack uint32
	var ok bool

	origSlot := brOrig.Slot()
	ret = brOrig.CoverageDelta
	br := brOrig

	for slotsBack < 64 {
		predID := br.StemPredecessorBranchID()
		if br, ok = b._getAndCacheNoLock(predID); !ok {
			break
		}
		slotsBack = uint32(origSlot - predID.Slot())
		if br.ledgerCoverage > 0 {
			ret += br.ledgerCoverage >> slotsBack
			break
		}
		ret += br.CoverageDelta >> slotsBack
	}
	return
}

// LedgerCoverage strictly speaking, is non-deterministic if the snapshot is after the genesis
// However:
//   - if branchID is far enough (63 slots), it is guaranteed to be the real value and therefore deterministic
//   - if the snapshot is N slots behind the branchID, it is guaranteed that the returned value differs from
//     the real value no more than by 1/2^N
func (b *Branches) LedgerCoverage(branchID base.TransactionID) uint64 {
	util.Assertf(branchID.IsBranchTransaction(), "branch transaction ID expected. Got %s", branchID.StringShort)

	b.mutex.Lock()
	defer b.mutex.Unlock()

	bd, ok := b._getAndCacheNoLock(branchID)
	if !ok {
		return 0
	}
	if bd.ledgerCoverage > 0 {
		return bd.ledgerCoverage
	}
	bd.ledgerCoverage = b._ledgerCoverage(bd)
	b.Assertf(bd.ledgerCoverage > 0, "LedgerCoverage: bd.ledgerCoverage > 0 for %s", branchID.StringShort)

	b.m[branchID] = bd
	return bd.ledgerCoverage
}

func (b *Branches) Supply(branchID base.TransactionID) uint64 {
	util.Assertf(branchID.IsBranchTransaction(), "branch transaction ID expected. Got %s", branchID.StringShort)

	b.mutex.Lock()
	defer b.mutex.Unlock()

	if bd, ok := b._getAndCacheNoLock(branchID); ok {
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
		if time.Since(br.lastActive) > ttl {
			delete(b.m, txid)
			count++
		}
	}
	return count, len(b.m)
}

func (b *Branches) SequencerOutputID(branchID base.TransactionID) (base.OutputID, bool) {
	util.Assertf(branchID.IsBranchTransaction(), "branch transaction ID expected. Got %s", branchID.StringShort)
	b.mutex.Lock()
	defer b.mutex.Unlock()

	bd, ok := b._getAndCacheNoLock(branchID)
	if !ok {
		return base.OutputID{}, false
	}
	return bd.SequencerOutput.ID, true
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
	bd, found := b._getAndCacheNoLock(branchID)
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

func (b *Branches) TransactionIsInSnapshotState(txid base.TransactionID) bool {
	if txid.Timestamp().After(b.snapshotBranchID.Timestamp()) {
		return false
	}
	return b.BranchKnowsTransaction(b.snapshotBranchID, txid)
}

//func (b *Branches) IterateBranchesBack(tip base.TransactionID, fun func(branchID base.TransactionID, branchData *multistate.BranchData) bool) {
//	b.mutex.Lock()
//	defer b.mutex.Unlock()
//
//	bd, ok := b._getAndCacheNoLock(tip)
//	for ok && fun(tip, bd) {
//		tip = bd.StemPredecessorBranchID()
//		bd, ok = b.getNoLock(tip)
//	}
//}

// works badly in startup, where enough to have lrb from DB, i.e., without recursively calculated coverage

//func (b *Branches) FindLatestReliableBranch(fraction global.Fraction) *multistate.BranchData {
//	tipRoots, ok := multistate.FindRootsFromLatestHealthySlot(b.StateStore(), fraction)
//	if !ok {
//		return nil
//	}
//	b.Assertf(len(tipRoots) > 0, "healthyRoots is empty")
//	tipRoots = util.PurgeSlice(tipRoots, func(rr multistate.RootRecord) bool {
//		return global.IsHealthyCoverageDelta(rr.CoverageDelta, rr.Supply, fraction)
//	})
//	util.Assertf(len(tipRoots) > 0, "len(tipRoots)>0")
//
//	if len(tipRoots) == 1 {
//		// if only one branch is in the latest healthy slot, it is the one reliable
//		bd, ok := b.Get(multistate.FetchBranchIDByRoot(b.StateStore(), tipRoots[0].Root))
//		util.Assertf(ok, "inconsistency: branchID by root not found")
//		return util.Ref(bd)
//	}
//
//	rootMaxIdx := util.IndexOfMaximum(tipRoots, func(i, j int) bool {
//		return tipRoots[i].CoverageDelta < tipRoots[j].CoverageDelta
//	})
//	util.Assertf(global.IsHealthyCoverageDelta(tipRoots[rootMaxIdx].CoverageDelta, tipRoots[rootMaxIdx].Supply, fraction),
//		"global.IsHealthyCoverageDelta(rootMax.LedgerCoverage, rootMax.Supply, fraction)")
//
//	tipBranchID := multistate.FetchBranchIDByRoot(b.StateStore(), tipRoots[rootMaxIdx].Root)
//
//	readers := make([]*multistate.Readable, 0, len(tipRoots)-1)
//	for i := range tipRoots {
//		// no need to check in the main tip, skip it
//		if !ledger.CommitmentModel.EqualCommitments(tipRoots[i].Root, tipRoots[rootMaxIdx].Root) {
//			readers = append(readers, multistate.MustNewReadable(b.StateStore(), tipRoots[i].Root))
//		}
//	}
//	util.Assertf(len(readers) > 0, "len(readers) > 0")
//
//	var branchFound *multistate.BranchData
//	first := true
//
//	b.IterateBranchesBack(tipBranchID, func(branchID base.TransactionID, bd *multistate.BranchData) bool {
//		if first {
//			// skip the tip itself
//			first = false
//			return true
//		}
//		// check if the branch is included in every reader
//		for _, rdr := range readers {
//			if !rdr.KnowsCommittedTransaction(branchID) {
//				// the transaction is not known by at least one of selected states,
//				// it is not a reliable branch, keep traversing back
//				return true
//			}
//		}
//		// branchID is known in all tip states. It is the reliable one
//		branchFound = bd
//		return false
//	})
//	return branchFound
//}
