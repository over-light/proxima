package inflator

import (
	"crypto/ed25519"
	"sort"
	"time"

	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
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
		Target     ledger.AddressED25519
		PrivateKey ed25519.PrivateKey
	}

	InflatableOutput struct {
		ledger.OutputWithChainID
		Inflation uint64
		Margin    uint64
		Successor *ledger.Output
	}
)

const (
	minimumInflationPerOutput = 50
	marginPromille            = 10
	tagAlongAmount            = 50
)

func New(env environment, par Params) *Inflator {
	return &Inflator{
		environment: env,
		par:         par,
		consumed:    make(map[ledger.OutputID]time.Time),
	}
}

func (fl *Inflator) collectInflatableOutputs(targetTs ledger.Time, rdr multistate.SugaredStateReader) []*InflatableOutput {
	if targetTs.IsSlotBoundary() {
		return nil
	}
	ret := make([]*InflatableOutput, 0)
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
	return ret
}

func (fl *Inflator) makeTransaction(targetTs ledger.Time, rdr multistate.SugaredStateReader) ([]byte, error) {
	outs := fl.collectInflatableOutputs(targetTs, rdr)
	if len(outs) == 0 {
		return nil, nil
	}
	totalMargin := uint64(0)
	for _, o := range outs {
		totalMargin += o.Margin
	}
	ownOuts := rdr.GetOutputsLockedInAddressED25519(fl.par.Target)
	sort.Slice(ownOuts, func(i, j int) bool {
		return ownOuts[i].Output.Amount() > ownOuts[j].Output.Amount()
	})
	ownOuts = util.TrimSlice(ownOuts, 10)
	totalInOwnAccount := uint64(0)
	for _, o := range ownOuts {
		totalInOwnAccount += o.Output.Amount()
	}

	txb := txbuilder.New()

}

func makeDelegationSuccessor(pred *ledger.OutputWithID, predInputIdx byte, inflation, margin uint64) (*ledger.Output, byte) {
	ccPred, ccIdx := pred.Output.ChainConstraint()
	util.Assertf(ccIdx != 0xff, "inconsistency: can't find chain constraint")
	chainID := ccPred.ID
	if ccPred.IsOrigin() {
		chainID = ledger.MakeOriginChainID(&pred.ID)
	}

	succOut := ledger.NewOutput(func(o *ledger.Output) {
		o.WithAmount(pred.Output.Amount() + inflation - margin)
		o.WithLock(pred.Output.Lock())
		ccSucc := ledger.ChainConstraint{
			ID:                         chainID,
			TransitionMode:             0,
			PredecessorInputIndex:      predInputIdx,
			PredecessorConstraintIndex: ccIdx,
		}
		ccIdx, _ = o.PushConstraint(ccSucc.Bytes())
		if inflation > 0 {
			ccInfl := ledger.InflationConstraint{
				ChainInflation:       inflation,
				ChainConstraintIndex: ccIdx,
			}
			_, _ = o.PushConstraint(ccInfl.Bytes())
		}
	})

	return succOut, ccIdx
}
