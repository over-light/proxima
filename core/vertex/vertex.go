package vertex

import (
	"fmt"
	"strings"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/ledger/transaction"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/lines"
	"github.com/lunfardo314/proxima/util/set"
)

func NewVertex(tx *transaction.Transaction) *Vertex {
	return &Vertex{
		Tx:           tx,
		Inputs:       make([]*WrappedTx, tx.NumInputs()),
		Endorsements: make([]*WrappedTx, tx.NumEndorsements()),
	}
}

func (v *Vertex) TimeSlot() base.Slot {
	return v.Tx.Slot()
}

// ReferenceInput puts new input and references it. If referencing fails, no change happens and returns false
func (v *Vertex) ReferenceInput(i byte, vid *WrappedTx) {
	util.Assertf(int(i) < len(v.Inputs), "ReferenceInput: wrong input index")
	util.Assertf(v.Inputs[i] == nil, "ReferenceInput: repetitive")

	v.Inputs[i] = vid
}

func (v *Vertex) ReferenceEndorsement(i byte, vid *WrappedTx) {
	util.Assertf(int(i) < len(v.Endorsements), "ReferenceEndorsement: wrong endorsement index")
	util.Assertf(v.Endorsements[i] == nil, "ReferenceEndorsement: repetitive")

	v.Endorsements[i] = vid
}

// UnReferenceDependencies un-references all not nil inputs and endorsements and invalidates vertex structure
// TODO revisit usages
func (v *Vertex) UnReferenceDependencies() {
	clear(v.Inputs)
	clear(v.Endorsements)
	v.BaselineBranch = nil
}

// InputLoaderByIndex returns consumed output at index i or nil (if input is orphaned or inaccessible in the virtualTx)
func (v *Vertex) InputLoaderByIndex(i byte) (*ledger.Output, error) {
	o := v.GetConsumedOutput(i)
	if o == nil {
		oid := v.Tx.MustInputAt(i)
		return nil, fmt.Errorf("InputLoaderByIndex: consumed output %s at index %d is not available", oid.StringShort(), i)
	}
	return o, nil
}

// GetConsumedOutput return produced output, is available. Returns nil if unavailable for any reason
func (v *Vertex) GetConsumedOutput(i byte) (ret *ledger.Output) {
	if int(i) >= len(v.Inputs) || v.Inputs[i] == nil {
		return
	}
	v.Inputs[i].RUnwrap(UnwrapOptions{
		Vertex: func(vCons *Vertex) {
			ret = vCons.Tx.MustProducedOutputAt(v.Tx.MustOutputIndexOfTheInput(i))
		},
		DetachedVertex: func(v *DetachedVertex) {
			ret = v.Tx.MustProducedOutputAt(v.Tx.MustOutputIndexOfTheInput(i))
		},
		VirtualTx: func(vCons *VirtualTransaction) {
			ret, _ = vCons.OutputAt(v.Tx.MustOutputIndexOfTheInput(i))
		},
	})
	return
}

// ValidateConstraints creates full transaction context from the (solid) vertex data
// and runs validation of all constraints in the context
func (v *Vertex) ValidateConstraints(traceOption ...int) error {
	traceOpt := transaction.TraceOptionFailedConstraints
	if len(traceOption) > 0 {
		traceOpt = traceOption[0]
	}
	ctx, err := transaction.TxContextFromTransaction(v.Tx, v.InputLoaderByIndex, traceOpt)
	if err != nil {
		return fmt.Errorf("ValidateConstraints of %s: %w", v.Tx.IDShortString(), err)
	}
	err = ctx.Validate()

	const validateConstraintsVerbose = false

	if err != nil {
		if validateConstraintsVerbose {
			err = fmt.Errorf("ValidateConstraints: %w \n>>>>>>>>>>>>>>>>>>>>>\n%s", err, ctx.String())
		} else {
			err = fmt.Errorf("ValidateConstraints: %s: %w", v.Tx.IDShortString(), err)
		}
		return err
	}
	return nil
}

func (v *Vertex) NumMissingInputs() (missingInputs int, missingEndorsements int) {
	v.ForEachInputDependency(func(_ byte, vidInput *WrappedTx) bool {
		if vidInput == nil {
			missingInputs++
		}
		return true
	})
	v.ForEachEndorsement(func(_ byte, vidEndorsed *WrappedTx) bool {
		if vidEndorsed == nil {
			missingEndorsements++
		}
		return true
	})
	return
}

// MissingInputTxIDSet returns set of txids for the missing inputs and endorsements
func (v *Vertex) MissingInputTxIDSet() set.Set[ledger.TransactionID] {
	ret := set.New[ledger.TransactionID]()
	var oid ledger.OutputID
	v.ForEachInputDependency(func(i byte, vidInput *WrappedTx) bool {
		if vidInput == nil {
			oid = v.Tx.MustInputAt(i)
			ret.Insert(oid.TransactionID())
		}
		return true
	})
	v.ForEachEndorsement(func(i byte, vidEndorsed *WrappedTx) bool {
		if vidEndorsed == nil {
			ret.Insert(v.Tx.EndorsementAt(i))
		}
		return true
	})
	return ret
}

func (v *Vertex) MissingInputTxIDString() string {
	s := v.MissingInputTxIDSet()
	if len(s) == 0 {
		return "(none)"
	}
	ret := make([]string, 0)
	for txid := range s {
		ret = append(ret, txid.StringShort())
	}
	return strings.Join(ret, ", ")
}

func (v *Vertex) StemInputIndex() byte {
	util.Assertf(v.Tx.IsBranchTransaction(), "branch vertex expected")

	predOID := v.Tx.StemOutputData().PredecessorOutputID
	var stemInputIdx byte
	var stemInputFound bool

	v.Tx.ForEachInput(func(i byte, oid ledger.OutputID) bool {
		if oid == predOID {
			stemInputIdx = i
			stemInputFound = true
		}
		return !stemInputFound
	})
	util.Assertf(stemInputFound, "can't find stem input")
	return stemInputIdx
}

func (v *Vertex) SequencerInputIndex() byte {
	util.Assertf(v.Tx.IsSequencerTransaction(), "sequencer milestone expected")
	return v.Tx.SequencerTransactionData().SequencerOutputData.ChainConstraint.PredecessorInputIndex
}

func (v *Vertex) ForEachInputDependency(fun func(i byte, vidInput *WrappedTx) bool) {
	for i, inp := range v.Inputs {
		if !fun(byte(i), inp) {
			return
		}
	}
}

func (v *Vertex) ForEachEndorsement(fun func(i byte, vidEndorsed *WrappedTx) bool) {
	for i, vEnd := range v.Endorsements {
		if !fun(byte(i), vEnd) {
			return
		}
	}
}

func (v *Vertex) SetOfInputTransactions() set.Set[*WrappedTx] {
	ret := set.New[*WrappedTx]()
	v.ForEachInputDependency(func(_ byte, vidInput *WrappedTx) bool {
		ret.Insert(vidInput)
		return true
	})
	return ret
}

func (v *Vertex) Lines(prefix ...string) *lines.Lines {
	return v.Tx.Lines(func(i byte) (*ledger.Output, error) {
		if v.Inputs[i] == nil {
			return nil, fmt.Errorf("input #%d not solid", i)
		}
		inpOid, err := v.Tx.InputAt(i)
		if err != nil {
			return nil, fmt.Errorf("input #%d: %v", i, err)
		}
		return v.Inputs[i].OutputAt(inpOid.Index())
	}, prefix...)
}

func (v *DetachedVertex) Lines(prefix ...string) *lines.Lines {
	return v.Tx.LinesShort(prefix...)
}

func (v *Vertex) Wrap() *WrappedTx {
	var seqID *ledger.ChainID
	if v.Tx.IsSequencerTransaction() {
		seqID = util.Ref(v.Tx.SequencerTransactionData().SequencerID)
	}
	return _newVID(_vertex{Vertex: v}, v.Tx.ID(), seqID)
}
