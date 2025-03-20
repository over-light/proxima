package attacher

import (
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/lunfardo314/proxima/core/vertex"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/transaction"
	"github.com/lunfardo314/proxima/ledger/txbuilder"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/trackgc"
)

const TraceTagIncrementalAttacher = "incAttach"

var trackedIncrementalAttachers = trackgc.New[IncrementalAttacher](func(p *IncrementalAttacher) string {
	return "incAttacher " + p.name
})

func NewIncrementalAttacher(name string, env Environment, targetTs ledger.Time, extend vertex.WrappedOutput, endorse ...*vertex.WrappedTx) (*IncrementalAttacher, error) {
	env.Assertf(ledger.ValidSequencerPace(extend.Timestamp(), targetTs), "NewIncrementalAttacher: target is closer than allowed pace (%d): %s -> %s",
		ledger.TransactionPaceSequencer(), extend.Timestamp().String, targetTs.String)

	for _, endorseVID := range endorse {
		env.Assertf(endorseVID.IsSequencerMilestone(), "NewIncrementalAttacher: endorseVID.IsSequencerMilestone()")
		env.Assertf(targetTs.Slot() == endorseVID.Slot(), "NewIncrementalAttacher: targetTs.Slot() == endorseVid.Slot()")
		env.Assertf(ledger.ValidTransactionPace(endorseVID.Timestamp(), targetTs), "NewIncrementalAttacher: ledger.ValidTransactionPace(endorseVID.Timestamp(), targetTs)")
	}
	env.Tracef(TraceTagIncrementalAttacher, "NewIncrementalAttacher(%s). extend: %s, endorse: {%s}",
		name, extend.IDStringShort, func() string { return vertex.VerticesLines(endorse).Join(",") })

	var baselineDirection *vertex.WrappedTx
	if targetTs.Tick() == 0 {
		// target is branch
		env.Assertf(len(endorse) == 0, "NewIncrementalAttacher: len(endorse)==0")
		if !extend.VID.IsSequencerMilestone() {
			return nil, fmt.Errorf("NewIncrementalAttacher %s: cannot extend non-sequencer transaction %s into a branch",
				name, extend.VID)
		}
		baselineDirection = extend.VID
	} else {
		// target is not branch
		if extend.Slot() != targetTs.Slot() {
			// cross-slot, must have endorsement
			if len(endorse) > 0 {
				baselineDirection = endorse[0]
			}
		} else {
			// same slot
			baselineDirection = extend.VID
		}
	}
	if baselineDirection == nil {
		return nil, fmt.Errorf("NewIncrementalAttacher %s: failed to determine baseline direction in %s",
			name, extend.IDStringShort())
	}
	baseline := baselineDirection.BaselineBranch()
	if baseline == nil {
		// may happen when baselineDirection is virtualTx
		return nil, fmt.Errorf("NewIncrementalAttacher %s: failed to determine valid baselineDirection branch of %s. baseline direction: %s",
			name, extend.IDStringShort(), baselineDirection.IDShortString())
	}

	ret := &IncrementalAttacher{
		attacher: newPastConeAttacher(env, nil, targetTs, name),
		endorse:  make([]*vertex.WrappedTx, 0),
		inputs:   make([]vertex.WrappedOutput, 0),
		targetTs: targetTs,
	}

	if err := ret.initIncrementalAttacher(baseline, targetTs, extend, endorse...); err != nil {
		ret.Close()
		return nil, err
	}
	if conflict := ret.Check(); conflict != nil {
		ret.Close()
		return nil, fmt.Errorf("NewIncrementalAttacher %s: failed to create incremental attacher extending  %s: double-spend (conflict) %s in the past cone",
			name, extend.IDStringShort(), conflict.IDStringShort())
	}
	trackedIncrementalAttachers.TrackPointerNotGCed(ret, "incAttacher "+name, 10*time.Second)
	return ret, nil
}

// Close releases all references of Vertices. Incremental attacher must be closed before disposing it,
// otherwise memDAG starts leaking Vertices. Repetitive closing has no effect
// TODO some kind of checking if it is closed after some time
func (a *IncrementalAttacher) Close() {
	if a != nil && !a.IsClosed() {
		a.pastCone = nil
		a.closed = true
	}
}

func (a *IncrementalAttacher) IsClosed() bool {
	return a.closed
}

func (a *IncrementalAttacher) initIncrementalAttacher(baseline *vertex.WrappedTx, targetTs ledger.Time, extend vertex.WrappedOutput, endorse ...*vertex.WrappedTx) error {
	if !a.setBaseline(baseline, targetTs) {
		return fmt.Errorf("NewIncrementalAttacher: failed to set baseline branch of %s", extend.IDStringShort())
	}
	a.Tracef(TraceTagIncrementalAttacher, "NewIncrementalAttacher(%s). baseline: %s",
		a.name, baseline.IDShortString)

	// attach endorsements
	for _, endorsement := range endorse {
		a.Tracef(TraceTagIncrementalAttacher, "NewIncrementalAttacher(%s). insertEndorsement: %s", a.name, endorsement.IDShortString)
		if err := a.insertEndorsement(endorsement); err != nil {
			return err
		}
	}

	// extend input will always be at index 0
	if err := a.insertVirtuallyConsumedOutput(extend); err != nil {
		return err
	}

	if targetTs.IsSlotBoundary() {
		// stem input, if any, will be at index 1
		// for branches, include stem input
		a.Tracef(TraceTagIncrementalAttacher, "NewIncrementalAttacher(%s). insertStemInput", a.name)
		a.stemOutput = a.GetStemWrappedOutput(baseline.ID())
		if a.stemOutput.VID == nil {
			return fmt.Errorf("NewIncrementalAttacher: stem output is not available for baseline %s", baseline.IDShortString())
		}
		if err := a.insertVirtuallyConsumedOutput(a.stemOutput); err != nil {
			return err
		}
	}
	return nil
}

func (a *IncrementalAttacher) BaselineBranch() *vertex.WrappedTx {
	return a.baseline
}

func (a *IncrementalAttacher) insertVirtuallyConsumedOutput(wOut vertex.WrappedOutput) error {
	a.Assertf(wOut.ValidID(), "wOut.ValidID()")

	if !a.refreshDependencyStatus(wOut.VID) {
		return a.err
	}
	if !a.attachOutput(wOut) {
		return a.err
	}
	if !a.pastCone.IsKnownDefined(wOut.VID) {
		return fmt.Errorf("output %s not solid yet", wOut.IDStringShort())
	}
	if conflict := a.pastCone.AddVirtuallyConsumedOutput(wOut, a.baselineStateReader()); conflict != nil {
		return fmt.Errorf("past cone contains double-spend %s", conflict.IDStringShort())
	}
	a.inputs = append(a.inputs, wOut)
	return nil
}

// InsertEndorsement preserves consistency in case of failure
func (a *IncrementalAttacher) InsertEndorsement(endorsement *vertex.WrappedTx) error {
	util.Assertf(!a.IsClosed(), "a.IsClosed()")
	if a.pastCone.IsKnown(endorsement) {
		return fmt.Errorf("endorsing makes no sense: %s is already in the past cone", endorsement.IDShortString())
	}

	a.pastCone.BeginDelta()
	if err := a.insertEndorsement(endorsement); err != nil {
		a.pastCone.RollbackDelta()
		a.setError(nil)
		return err
	}
	a.pastCone.CommitDelta()
	return nil
}

// insertEndorsement in case of error, attacher remains inconsistent
func (a *IncrementalAttacher) insertEndorsement(endorsement *vertex.WrappedTx) error {
	if !a.attachEndorsementDependency(endorsement) {
		return a.err
	}

	if conflict := a.Check(); conflict != nil {
		return fmt.Errorf("insertEndorsement: double-spend (conflict) %s in the past cone", conflict.IDStringShort())
	}
	a.endorse = append(a.endorse, endorsement)
	return nil
}

// InsertInput inserts tag along or delegation input.
// In case of failure return false and attacher state with vertex references remains consistent
func (a *IncrementalAttacher) InsertInput(wOut vertex.WrappedOutput) (bool, error) {
	util.Assertf(!a.IsClosed(), "a.IsClosed()")
	util.AssertNoError(a.err)

	// save state for possible rollback because in case of fail the side effect makes attacher inconsistent
	a.pastCone.BeginDelta()
	err := a.insertVirtuallyConsumedOutput(wOut)
	if err != nil {
		// it is either conflicting, or not solid yet
		// in either case rollback
		a.pastCone.RollbackDelta()
		err = fmt.Errorf("InsertInput: %w", err)
		a.setError(nil)
		return false, err
	}
	util.AssertNoError(a.err)

	a.pastCone.CommitDelta()
	return true, nil
}

// MakeSequencerTransaction creates sequencer transaction from the incremental attacher.
// Increments slotInflation by the amount inflated in the transaction
func (a *IncrementalAttacher) MakeSequencerTransaction(seqName string, privateKey ed25519.PrivateKey, cmdParser SequencerCommandParser) (*transaction.Transaction, error) {
	util.Assertf(!a.IsClosed(), "!a.IsDisposed()")

	tagAlongInputs := make([]*ledger.OutputWithID, 0, len(a.inputs))
	otherOutputs := make([]*ledger.Output, 0)
	delegationInputs := make([]*ledger.OutputWithChainID, 0)

	var chainIn *ledger.OutputWithID
	var stemIn *ledger.OutputWithID
	var err error

	// separate delegation and tag-along outputs
	for i, wOut := range a.inputs {
		if i == 0 {
			// main chain input expected at index 0
			chainIn = wOut.OutputWithID()
			util.Assertf(chainIn != nil, "chainIn!=nil")
			continue
		}

		o := wOut.OutputWithID()
		util.Assertf(o != nil, "o!=nil")

		switch o.Output.Lock().Name() {
		case ledger.DelegationLockName:
			delegationID, predIdx, ok := o.ExtractChainID()
			util.Assertf(ok, "must be delegation output")

			delegationInputs = append(delegationInputs, &ledger.OutputWithChainID{
				OutputWithID:               *o,
				ChainID:                    delegationID,
				PredecessorConstraintIndex: predIdx,
			})
		case ledger.ChainLockName:
			tagAlongInputs = append(tagAlongInputs, o)
			// parse sequencer command if any
			outputs, err := cmdParser.ParseSequencerCommandToOutputs(o)
			if err != nil {
				a.Tracef(TraceTagIncrementalAttacher, "error while parsing input: %v", err)
			} else {
				otherOutputs = append(otherOutputs, outputs...)
			}
		case ledger.StemLockName:
			a.Assertf(a.targetTs.IsSlotBoundary(), "a.targetTs.IsSlotBoundary()")
			stemIn = a.stemOutput.OutputWithID()
			a.Assertf(stemIn != nil && stemIn.ID == o.ID, "stemIn != nil && stemIn.id == o.id")
			// skip
		default:
			a.Assertf(false, "unexpected output type %s", o.Output.Lock().Name())
		}
	}

	endorsements := make([]ledger.TransactionID, len(a.endorse))
	for i, vid := range a.endorse {
		endorsements[i] = vid.ID()
	}
	// create sequencer transaction
	txBytes, inputLoader, err := txbuilder.MakeSequencerTransactionWithInputLoader(txbuilder.MakeSequencerTransactionParams{
		SeqName:           seqName,
		ChainInput:        chainIn.MustAsChainOutput(),
		StemInput:         stemIn,
		Timestamp:         a.targetTs,
		DelegationOutputs: delegationInputs,
		AdditionalInputs:  tagAlongInputs,
		WithdrawOutputs:   otherOutputs,
		Endorsements:      endorsements,
		PrivateKey:        privateKey,
		InflateMainChain:  true,
	})
	if err != nil {
		return nil, err
	}
	tx, err := transaction.FromBytes(txBytes, append(transaction.MainTxValidationOptions, transaction.ValidateOptionWithFullContext(inputLoader))...)
	if err != nil {
		if tx != nil {
			err = fmt.Errorf("%w:\n%s", err, tx.ToStringWithInputLoaderByIndex(inputLoader))
		}
		a.Log().Fatalf("IncrementalAttacher.MakeSequencerTransaction: %v", err) // should produce correct transaction
		return nil, err
	}

	a.slotInflation = a.pastCone.CalculateSlotInflation()
	// in the incremental attacher we must add inflation on the branch
	a.slotInflation += tx.InflationAmount()

	//a.Log().Infof("\n>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>\n%s\n<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<", a.dumpLines().String())
	return tx, nil
}

func (a *IncrementalAttacher) TargetTs() ledger.Time {
	return a.targetTs
}

func (a *IncrementalAttacher) NumInputs() int {
	return len(a.inputs) + 2
}

// Completed returns true is past cone is all solid and consistent (no conflicts)
// For incremental attacher it may happen (in theory) that some outputs need re-pull,
// if unlucky. The owner of the attacher will have to dismiss the attacher
// and try again later
func (a *IncrementalAttacher) Completed() bool {
	return a.pastCone.IsComplete()
}

func (a *IncrementalAttacher) Extending() vertex.WrappedOutput {
	return a.inputs[0]
}

func (a *IncrementalAttacher) Endorsing() []*vertex.WrappedTx {
	return a.endorse
}

func (a *IncrementalAttacher) Check() *vertex.WrappedOutput {
	return a.pastCone.Check(a.baselineStateReader())
}
