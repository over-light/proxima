package utangle

import (
	"errors"
	"fmt"

	"github.com/lunfardo314/proxima/core"
	"github.com/lunfardo314/proxima/multistate"
	"github.com/lunfardo314/proxima/transaction"
	"github.com/lunfardo314/proxima/util"
)

func (ut *UTXOTangle) SolidifyInputsFromTxBytes(txBytes []byte) (*Vertex, error) {
	tx, err := transaction.FromBytesMainChecksWithOpt(txBytes)
	if err != nil {
		return nil, err
	}
	return ut.SolidifyInputs(tx)
}

func (ut *UTXOTangle) SolidifyInputs(tx *transaction.Transaction) (*Vertex, error) {
	ret := NewVertex(tx)
	if err := ret.FetchMissingDependencies(ut); err != nil {
		return nil, err
	}
	return ret, nil
}

func (ut *UTXOTangle) GetWrappedOutput(oid *core.OutputID, baselineState ...multistate.SugaredStateReader) (WrappedOutput, bool, bool) {
	txid := oid.TransactionID()
	if vid, found := ut.GetVertex(&txid); found {
		hasIt, invalid := vid.HasOutputAt(oid.Index())
		if invalid {
			return WrappedOutput{}, false, true
		}
		if hasIt {
			return WrappedOutput{VID: vid, Index: oid.Index()}, true, false
		}
		if oid.IsBranchTransaction() {
			// it means a virtual branch vertex exist but the output is not cached on it.
			// It won't be a seq or stem output
			return ut.wrapNewIntoExistingBranch(vid, oid)
		}
		// it is a virtual tx, output not cached
		return wrapNewIntoExistingNonBranch(vid, oid, baselineState...)
	}
	// transaction not on UTXO tangle
	if oid.BranchFlagON() {
		return ut.fetchAndWrapBranch(oid)
	}
	// non-branch not on the utxo tangle
	if len(baselineState) == 0 {
		// no info on input, maybe later
		return WrappedOutput{}, false, false
	}
	// looking for output in the provided state
	o, err := baselineState[0].GetOutput(oid)
	if err != nil {
		return WrappedOutput{}, false, !errors.Is(err, multistate.ErrNotFound)
	}
	// found. Creating and wrapping new virtual tx
	vt := newVirtualTx(&txid)
	vt.addOutput(oid.Index(), o)
	vid := vt.Wrap()
	ut.AddVertexNoSaveTx(vid)

	return WrappedOutput{VID: vid, Index: oid.Index()}, true, false
}

func (ut *UTXOTangle) fetchAndWrapBranch(oid *core.OutputID) (WrappedOutput, bool, bool) {
	// it is a branch tx output, fetch the whole branch
	bd, branchFound := multistate.FetchBranchData(ut.stateStore, oid.TransactionID())
	if !branchFound {
		// maybe later
		return WrappedOutput{}, false, false
	}
	// branch found. Create virtualTx with seq and stem outputs
	vt := newVirtualBranchTx(&bd)
	if oid.Index() != bd.SeqOutput.ID.Index() && oid.Index() != bd.Stem.ID.Index() {
		// not seq or stem
		rdr := multistate.MustNewSugaredStateReader(ut.stateStore, bd.Root)
		o, err := rdr.GetOutput(oid)
		if err != nil {
			// if the output cannot be fetched from the branch state, it does not exist
			return WrappedOutput{}, false, true
		}
		vt.addOutput(oid.Index(), o)
	}
	vid := vt.Wrap()
	ut.AddVertexAndBranch(vid, bd.Root)
	return WrappedOutput{VID: vid, Index: oid.Index()}, true, false
}

func wrapNewIntoExistingNonBranch(vid *WrappedTx, oid *core.OutputID, baselineState ...multistate.SugaredStateReader) (WrappedOutput, bool, bool) {
	util.Assertf(!oid.BranchFlagON(), "%s should not be branch", oid.Short())
	// Don't have output in existing vertex, but it may be a virtualTx
	if len(baselineState) == 0 {
		return WrappedOutput{}, false, false
	}
	var ret WrappedOutput
	var available, invalid bool
	vid.Unwrap(UnwrapOptions{
		VirtualTx: func(v *VirtualTransaction) {
			o, err := baselineState[0].GetOutput(oid)
			if errors.Is(err, multistate.ErrNotFound) {
				return // null, false, false
			}
			if err != nil {
				invalid = true
				return // null, false, true
			}
			v.addOutput(oid.Index(), o)
			ret = WrappedOutput{VID: vid, Index: oid.Index()}
			available = true
			return // ret, true, false
		},
	})
	return ret, available, invalid
}

func (ut *UTXOTangle) wrapNewIntoExistingBranch(vid *WrappedTx, oid *core.OutputID) (WrappedOutput, bool, bool) {
	util.Assertf(oid.BranchFlagON(), "%s should be a branch", oid.Short())

	var ret WrappedOutput
	var available, invalid bool

	vid.Unwrap(UnwrapOptions{
		Vertex: func(v *Vertex) {
			util.Panicf("should be a virtualTx %s", oid.Short())
		},
		VirtualTx: func(v *VirtualTransaction) {
			_, already := v.OutputAt(oid.Index())
			util.Assertf(!already, "inconsistency: output %s should not exist in the virtualTx", func() any { return oid.Short() })

			bd, branchFound := multistate.FetchBranchData(ut.stateStore, oid.TransactionID())
			util.Assertf(branchFound, "inconsistency: branch %s must exist", oid.Short())

			rdr := multistate.MustNewSugaredStateReader(ut.stateStore, bd.Root)

			o, err := rdr.GetOutput(oid)
			if errors.Is(err, multistate.ErrNotFound) {
				return // null, false, false
			}
			if err != nil {
				invalid = true
				return // null, false, true
			}
			v.addOutput(oid.Index(), o)
			ret = WrappedOutput{VID: vid, Index: oid.Index()}
			available = true
			return // ret, true, false
		},
		Orphaned: func() {
			util.Panicf("should be a virtualTx %s", oid.Short())
		},
	})
	return ret, available, invalid
}

// FetchMissingDependencies check solidity of inputs and fetches what is available
// Does not obtain global lock on the tangle
// It means in general the result is non-deterministic, because some dependencies may be unavailable. This is ok for solidifier
// Once transaction has all dependencies solid, further on the result is deterministic
func (v *Vertex) FetchMissingDependencies(ut *UTXOTangle) error {
	var err error

	if v.StateDelta.branchTxID == nil {
		// if baseline not known yet, first try to solidify from the utxo tangle or from the heaviest state
		if err = v.fetchMissingInputs(ut, ut.HeaviestStateForLatestTimeSlot()); err != nil {
			return err
		}
		v.fetchMissingEndorsements(ut)
	}

	if v.Tx.IsSequencerMilestone() {
		// baseline state must ultimately be determined for milestone
		baselineBranchID, conflict := v.getInputBaselineBranchID()
		if conflict {
			return fmt.Errorf("conflicting branches among inputs of %s", v.Tx.IDShort())
		}
		if baselineBranchID == nil {
			if v.IsSolid() {
				return fmt.Errorf("can't determine baseline branch from inputs of %s", v.Tx.IDShort())
			}
			util.Panicf("inconsistency in %s", v.Tx.IDShort())
		}
		v.StateDelta.branchTxID = baselineBranchID
		if !v.IsSolid() {
			// if still not solid, try to fetch remaining inputs with baseline state
			if err = v.fetchMissingInputs(ut, ut.MustGetSugaredStateReader(baselineBranchID)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (v *Vertex) fetchMissingInputs(ut *UTXOTangle, baselineState ...multistate.SugaredStateReader) error {
	var err error
	var ok, invalid bool
	var wOut WrappedOutput

	v.Tx.ForEachInput(func(i byte, oid *core.OutputID) bool {
		if v.Inputs[i] != nil {
			// it is already solid
			return true
		}
		wOut, ok, invalid = ut.GetWrappedOutput(oid, baselineState...)
		if invalid {
			err = fmt.Errorf("wrong output %s", oid.Short())
			return false
		}
		if ok {
			v.Inputs[i] = wOut.VID
		}
		return true
	})
	return err
}

func (v *Vertex) fetchMissingEndorsements(ut *UTXOTangle) {
	v.Tx.ForEachEndorsement(func(i byte, txid *core.TransactionID) bool {
		if v.Endorsements[i] != nil {
			// already solid
			return true
		}
		util.Assertf(v.Tx.TimeSlot() == txid.TimeSlot(), "tx.TimeTick() == txid.TimeTick()")
		if vEnd, solid := ut.GetVertex(txid); solid {
			util.Assertf(vEnd.IsSequencerMilestone(), "vEnd.IsSequencerMilestone()")
			v.Endorsements[i] = vEnd
		}
		return true
	})
}

// getInputBaselineBranchID scans known (solid) inputs and extracts baseline branch ID. Returns:
// - conflict == true if inputs belongs to conflicting branches
// - nil, false if known inputs does not give a common baseline (yet)
// - txid, false if known inputs has latest branchID (even if not all solid yet)
func (v *Vertex) getInputBaselineBranchID() (ret *core.TransactionID, conflict bool) {
	branchIDsBySlot := make(map[core.TimeSlot]*core.TransactionID)
	v.forEachDependency(func(inp *WrappedTx) bool {
		if inp == nil {
			return true
		}
		branchTxID := inp.BaseBranchTXID()
		if branchTxID == nil {
			return true
		}
		slot := branchTxID.TimeSlot()
		if branchTxID1, already := branchIDsBySlot[slot]; already {
			if *branchTxID != *branchTxID1 {
				// two different branches in the same slot -> conflict
				conflict = true
				return false
			}
		} else {
			branchIDsBySlot[slot] = branchTxID
		}
		return true
	})
	if conflict {
		return
	}
	if len(branchIDsBySlot) == 0 {
		return
	}
	ret = util.Maximum(util.Values(branchIDsBySlot), func(branchTxID1, branchTxID2 *core.TransactionID) bool {
		return branchTxID1.TimeSlot() < branchTxID2.TimeSlot()
	})
	return
}

// getBranchConeTipVertex for a sequencer transaction, it finds a vertex which is to follow towards
// the branch transaction
// Returns:
// - nil, nil if it is not solid
// - nil, err if input is wrong, i.e. it cannot be solidified
// - vertex, nil if vertex, the branch cone tip, has been found
func (ut *UTXOTangle) getBranchConeTipVertex(tx *transaction.Transaction) (*WrappedTx, error) {
	util.Assertf(tx.IsSequencerMilestone(), "tx.IsSequencerMilestone()")
	oid := tx.SequencerChainPredecessorOutputID()
	if oid == nil {
		// this transaction is chain origin, i.e. it does not have predecessor
		// follow the first endorsement. It enforced by transaction constraint layer
		return ut.mustGetFirstEndorsedVertex(tx), nil
	}
	// sequencer chain predecessor exists
	if oid.TimeSlot() == tx.TimeSlot() {
		if oid.SequencerFlagON() {
			ret, ok, invalid := ut.GetWrappedOutput(oid)
			if invalid {
				return nil, fmt.Errorf("wrong output %s", oid.Short())
			}
			if !ok {
				return nil, nil
			}
			return ret.VID, nil
		}
		return ut.mustGetFirstEndorsedVertex(tx), nil
	}
	if tx.IsBranchTransaction() {
		ret, ok, invalid := ut.GetWrappedOutput(oid)
		if invalid {
			return nil, fmt.Errorf("wrong output %s", oid.Short())
		}
		if !ok {
			return nil, nil
		}
		return ret.VID, nil
	}
	return ut.mustGetFirstEndorsedVertex(tx), nil
}

// mustGetFirstEndorsedVertex returns first endorsement or nil if not solid
func (ut *UTXOTangle) mustGetFirstEndorsedVertex(tx *transaction.Transaction) *WrappedTx {
	util.Assertf(tx.NumEndorsements() > 0, "tx.NumEndorsements() > 0 @ %s", func() any { return tx.IDShort() })
	txid := tx.EndorsementAt(0)
	if ret, ok := ut.GetVertex(&txid); ok {
		return ret
	}
	// not solid
	return nil
}
