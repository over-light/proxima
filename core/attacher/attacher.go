package attacher

import (
	"errors"
	"fmt"

	"github.com/lunfardo314/proxima/core/vertex"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/lazyargs"
	"github.com/lunfardo314/proxima/util/lines"
	"github.com/lunfardo314/unitrie/common"
)

func newPastConeAttacher(env Environment, tip *vertex.WrappedTx, targetTs base.LedgerTime, name string) attacher {
	ret := attacher{
		Environment: env,
		name:        name,
		pokeMe:      func(_ *vertex.WrappedTx) {},
		pastCone:    vertex.NewPastCone(env, tip, targetTs, name),
	}
	return ret
}

const (
	TraceTagAttach       = "attach"
	TraceTagAttachVertex = "attachVertex"
)

func (a *attacher) Name() string {
	return a.name
}

func (a *attacher) BaselineSugaredStateReader() multistate.SugaredStateReader {
	return multistate.MakeSugared(a.baselineStateReader())
}

func (a *attacher) baselineStateReader() multistate.IndexedStateReader {
	return a.GetStateReaderForTheBranch(a.baseline.ID())
}

func (a *attacher) setError(err error) {
	a.err = err
}

const TraceTagSolidifySequencerBaseline = "seqBase"

// solidifySequencerBaseline directs the attachment process down the MemDAG to reach the deterministically known baseline state
// for a sequencer milestone. Existence of it is guaranteed by the ledger constraints
// Success of the baseline solidification is when the function returns true and v.BaselineBranch != nil
// Special edge case: when the baseline branch is before the snapshot state, it has to be taken into account if
// it can be used as a baseline or not
func (a *attacher) solidifyBaselineUnwrapped(v *vertex.Vertex, vidUnwrapped *vertex.WrappedTx) (ok bool) {
	a.Tracef(TraceTagSolidifySequencerBaseline, "IN for %s", v.Tx.IDShortString)
	defer a.Tracef(TraceTagSolidifySequencerBaseline, "OUT for %s", v.Tx.IDShortString)

	// determine the baseline
	baselineDirectionID := v.Tx.BaselineDirection()
	util.Assertf(baselineDirectionID != base.TransactionID{}, "baselineDirectionID!=base.TransactionID()")

	baselineDirection := AttachTxID(baselineDirectionID, a,
		WithInvokedBy(a.name),
		WithAttachmentDepth(vidUnwrapped.GetAttachmentDepthNoLock()+1),
	)
	a.pastCone.MarkVertexKnown(baselineDirection)

	switch baselineDirection.GetTxStatus() {
	case vertex.Good:
		// in case the baseline is already detached, we provide a reattach function for the branch
		baseline := baselineDirection.BaselineBranch(func(txid base.TransactionID) *vertex.WrappedTx {
			return AttachTxID(txid, a, WithInvokedBy(a.name+"_reattach"))
		})
		a.Assertf(baseline != nil, "baseline is nil in %s. Baseline direction:\n%s",
			a.name, func() string { return baselineDirection.Lines("    ").String() })

		v.BaselineBranch = baseline
		return true

	case vertex.Bad:
		a.setError(baselineDirection.GetError())
		return false

	case vertex.Undefined:
		return a.pullIfNeeded(baselineDirection, "solidifyBaselineUnwrapped")
	}
	panic("wrong vertex state")
}

// attachVertexNonBranch if vertex undefined, recursively attaches past cone
// Does not check for past cone consistency -> resulting past cone may contain double spends util attacher solidifies all of it
func (a *attacher) attachVertexNonBranch(vid *vertex.WrappedTx) (ok bool) {
	a.Assertf(!vid.IsBranchTransaction(), "!vid.IsBranchTransaction(): %s", vid.IDShortString)

	if a.pastCone.IsKnownDefined(vid) {
		return true
	}
	var deterministicPastCone *vertex.PastConeBase

	defined := false
	vid.Unwrap(vertex.UnwrapOptions{
		Vertex: func(v *vertex.Vertex) {
			switch vid.GetTxStatusNoLock() {
			case vertex.Undefined:
				if vid.IsSequencerMilestone() {
					// don't go deeper for undefined sequencers
					ok = true
					return
				}
				// non-sequencer transaction
				ok = a.attachVertexUnwrapped(v, vid)
				if ok && vid.FlagsUpNoLock(vertex.FlagVertexConstraintsValid) && a.pastCone.Flags(vid).FlagsUp(vertex.FlagPastConeVertexInputsSolid|vertex.FlagPastConeVertexEndorsementsSolid) {
					a.pastCone.SetFlagsUp(vid, vertex.FlagPastConeVertexDefined)
					defined = true
				}
			case vertex.Good:
				a.Assertf(vid.IsSequencerMilestone(), "vid.IsSequencerTransaction()")
				if !a.branchesCompatible(a.baseline, v.BaselineBranch) {
					a.setError(fmt.Errorf("conflicting baseline of %s", vid.IDShortString()))
					return
				}
				ok = true
				// here cut the recursion and merge 'good' past cone
				deterministicPastCone = vid.GetPastConeNoLock()
				a.Assertf(deterministicPastCone != nil, "deterministicPastCone!=nil")
			case vertex.Bad:
				a.setError(vid.GetErrorNoLock())

			default:
				a.Log().Fatalf("inconsistency: wrong tx status")
			}
		},
		DetachedVertex: func(v *vertex.DetachedVertex) {
			AttachTransaction(v.Tx, a,
				WithInvokedBy(a.name),
				WithAttachmentDepth(vid.GetAttachmentDepthNoLock()+1),
			)
			ok = true
		},
		VirtualTx: func(_ *vertex.VirtualTransaction) {
			ok = true
		},
	})
	if !ok {
		a.Assertf(a.err != nil, "a.err != nil: %s", vid.IDShortString())
		return
	}
	if deterministicPastCone != nil {
		a.pastCone.AppendPastCone(deterministicPastCone, a.baselineStateReader())
		ok = true
		defined = true
	}

	if defined {
		a.pastCone.SetFlagsUp(vid, vertex.FlagPastConeVertexDefined)
	} else {
		a.pokeMe(vid)
	}
	return
}

// attachVertexUnwrapped: vid corresponds to the vertex v
// it solidifies vertex by traversing the past cone down to rooted outputs or undefined Vertices
// Repetitive calling of the function reaches all past vertices down to the rooted outputs
// The exit condition of the loop is fully determined states of the past cone.
// It results in all Vertices are vertex.Good
// Otherwise, repetition reaches vertex.Bad vertex and exits
// Returns OK (== not bad)
func (a *attacher) attachVertexUnwrapped(v *vertex.Vertex, vidUnwrapped *vertex.WrappedTx) (ok bool) {
	a.Assertf(!v.Tx.IsSequencerTransaction() || a.baseline != nil, "!v.Tx.IsSequencerTransaction() || a.baseline != nil in %s", v.Tx.IDShortString)

	if vidUnwrapped.GetTxStatusNoLock() == vertex.Bad {
		a.setError(vidUnwrapped.GetErrorNoLock())
		a.Assertf(a.err != nil, "a.err != nil")
		return false
	}

	a.Tracef(TraceTagAttachVertex, " %s IN: %s", a.name, vidUnwrapped.IDShortString)
	a.Assertf(!util.IsNil(a.BaselineSugaredStateReader), "!util.IsNil(a.BaselineSugaredStateReader)")

	if !a.pastCone.Flags(vidUnwrapped).FlagsUp(vertex.FlagPastConeVertexEndorsementsSolid) {
		a.Tracef(TraceTagAttachVertex, "endorsements not all solidified in %s -> attachEndorsements", v.Tx.IDShortString)
		// depth-first along endorsements
		if !a.attachEndorsements(v, vidUnwrapped) { // <<< recursive
			// not ok -> leave attacher
			a.Assertf(a.err != nil, "a.err != nil")
			return false
		}
	}
	// check consistency
	if a.pastCone.Flags(vidUnwrapped).FlagsUp(vertex.FlagPastConeVertexEndorsementsSolid) {
		a.Assertf(a.allEndorsementsDefined(v), "not all endorsements defined:\n%s", func() string { return a.pastCone.Lines("       ").String() })

		a.Tracef(TraceTagAttachVertex, "endorsements are all solid in %s", v.Tx.IDShortString)
	} else {
		a.Tracef(TraceTagAttachVertex, "endorsements NOT marked solid in %s", v.Tx.IDShortString)
	}

	if !a.pastCone.Flags(vidUnwrapped).FlagsUp(vertex.FlagPastConeVertexInputsSolid) {
		if !a.attachInputs(v, vidUnwrapped) {
			a.Assertf(a.err != nil, "a.err!=nil")
			return false
		}
	}

	if a.pastCone.Flags(vidUnwrapped).FlagsUp(vertex.FlagPastConeVertexInputsSolid) {
		a.Assertf(a.allInputsDefined(v), "a.allInputsDefined(v)")

		if !v.Tx.IsSequencerTransaction() {
			if !a.finalTouchNonSequencer(v, vidUnwrapped) {
				a.Assertf(a.err != nil, "a.err!=nil")
				return false
			}
		}
	} else {
		a.Tracef(TraceTagAttachVertex, "attachVertexUnwrapped(%s) not all inputs solid", v.Tx.IDShortString)
	}

	a.Tracef(TraceTagAttachVertex, "attachVertexUnwrapped(%s) return OK", v.Tx.IDShortString)
	return true
}

// finalTouchNonSequencer finishes validation of non-sequencer transactions
func (a *attacher) finalTouchNonSequencer(v *vertex.Vertex, vid *vertex.WrappedTx) (ok bool) {
	a.Assertf(!vid.IsSequencerMilestone(), "non-sequencer tx expected, got %s", vid.IDShortString)

	glbFlags := vid.FlagsNoLock()
	if !glbFlags.FlagsUp(vertex.FlagVertexConstraintsValid) {
		// in either case, for non-sequencer transaction validation makes attachment
		// finished and transaction ready to be pruned from the memDAG
		vid.SetFlagsUpNoLock(vertex.FlagVertexTxAttachmentFinished)

		//{ // debug
		//	a.Log().Infof(">>>>>>> finalTouchNonSequencer:\n%s", v.Lines("     ").String())
		//}

		// constraints are not validated yet
		if err := v.ValidateConstraints(); err != nil {
			v.UnReferenceDependencies()
			a.setError(err)
			a.Tracef(TraceTagAttachVertex, "constraint validation failed in %s: '%v'", vid.IDShortString(), err)
			return false
		}
		// mark transaction validated
		vid.SetFlagsUpNoLock(vertex.FlagVertexConstraintsValid)

		a.Tracef(TraceTagAttachVertex, "constraints has been validated OK: %s", v.Tx.IDShortString)
		a.PokeAllWith(vid)
	}
	glbFlags = vid.FlagsNoLock()
	a.Assertf(glbFlags.FlagsUp(vertex.FlagVertexConstraintsValid), "glbFlags.FlagsUp(vertex.FlagConstraintsValid)")

	// non-sequencer, all inputs solid, constraints valid -> we can mark it 'defined' in the attacher
	a.pastCone.SetFlagsUp(vid, vertex.FlagPastConeVertexDefined)
	return true
}

// refreshDependencyStatus ensures it is known in the past cone, checks in the state status, pulls if needed
func (a *attacher) refreshDependencyStatus(vidDep *vertex.WrappedTx) (ok bool) {
	if vidDep.GetTxStatus() == vertex.Bad {
		a.setError(vidDep.GetError())
		return false
	}

	a.pastCone.MarkVertexKnown(vidDep)
	a.defineInTheStateStatus(vidDep)
	if !a.pullIfNeeded(vidDep, "refreshDependencyStatus") {
		return false
	}
	return true
}

// defineInTheStateStatus checks if dependency is in the baseline state and marks it correspondingly, if possible
func (a *attacher) defineInTheStateStatus(vid *vertex.WrappedTx) {
	a.Assertf(a.pastCone.IsKnown(vid), "a.pastCone.IsKnown(vid): %s", vid.IDShortString)
	a.Assertf(a.baseline != nil, "a.baseline != nil")

	if a.pastCone.Flags(vid).FlagsUp(vertex.FlagPastConeVertexCheckedInTheState) {
		return
	}

	if a.BaselineSugaredStateReader().KnowsCommittedTransaction(vid.ID()) {
		a.pastCone.SetFlagsUp(vid, vertex.FlagPastConeVertexCheckedInTheState|vertex.FlagPastConeVertexInTheState|vertex.FlagPastConeVertexDefined)
	} else {
		// not on the state, so it is not defined
		a.pastCone.SetFlagsUp(vid, vertex.FlagPastConeVertexCheckedInTheState)
	}
}

func (a *attacher) attachEndorsements(v *vertex.Vertex, vid *vertex.WrappedTx) (ok bool) {
	if a.pastCone.Flags(vid).FlagsUp(vertex.FlagPastConeVertexEndorsementsSolid) {
		return true
	}
	for i := range v.Endorsements {
		if !a.attachEndorsement(v, vid, byte(i)) {
			return false
		}
	}

	if a.allEndorsementsDefined(v) {
		a.pastCone.SetFlagsUp(vid, vertex.FlagPastConeVertexEndorsementsSolid)
	}
	return true
}

func (a *attacher) attachEndorsement(v *vertex.Vertex, vidUnwrapped *vertex.WrappedTx, index byte) bool {
	vidEndorsed := v.Endorsements[index]
	if vidEndorsed == nil {
		vidEndorsed = AttachTxID(v.Tx.EndorsementAt(index), a,
			WithInvokedBy(a.name),
			WithAttachmentDepth(vidUnwrapped.GetAttachmentDepthNoLock()+1),
		)
		v.ReferenceEndorsement(index, vidEndorsed)
	}
	a.Assertf(vidEndorsed != nil, "vidEndorsed!=nil")

	return a.attachEndorsementDependency(vidEndorsed)
}

func (a *attacher) attachEndorsementDependency(vidEndorsed *vertex.WrappedTx) bool {
	if !a.refreshDependencyStatus(vidEndorsed) {
		return false
	}
	if vidEndorsed.IsBranchTransaction() {
		if vidEndorsed != a.baseline {
			a.setError(fmt.Errorf("conflicting branch endorsement %s", vidEndorsed.IDShortString()))
			return false
		}
		a.Assertf(a.pastCone.IsKnownDefined(vidEndorsed), "expected to be 'defined': %s", vidEndorsed.IDShortString())
		return true
	}
	return a.attachVertexNonBranch(vidEndorsed)
}

func (a *attacher) attachInput(v *vertex.Vertex, vidUnwrapped *vertex.WrappedTx, inputIdx byte) bool {
	oid := v.Tx.MustInputAt(inputIdx)
	vidDep := v.Inputs[inputIdx]

	var ok bool
	if vidDep == nil {
		vidDep = AttachTxID(oid.TransactionID(), a,
			WithInvokedBy(a.name),
			WithAttachmentDepth(vidUnwrapped.GetAttachmentDepthNoLock()+1),
		)
		v.ReferenceInput(inputIdx, vidDep)
	}
	a.Assertf(vidDep != nil, "vidDep!=nil")

	if !a.refreshDependencyStatus(vidDep) {
		return false
	}
	vidDep.AddConsumer(oid.Index(), vidUnwrapped)

	wOut := vertex.WrappedOutput{
		VID:   vidDep,
		Index: oid.Index(),
	}
	a.Tracef(TraceTagBranchAvailable, "before attachOutput(%s): %s", wOut.IDStringShort, a.pastCone.Flags(vidDep).String())
	ok = a.attachOutput(wOut)
	if !ok {
		return false
	}
	a.Tracef(TraceTagBranchAvailable, "after attachOutput(%s): %s", wOut.IDStringShort, a.pastCone.Flags(vidDep).String())
	return true
}

func (a *attacher) attachInputs(v *vertex.Vertex, vidUnwrapped *vertex.WrappedTx) (ok bool) {
	for i := range v.Inputs {
		if !a.attachInput(v, vidUnwrapped, byte(i)) {
			a.Assertf(a.err != nil, "a.err!=nil in %s, idx %d", a.name, i)
			return false
		}
	}
	if a.allInputsDefined(v) {
		a.pastCone.SetFlagsUp(vidUnwrapped, vertex.FlagPastConeVertexInputsSolid)
	}
	return true
}

func (a *attacher) allInputsDefined(v *vertex.Vertex) bool {
	for _, vidInp := range v.Inputs {
		if vidInp == nil {
			return false
		}
		if !a.pastCone.IsKnownDefined(vidInp) {
			return false
		}
	}
	return true
}

// checkOutputInTheState expects the produced UTXO ID of the transaction is in the state.
// If it is not, sets an error that UTXO is already consumed
func (a *attacher) checkOutputInTheState(vid *vertex.WrappedTx, inputID base.OutputID) bool {
	a.Assertf(a.pastCone.IsInTheState(vid), "a.pastCone.IsInTheState(wOut.VID)")
	o, err := a.BaselineSugaredStateReader().GetOutputWithID(inputID)
	if errors.Is(err, multistate.ErrNotFound) {
		a.setError(fmt.Errorf("output %s is already consumed", inputID.StringShort()))
		return false
	}
	a.AssertNoError(err)
	vid.MustEnsureOutput(o.Output, o.ID.Index())
	return true
}

func (a *attacher) attachOutput(wOut vertex.WrappedOutput) bool {
	if !wOut.ValidID() {
		return false
	}
	a.Assertf(a.pastCone.IsKnown(wOut.VID), "a.pastCone.IsKnown(wOut.VID)")

	if a.pastCone.IsInTheState(wOut.VID) {
		// transaction is marked 'in the state, i.e. rooted'
		if !a.checkOutputInTheState(wOut.VID, wOut.DecodeID()) {
			// output is not in the state, it means it is consumed
			return false
		}
	}
	// output is available in the baseline state
	if a.pastCone.Flags(wOut.VID).FlagsUp(vertex.FlagPastConeVertexDefined) {
		return true
	}
	// not marked yet as defined
	if wOut.VID.IsBranchTransaction() {
		// if it is on the branch tx, it must be marked as defined
		a.pastCone.SetFlagsUp(wOut.VID, vertex.FlagPastConeVertexDefined)
		return true
	}
	// not defined, not branch, not in the state or unknown
	return a.attachVertexNonBranch(wOut.VID)
}

func (a *attacher) branchesCompatible(vidBranch1, vidBranch2 *vertex.WrappedTx) bool {
	a.Assertf(vidBranch1.IsBranchTransaction() && vidBranch1.IsBranchTransaction(), "vidBranch1.IsBranchTransaction() && vidBranch1.IsBranchTransaction()")
	switch {
	case vidBranch1 == vidBranch2:
		return true
	case vidBranch1.Slot() == vidBranch2.Slot():
		// two different branches on the same slot conflicts
		return false
	case vidBranch1.Slot() < vidBranch2.Slot():
		return multistate.BranchKnowsTransaction(vidBranch2.ID(), vidBranch1.ID(), func() common.KVReader { return a.StateStore() })
	default:
		return multistate.BranchKnowsTransaction(vidBranch1.ID(), vidBranch2.ID(), func() common.KVReader { return a.StateStore() })
	}
}

// setBaseline sets baseline, references it from the attacher
// For sequencer transaction baseline will be on the same slot, for branch transactions it can be further in the past
func (a *attacher) setBaseline(baselineVID *vertex.WrappedTx) bool {
	a.Assertf(baselineVID.IsBranchTransaction(), "setBaseline: baselineVID.IsBranchTransaction()")

	a.pastCone.SetBaseline(baselineVID)
	a.Tracef(TraceTagSolidifySequencerBaseline, "setBaseline %s", baselineVID.IDShortString)

	// FIXME baseline may not be in the state due to snapshot

	rr, found := multistate.FetchRootRecord(a.StateStore(), baselineVID.ID())
	if !found {
		// it can happen when the root record is pruned
		a.Tracef(TraceTagSolidifySequencerBaseline, "setBaseline can't fetch root record for %s", baselineVID.IDShortString)
		return false
	}

	a.baseline = baselineVID
	a.baselineSupply = rr.Supply
	return true
}

// dumpLines beware deadlocks
func (a *attacher) dumpLines(prefix ...string) *lines.Lines {
	ret := lines.New(prefix...)
	ret.Add("attacher %s", a.name).
		Add("   baseline: %s", a.baseline.IDShortString()).
		Add("   baselineSupply: %s", util.Th(a.baselineSupply)).
		Add("   Past cone:").
		Append(a.pastCone.Lines(prefix...))
	return ret
}

func (a *attacher) dumpLinesString(prefix ...string) string {
	return a.dumpLines(prefix...).String()
}

func (a *attacher) allEndorsementsDefined(v *vertex.Vertex) bool {
	for _, vid := range v.Endorsements {
		if vid == nil {
			return false
		}
		if !a.pastCone.IsKnownDefined(vid) {
			return false
		}
	}
	return true
}

func (a *attacher) SetTraceAttacher(name string) {
	a.forceTrace = name
}

func (a *attacher) Tracef(traceLabel string, format string, args ...any) {
	if a.forceTrace != "" {
		lazyArgs := fmt.Sprintf(format, lazyargs.Eval(args...)...)
		a.Log().Infof("%s LOCAL TRACE(%s//%s) %s", a.name, traceLabel, a.forceTrace, lazyArgs)
		return
	}
	a.Environment.Tracef(traceLabel, a.name+format+" ", args...)
}

func (a *attacher) SlotInflation() uint64 {
	return a.slotInflation
}

func (a *attacher) FinalSupply() uint64 {
	return a.baselineSupply + a.slotInflation
}

func (a *attacher) LedgerCoverage() uint64 {
	return a.pastCone.LedgerCoverage()
}

func (a *attacher) CoverageAndDelta() (uint64, uint64) {
	return a.pastCone.CoverageAndDelta()
}
