package tippool

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/lunfardo314/proxima/core/vertex"
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/set"
	"go.uber.org/atomic"
)

type (
	Environment interface {
		global.Logging
		ListenToAccount(account ledger.Accountable, fun func(wOut vertex.WrappedOutput))
		ListenToSequencers(fun func(vid *vertex.WrappedTx))
		PullSequencerTips(seqID ledger.ChainID, loadOwnMilestones bool) (set.Set[vertex.WrappedOutput], error)
		SequencerID() ledger.ChainID
	}

	SequencerTipPool struct {
		Environment
		mutex                    sync.RWMutex
		outputs                  set.Set[vertex.WrappedOutput]
		name                     string
		latestMilestones         map[ledger.ChainID]*vertex.WrappedTx
		lastPruned               atomic.Time
		outputCount              int
		removedOutputsSinceReset int
	}

	Stats struct {
		NumOtherSequencers       int
		NumOutputs               int
		OutputCount              int
		RemovedOutputsSinceReset int
	}
)

// TODO tag-along and delegation locks

type Option byte

// OptionDoNotLoadOwnMilestones is used for tests only
const OptionDoNotLoadOwnMilestones = Option(iota)

func New(env Environment, namePrefix string, opts ...Option) (*SequencerTipPool, error) {
	seqID := env.SequencerID()
	ret := &SequencerTipPool{
		Environment:      env,
		outputs:          set.New[vertex.WrappedOutput](),
		name:             fmt.Sprintf("%s-%s", namePrefix, seqID.StringVeryShort()),
		latestMilestones: make(map[ledger.ChainID]*vertex.WrappedTx),
	}
	env.Tracef("tippool", "starting tipPool..")

	ret.mutex.RLock()
	defer ret.mutex.RUnlock()

	// start listening to chain-locked account
	env.ListenToAccount(seqID.AsChainLock(), func(wOut vertex.WrappedOutput) {
		env.Tracef("tippool", "[%s] output IN: %s", ret.name, wOut.IDShortString)
		ret.purge()

		if !isCandidateToTagAlong(wOut) {
			return
		}

		ret.mutex.Lock()
		defer ret.mutex.Unlock()

		ret.outputs.Insert(wOut)
		ret.outputCount++
		env.Tracef("tippool", "output stored in tippool: %s", wOut.IDShortString)
	})

	// start listening to sequencers, including the current sequencer
	env.ListenToSequencers(func(vid *vertex.WrappedTx) {
		env.Tracef("tippool", "seq milestone IN: %s", vid.IDShortString)
		seqIDIncoming, ok := vid.SequencerIDIfAvailable()
		util.Assertf(ok, "sequencer milestone expected")

		ret.mutex.Lock()
		defer ret.mutex.Unlock()

		old, prevExists := ret.latestMilestones[seqIDIncoming]
		if !prevExists || !vid.Timestamp().Before(old.Timestamp()) {
			// store the latest one for each seqID
			ret.latestMilestones[seqIDIncoming] = vid
		}
		env.Tracef("tippool", "seq milestone stored in tippool: %s", vid.IDShortString)
	})

	// fetch all sequencers and all outputs in the sequencer account into to tip pool once
	var err error
	doNotLoadOwnMilestones := slices.Index(opts, OptionDoNotLoadOwnMilestones) >= 0
	env.Tracef("tippool", "PullSequencerTips: doNotLoadOwnMilestones = %v", doNotLoadOwnMilestones)
	ret.outputs, err = env.PullSequencerTips(seqID, !doNotLoadOwnMilestones)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (tp *SequencerTipPool) OtherMilestonesSorted() []*vertex.WrappedTx {
	tp.mutex.RLock()
	defer tp.mutex.RUnlock()

	ret := make([]*vertex.WrappedTx, 0, len(tp.latestMilestones))
	ownSeqID := tp.SequencerID()
	for seqID, vid := range tp.latestMilestones {
		if seqID != ownSeqID {
			ret = append(ret, vid)
		}
	}
	sort.Slice(ret, func(i, j int) bool {
		return isPreferredMilestoneAgainstTheOther(ret[i], ret[j])
	})
	return ret
}

func (tp *SequencerTipPool) GetOwnLatestMilestoneTx() *vertex.WrappedTx {
	tp.mutex.RLock()
	defer tp.mutex.RUnlock()

	return tp.latestMilestones[tp.SequencerID()]
}

func isCandidateToTagAlong(wOut vertex.WrappedOutput) bool {
	if wOut.VID.IsBadOrDeleted() {
		return false
	}
	if wOut.VID.IsBranchTransaction() {
		// outputs of branch transactions are filtered out
		// TODO probably ordinary outputs must not be allowed at ledger level
		return false
	}
	o, err := wOut.VID.OutputAt(wOut.Index)
	if err != nil {
		return false
	}
	if o != nil {
		if _, idx := o.ChainConstraint(); idx != 0xff {
			// filter out all chain constrained outputs
			// TODO must be revisited with delegated accounts (delegation-locked on the current sequencer)
			return false
		}
	}
	return true
}

// TODO purge also bad ones
func (tp *SequencerTipPool) purge() {
	cleanupPeriod := ledger.SlotDuration() / 2
	if time.Since(tp.lastPruned.Load()) < cleanupPeriod {
		return
	}

	tp.mutex.Lock()
	defer tp.mutex.Unlock()

	toDelete := make([]vertex.WrappedOutput, 0)
	for wOut := range tp.outputs {
		if !isCandidateToTagAlong(wOut) {
			toDelete = append(toDelete, wOut)
		}
	}
	for _, wOut := range toDelete {
		delete(tp.outputs, wOut)
	}
	tp.removedOutputsSinceReset += len(toDelete)

	toDeleteMilestoneChainID := make([]ledger.ChainID, 0)
	for chainID, vid := range tp.latestMilestones {
		if vid.IsBadOrDeleted() {
			toDeleteMilestoneChainID = append(toDeleteMilestoneChainID, chainID)
		}
	}
	for i := range toDeleteMilestoneChainID {
		delete(tp.latestMilestones, toDeleteMilestoneChainID[i])
	}
	tp.lastPruned.Store(time.Now())
}

func (tp *SequencerTipPool) FilterAndSortOutputs(filter func(wOut vertex.WrappedOutput) bool) []vertex.WrappedOutput {
	tp.purge()

	tp.mutex.RLock()
	defer tp.mutex.RUnlock()

	ret := util.KeysFiltered(tp.outputs, filter)
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Timestamp().Before(ret[j].Timestamp())
	})
	return ret
}

func (tp *SequencerTipPool) preSelectAndSortEndorsableMilestones(targetTs ledger.LogicalTime) []*vertex.WrappedTx {
	tp.purge()

	tp.mutex.RLock()
	defer tp.mutex.RUnlock()

	ret := make([]*vertex.WrappedTx, 0)
	for _, ms := range tp.latestMilestones {
		if ms.Slot() != targetTs.Slot() || !ledger.ValidTimePace(ms.Timestamp(), targetTs) {
			continue
		}
		ret = append(ret, ms)
	}
	sort.Slice(ret, func(i, j int) bool {
		return isPreferredMilestoneAgainstTheOther(ret[i], ret[j]) // order is important !!!
	})
	return ret
}

// betterMilestone returns if vid1 is strongly better than vid2
func isPreferredMilestoneAgainstTheOther(vid1, vid2 *vertex.WrappedTx) bool {
	util.Assertf(vid1.IsSequencerMilestone() && vid2.IsSequencerMilestone(), "vid1.IsSequencerMilestone() && vid2.IsSequencerMilestone()")

	if vid1 == vid2 {
		return false
	}
	if vid2 == nil {
		return true
	}

	coverage1 := vid1.LedgerCoverageSum()
	coverage2 := vid2.LedgerCoverageSum()
	switch {
	case coverage1 > coverage2:
		// main preference is by ledger coverage
		return true
	case coverage1 == coverage2:
		if vid1.Timestamp() == vid2.Timestamp() {
			// prefer with bigger hash
			return bytes.Compare(vid1.ID[:], vid2.ID[:]) > 0
		}
		// prefer younger
		return vid2.Timestamp().Before(vid1.Timestamp())
	default:
		return false
	}
}

func (tp *SequencerTipPool) NumOutputsInBuffer() int {
	tp.mutex.RLock()
	defer tp.mutex.RUnlock()

	return len(tp.outputs)
}

func (tp *SequencerTipPool) NumMilestones() int {
	tp.mutex.RLock()
	defer tp.mutex.RUnlock()

	return len(tp.latestMilestones)
}

func (tp *SequencerTipPool) getStatsAndReset() (ret Stats) {
	tp.mutex.RLock()
	defer tp.mutex.RUnlock()

	ret = Stats{
		NumOtherSequencers:       len(tp.latestMilestones),
		NumOutputs:               len(tp.outputs),
		OutputCount:              tp.outputCount,
		RemovedOutputsSinceReset: tp.removedOutputsSinceReset,
	}
	tp.removedOutputsSinceReset = 0
	return
}

func (tp *SequencerTipPool) numOutputs() int {
	tp.mutex.RLock()
	defer tp.mutex.RUnlock()

	return len(tp.outputs)
}
