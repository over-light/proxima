package attacher

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"time"

	"github.com/lunfardo314/proxima/core/memdag"
	"github.com/lunfardo314/proxima/core/txmetadata"
	"github.com/lunfardo314/proxima/core/vertex"
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/checkpoints"
)

const (
	TraceTagAttachMilestone = "milestone"
	// lazyRepeatEach polling time. TODO Probably should be adaptable rather than a constant
	lazyRepeatEach = 50 * time.Millisecond
)

func runMilestoneAttacher(
	vid *vertex.WrappedTx,
	metadata *txmetadata.TransactionMetadata,
	callback func(vid *vertex.WrappedTx, err error),
	env Environment,
	ctx context.Context,
) {
	a := newMilestoneAttacher(vid, env, metadata, ctx)
	var err error

	defer func() {
		go func() {
			//a.IncCounter("close")
			a.close()
			//a.DecCounter("close")
		}()
		// it is guaranteed callback will always be called, if any
		if callback != nil {
			callback(vid, err)
		}
	}()

	if err = a.run(); err != nil {
		vid.SetTxStatusBad(err)
		if !errors.Is(err, ErrSolidificationDeadline) {
			// solidification errors with big attachment depth are too verbose
			env.Log().Warnf(a.logErrorStatusString(err))
		}
	} else {
		msData := env.ParseMilestoneData(vid)
		if vid.IsBranchTransaction() {
			env.Infof0(a.logFinalStatusString(msData))
			env.EvidenceBranchInflationBonus(vid.InflationAmount())
		} else {
			env.Infof1(a.logFinalStatusString(msData))
		}
	}
	// finished either way: good or bad
	vid.SetSequencerAttachmentFinished()

	env.PokeAllWith(vid)
}

func newMilestoneAttacher(vid *vertex.WrappedTx, env Environment, metadata *txmetadata.TransactionMetadata, providedCtx context.Context) *milestoneAttacher {
	env.Assertf(vid.IsSequencerMilestone(), "newMilestoneAttacher: %s is not a sequencer milestone", vid.IDShortString)

	ret := &milestoneAttacher{
		attacher:         newPastConeAttacher(env, vid, base.LedgerTime{}, vid.IDShortString()),
		vid:              vid,
		providedMetadata: metadata,
		pokeChan:         make(chan struct{}),
		finals:           attachFinals{started: time.Now()},
		ctx:              providedCtx,
	}
	if ret.ctx == nil {
		ret.ctx = env.Ctx()
	}

	ret.attacher.pokeMe = func(vid *vertex.WrappedTx) {
		ret.pokeMe(vid)
	}
	ret.vid.OnPoke(func() {
		ret._doPoke()
	})
	vid.Unwrap(vertex.UnwrapOptions{
		Vertex: func(v *vertex.Vertex) {
			ret.finals.numInputs = v.Tx.NumInputs()
			ret.finals.numOutputs = v.Tx.NumProducedOutputs()
		},
		DetachedVertex: func(_ *vertex.DetachedVertex) {
			env.Log().Fatalf("unexpected detached Tx: %s", vid.IDShortString())
		},
		VirtualTx: func(_ *vertex.VirtualTransaction) {
			env.Log().Fatalf("unexpected virtual Tx: %s", vid.IDShortString())
		},
	})
	ret.pastCone.MustMarkVertexNotInTheState(vid)

	return ret
}

func (a *milestoneAttacher) run() error {
	// first determine the baseline state

	if status := a.solidifyBaseline(); status != vertex.Good {
		a.Tracef(TraceTagAttachMilestone, "baseline solidification failed. Reason: %v", a.err)
		util.AssertMustError(a.err)
		return a.err
	}

	a.Assertf(a.BaselineBranch() != nil, "a.baseline != nil")
	a.Tracef(TraceTagAttachMilestone, "baseline is OK <- %s", a.BaselineBranch().StringShort)

	// then solidify past cone

	a.Tracef(TraceTagAttachMilestone, "BEFORE solidifyPastCone %s")
	status := a.solidifyPastCone()
	a.Tracef(TraceTagAttachMilestone, "AFTER solidifyPastCone %s")

	a.Assertf(status != vertex.Undefined, "status!=vertex.Undefined")

	if status != vertex.Good {
		a.Tracef(TraceTagAttachMilestone, "past cone solidification failed. Reason: %v", a.err)
		a.Assertf(a.err != nil, "a.err!=nil")
		return a.err
	}

	a.Tracef(TraceTagAttachMilestone, "past cone OK")
	a.AssertNoError(a.err)

	err := a.checkConsistencyBeforeWrapUp()
	if err != nil {
		memdag.SaveGraphPastCone(a.vid, "inconsistent_before_wrapup")
	}
	a.AssertNoError(err)

	// finalizing touches
	a.wrapUpAttacher()

	a.pastCone.SetFlagsUp(a.vid, vertex.FlagPastConeVertexDefined)
	if a.vid.IsBranchTransaction() {
		// branch transaction vertex is immediately detached. Thus branch transaction does not reference the past cone
		a.vid.ConvertToDetached()
		//a.vid.SetTxStatusGood(nil, a.pastCone.LedgerCoverage())
		a.vid.SetTxStatusGood(a.pastCone.PastConeBase.CloneImmutable(), a.FinalLedgerCoverage(a.vid.Timestamp()))
	} else {
		a.vid.SetTxStatusGood(a.pastCone.PastConeBase.CloneImmutable(), a.FinalLedgerCoverage(a.vid.Timestamp()))
		a.EvidencePastConeSize(a.pastCone.PastConeBase.Len())
	}

	{ // debug
		const (
			lastCheck     = true
			printPastCone = false
		)
		if lastCheck {
			err = a.pastCone.CheckFinalPastCone(a.Branches().GetStateReaderForTheBranch)
			if err != nil {
				err = fmt.Errorf("%w\n------ past cone of %s ------\n%s",
					err, a.vid.IDShortString(), a.pastCone.Lines("     ").Join("\n"))
				memdag.SaveGraphPastCone(a.vid, "past_cone_CheckFinalPastCone")
			}
			a.AssertNoError(err)
		}
		if printPastCone {
			a.Log().Infof(">>>>>>>>>>>>> past cone of attacher %s\n%s", a.Name(), a.pastCone.Lines("      ").String())
		}
	}

	a.SendToTippool(a.vid)

	return nil
}

// deadlock catcher, if enabled, calls the callback function whenever lazyRepeat loop is stuck for more than
// set duration threshold. EnableDeadlockCatching(0) disables deadlock catching
// Default is enabled for 10 seconds

const deadlockThreshold = 10 * time.Second

// lazyRepeat repeats closure until it returns Good or Bad
func (a *milestoneAttacher) lazyRepeat(loopName string, fun func() vertex.Status) vertex.Status {

	// ===== deadlock catching ====
	var checkpoint *checkpoints.Checkpoints
	checkName := a.Name() + "_" + loopName
	if !a.DeadlockCatchingDisabled() {
		checkpoint = checkpoints.New(func(name string) {
			buf := make([]byte, 2*math.MaxUint16)
			runtime.Stack(buf, true)
			a.Log().Fatalf(">>>>>>>> DEADLOCK suspected in the loop '%s' (stuck for %v):\n%s",
				checkName, deadlockThreshold, string(buf))
		})
		defer checkpoint.Close()
	}
	// ===== deadlock catching ====

	for {
		// repeat until becomes defined or interrupted
		if status := fun(); status != vertex.Undefined {
			return status
		}
		select {
		case <-a.pokeChan:
		case <-a.ctx.Done():
			a.setError(fmt.Errorf("%w. Undefined past cone: %s", global.ErrInterrupted, a.pastCone.UndefinedListLines().Join(", ")))
			return vertex.Bad
		case <-time.After(lazyRepeatEach):
		}

		if !a.DeadlockCatchingDisabled() {
			checkpoint.Check(checkName, deadlockThreshold)
		}
	}
}

func (a *milestoneAttacher) close() {
	a.closeOnce.Do(func() {
		a.pastCone.Dispose()
		a.pastCone = nil

		a.pokeClosingMutex.Lock()
		defer a.pokeClosingMutex.Unlock()

		a.closed = true
		close(a.pokeChan)
		a.vid.OnPokeNop()
		a.attacher.pokeMe = func(_ *vertex.WrappedTx) {}
	})
}

func (a *milestoneAttacher) solidifyBaseline() vertex.Status {
	return a.lazyRepeat("baseline solidification", func() vertex.Status {
		ok := false
		finalSuccess := false
		util.Assertf(a.vid.FlagsUp(vertex.FlagVertexTxAttachmentStarted), "AttachmentStarted flag must be up")
		util.Assertf(!a.vid.FlagsUp(vertex.FlagVertexTxAttachmentFinished), "AttachmentFinished flag must be down")

		a.vid.Unwrap(vertex.UnwrapOptions{
			Vertex: func(v *vertex.Vertex) {
				a.Assertf(a.vid.GetTxStatusNoLock() == vertex.Undefined, "a.vid.GetTxStatusNoLock() == vertex.Undefined:\n%s", a.vid.StringNoLock)
				a.Assertf(a.BaselineBranch() == nil, "a.baseline == nil")

				ok = a.solidifyBaselineUnwrapped(v, a.vid)
				if ok && v.BaselineBranchID != nil {
					a.setBaseline(v.BaselineBranchID)
					finalSuccess = true
				}
			},
			DetachedVertex: func(v *vertex.DetachedVertex) {
				// reattach
				AttachTransaction(v.Tx, a, WithInvokedBy(a.name+"_reattach"))
				//a.Log().Fatalf("solidifyBaseline: unexpected detached tx %s", a.vid.StringNoLock())
			},
			VirtualTx: func(_ *vertex.VirtualTransaction) {
				a.Log().Fatalf("solidifyBaseline: unexpected virtual tx %s", a.vid.StringNoLock())
			},
		})

		switch {
		case !ok:
			return vertex.Bad
		case finalSuccess:
			return vertex.Good
		default:
			return vertex.Undefined
		}
	})
}

// solidifyPastCone solidifies and validates sequencer transaction in the context of the known baseline state
func (a *milestoneAttacher) solidifyPastCone() vertex.Status {
	return a.lazyRepeat("past cone solidification", func() (status vertex.Status) {
		ok := false
		finalSuccess := false
		a.vid.Unwrap(vertex.UnwrapOptions{
			Vertex: func(v *vertex.Vertex) {
				a.Assertf(a.vid.GetTxStatusNoLock() == vertex.Undefined, "a.vid.GetTxStatusNoLock() == vertex.Undefined")

				if ok = a.attachVertexUnwrapped(v, a.vid); !ok {
					a.Assertf(a.err != nil, "a.err != nil")
					return
				}

				if ok, finalSuccess = a.validateSequencerTxUnwrapped(v); !ok {
					a.Assertf(a.err != nil, "a.err != nil")
					return
				}
				a.Tracef(TraceTagAttachVertex, "NOT final..")

				const doubleCheck = true
				if doubleCheck && finalSuccess {
					// double check
					conflict := a.CheckConflicts()
					a.Assertf(conflict == nil, "unexpected conflict %s in %s", conflict.IDStringShort(), a.name)
				}
			},
			DetachedVertex: func(v *vertex.DetachedVertex) {
				a.Log().Fatalf("solidifyPastCone: unexpected detached tx %s", a.vid.StringNoLock())
			},
			VirtualTx: func(_ *vertex.VirtualTransaction) {
				a.Log().Fatalf("solidifyPastCone: unexpected virtual tx %s", a.vid.StringNoLock())
			},
		})
		switch {
		case !ok:
			a.Assertf(a.err != nil, "a.err!=nil")
			return vertex.Bad

		case finalSuccess:
			util.Assertf(!a.pastCone.ContainsUndefined(),
				"inconsistency: attacher %s is 'finalSuccess' but still contains undefined Vertices. LinesVerbose:\n%s",
				a.name, a.dumpLinesString)
			return vertex.Good

		default:
			return vertex.Undefined
		}
	})
}

const TraceTagValidateSequencer = "validateSeq"

func (a *milestoneAttacher) validateSequencerTxUnwrapped(v *vertex.Vertex) (ok, finalSuccess bool) {
	if a.pastCone.ContainsUndefined() {
		a.Tracef(TraceTagValidateSequencer, "contains undefined in the past cone:\n%s", a.pastCone.Lines("     ").Join("\n"))
		return true, false
	}
	flags := a.pastCone.Flags(a.vid)
	if !flags.FlagsUp(vertex.FlagPastConeVertexEndorsementsSolid) || !flags.FlagsUp(vertex.FlagPastConeVertexInputsSolid) {
		return true, false
	}
	// inputs solid
	glbFlags := a.vid.FlagsNoLock()
	a.Assertf(!glbFlags.FlagsUp(vertex.FlagVertexConstraintsValid), "%s: !glbFlags.FlagsUp(vertex.FlagConstraintsValid) in %s", a.name, a.vid.IDShortString)

	if err := a.validateVertex(v); err != nil {
		a.setError(err)
		v.UnReferenceDependencies()
		a.Tracef(TraceTagValidateSequencer, "constraint validation failed in %s: '%v'", a.vid.IDShortString, err)
		return false, false
	}
	a.vid.SetFlagsUpNoLock(vertex.FlagVertexConstraintsValid)
	a.Tracef(TraceTagValidateSequencer, "constraints has been validated OK: %s", v.Tx.IDShortString)

	if conflict := a.pastCone.CheckAndClean(a.Branches().GetStateReaderForTheBranch); conflict != nil {
		a.setError(fmt.Errorf("conflict %s in the past cone:\n%s", conflict.IDStringShort(), a.pastCone.Lines("    ").String()))
		v.UnReferenceDependencies()
		return false, false
	}
	//a.Assertf(nil == a.pastCone.CheckConflicts(a.Branches().GetStateReaderForTheBranch), ">>>> inconsistency <<<<<<")
	return true, true
}

func (a *milestoneAttacher) _doPoke() {
	a.pokeClosingMutex.RLock()
	defer a.pokeClosingMutex.RUnlock()

	// must be non-blocking, otherwise deadlocks when syncing or high TPS
	if !a.closed {
		select {
		case a.pokeChan <- struct{}{}:
		default:
			// poke is lost when blocked, but that is ok because there's pull from the attacher's side
		}
	}
}

func (a *milestoneAttacher) pokeMe(with *vertex.WrappedTx) {
	flags := a.pastCone.Flags(with)
	util.Assertf(a.pastCone.IsKnown(with), "must be marked known %s", with.IDShortString)
	if !flags.FlagsUp(vertex.FlagPastConeVertexAskedForPoke) {
		a.Tracef(TraceTagAttachMilestone, "pokeMe with %s", with.IDShortString)
		a.PokeMe(a.vid, with)
		a.pastCone.SetFlagsUp(with, vertex.FlagPastConeVertexAskedForPoke)
	}
}

func (a *milestoneAttacher) logFinalStatusString(msData *ledger.MilestoneData) string {
	var msg string

	msDataStr := " (n/a)"
	if msData != nil {
		msDataStr = fmt.Sprintf(" %s %d/%d", msData.Name, msData.BranchHeight, msData.ChainHeight)
	}

	if a.vid.IsBranchTransaction() {
		msg = fmt.Sprintf("--- BRANCH%s %s(in %d, tx: %d), i = %s",
			msDataStr, a.vid.IDShortString(), a.finals.numInputs, a.finals.MutationStats.NumTransactions,
			util.Th(a.vid.InflationAmount()))
	} else {
		msg = fmt.Sprintf("--- SEQ TX%s %s(in %d), i = %s, lnow: %s",
			msDataStr, a.vid.IDShortString(), a.finals.numInputs, util.Th(a.vid.InflationAmount()), ledger.TimeNow().String())
	}
	if a.vid.GetTxStatus() == vertex.Bad {
		msg += fmt.Sprintf("BAD: err = '%v'", a.vid.GetError())
	} else {
		msg += fmt.Sprintf(", base: %s, cov/delta: %s/%s", a.finals.baseline.StringShort(),
			util.Th(*a.finals.TransactionMetadata.LedgerCoverage), util.Th(*a.finals.TransactionMetadata.CoverageDelta))
		if a.VerbosityLevel() > 0 {
			if a.vid.IsBranchTransaction() {
				msg += fmt.Sprintf(", slot inflation: %s, supply: %s", util.Th(*a.finals.TransactionMetadata.SlotInflation), util.Th(*a.finals.TransactionMetadata.Supply))
			} else {
				msg += fmt.Sprintf(", slot inflation: %s", util.Th(*a.finals.TransactionMetadata.SlotInflation))
			}
		}
	}
	return msg
}

func (a *milestoneAttacher) logErrorStatusString(err error) string {
	blStr := "baseline: N/A"
	if bl := a.BaselineBranch(); bl != nil {
		blStr = fmt.Sprintf("baseline: %s (hex = %s)", bl.StringShort(), bl.StringHex())
	}
	return fmt.Sprintf("ATTACH %s (%s) -> BAD(%v)", a.vid.IDShortString(), blStr, err)
}
