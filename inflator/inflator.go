package inflator

import (
	"crypto/ed25519"
	"fmt"
	"sort"
	"time"

	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/transaction"
	"github.com/lunfardo314/proxima/ledger/txbuilder"
	"github.com/lunfardo314/proxima/multistate"
	"github.com/lunfardo314/proxima/util"
)

const Name = "inflator"

type (
	environment interface {
		global.NodeGlobal
		LatestReliableState() (multistate.SugaredStateReader, error)
	}

	Inflator struct {
		environment
		par      Params
		consumed map[ledger.OutputID]time.Time
	}

	Params struct {
		Target            ledger.AddressED25519
		PrivateKey        ed25519.PrivateKey
		TagAlongSequencer ledger.ChainID
	}

	InflatableOutput struct {
		ledger.OutputWithChainID
		Inflation              uint64
		Margin                 uint64
		Successor              *ledger.Output
		SuccChainConstraintIdx byte
		UnlockParams           []byte
	}
)

const (
	minimumInflationPerOutput = 50
	marginPromille            = 10
	tagAlongAmount            = 50
	maxDelegationsPerTx       = 100
)

func New(env environment, par Params) *Inflator {
	return &Inflator{
		environment: env,
		par:         par,
		consumed:    make(map[ledger.OutputID]time.Time),
	}
}

func (fl *Inflator) collectTransitions(targetTs ledger.Time, rdr multistate.SugaredStateReader) ([]*InflatableOutput, uint64) {
	if targetTs.IsSlotBoundary() {
		return nil, 0
	}
	ret := make([]*InflatableOutput, 0)
	var totalMargin uint64

	rdr.IterateDelegatedOutputs(fl.par.Target, func(oid ledger.OutputID, o *ledger.Output, chainID ledger.ChainID, dLock *ledger.DelegationLock) bool {
		if _, already := fl.consumed[oid]; already {
			return true
		}
		if !ledger.IsOpenDelegationSlot(chainID, targetTs.Slot()) {
			// only considering delegated outputs which can be consumed in the target slo
			return true
		}
		inflation := ledger.L().CalcChainInflationAmount(oid.Timestamp(), targetTs, o.Amount(), 0)
		if inflation < minimumInflationPerOutput ||
			ledger.DiffTicks(targetTs, oid.Timestamp())%int64(ledger.TicksPerSlot) < int64(ledger.L().ID.ChainInflationOpportunitySlots/2) {
			// only consider outputs with enough inflation or older than half of the inflation opportunity window
			return true
		}
		ret = append(ret, &InflatableOutput{
			OutputWithChainID: ledger.OutputWithChainID{
				OutputWithID: ledger.OutputWithID{
					ID:     oid,
					Output: o,
				},
				ChainID: chainID,
			},
			Inflation: inflation,
			Margin:    (inflation * marginPromille) / 1000,
		})
		return true
	})
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Timestamp().Before(ret[j].Timestamp())
	})
	ret = util.TrimSlice(ret, maxDelegationsPerTx)

	for i, pred := range ret {
		util.Assertf(pred.Inflation-pred.Margin >= 0, "pred.Inflation-pred.Margin")
		ccPred, ccIdx := pred.Output.ChainConstraint()
		util.Assertf(ccIdx != 0xff, "inconsistency: can't find chain constraint")
		chainID := ccPred.ID
		if ccPred.IsOrigin() {
			chainID = ledger.MakeOriginChainID(&pred.ID)
		}
		var err error
		pred.Successor = ledger.NewOutput(func(o *ledger.Output) {
			o.WithAmount(o.Amount() + pred.Inflation - pred.Margin)
			o.WithLock(pred.Output.Lock())
			ccSucc := ledger.ChainConstraint{
				ID:                         chainID,
				TransitionMode:             0,
				PredecessorInputIndex:      byte(i),
				PredecessorConstraintIndex: ccIdx,
			}
			ccIdx, err = o.PushConstraint(ccSucc.Bytes())
			util.AssertNoError(err)
			if pred.Inflation > 0 {
				ccInfl := ledger.InflationConstraint{
					ChainInflation:       pred.Inflation,
					ChainConstraintIndex: ccIdx,
				}
				_, err = o.PushConstraint(ccInfl.Bytes())
				util.AssertNoError(err)
			}
		})
		pred.SuccChainConstraintIdx = ccIdx
		pred.UnlockParams = []byte{byte(i), ccIdx, 0}
		totalMargin += pred.Margin

	}
	return ret, totalMargin
}

func (fl *Inflator) makeTransaction(targetTs ledger.Time, rdr multistate.SugaredStateReader) ([]byte, error) {
	outs, totalMarginOut := fl.collectTransitions(targetTs, rdr)
	if len(outs) == 0 {
		return nil, nil
	}

	txb := txbuilder.New()
	for _, o := range outs {
		inIdx, _ := txb.ConsumeOutput(o.Output, o.ID)
		_, _ = txb.ProduceOutput(o.Successor)
		txb.PutUnlockParams(inIdx, o.PredecessorConstraintIndex, o.UnlockParams)
	}

	if totalMarginOut >= tagAlongAmount {
		// enough collected margin for tag along
		tagAlongOut := ledger.NewOutput(func(o *ledger.Output) {
			o.WithAmount(totalMarginOut)
			o.WithLock(fl.par.TagAlongSequencer.AsChainLock())
		})
		_, _ = txb.ProduceOutput(tagAlongOut)
	} else {
		// not enough collected margin for tag along. Use own funds
		ownOuts, actualAmount := rdr.GetOutputsLockedInAddressED25519ForAmount(fl.par.Target, tagAlongAmount)
		if actualAmount < tagAlongAmount {
			return nil, fmt.Errorf("not enough funds for the tag-along of the transaction")
		}
		first := true
		var firstIdx byte
		for _, o := range ownOuts {
			idx, err := txb.ConsumeOutput(o.Output, o.ID)
			if err != nil {
				return nil, err
			}
			if first {
				txb.PutSignatureUnlock(idx)
				first = false
				firstIdx = idx
			} else {
				err = txb.PutUnlockReference(idx, ledger.ConstraintIndexLock, firstIdx)
				if err != nil {
					return nil, err
				}
			}
		}
		tagAlongOut := ledger.NewOutput(func(o *ledger.Output) {
			o.WithAmount(tagAlongAmount)
			o.WithLock(fl.par.TagAlongSequencer.AsChainLock())
		})
		_, _ = txb.ProduceOutput(tagAlongOut)
		if tagAlongAmount < actualAmount {
			_, err := txb.ProduceOutput(ledger.NewOutput(func(o *ledger.Output) {
				o.WithAmount(actualAmount - tagAlongAmount) // TODO not completely correct wrt storage deposit constraint
				o.WithLock(fl.par.Target)
			}))
			if err != nil {
				return nil, err
			}
		}
	}

	txb.TransactionData.Timestamp = targetTs
	txb.TransactionData.InputCommitment = txb.InputCommitment()
	txb.SignED25519(fl.par.PrivateKey)

	txBytes := txb.TransactionData.Bytes()

	tx, err := transaction.FromBytes(txBytes, transaction.MainTxValidationOptions...)
	if err != nil {
		return nil, err
	}
	ctx, err := transaction.TxContextFromTransaction(tx, txb.LoadInput)
	if err != nil {
		return nil, err
	}
	err = ctx.Validate()
	if err != nil {
		return nil, err
	}
	return txBytes, nil
}
