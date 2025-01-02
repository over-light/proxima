package inflator

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/ledger/transaction"
	"github.com/lunfardo314/proxima/ledger/txbuilder"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/viper"
)

const Name = "inflator"

type (
	environment interface {
		global.NodeGlobal
		LatestReliableState() (multistate.SugaredStateReader, error)
		SubmitTxBytesFromInflator(txBytes []byte)
	}

	Inflator struct {
		environment
		cfg      *Params
		consumed map[ledger.OutputID]time.Time
	}

	Params struct {
		Enable                    bool
		Name                      string
		Target                    ledger.AddressED25519
		PrivateKey                ed25519.PrivateKey
		TagAlongSequencer         ledger.ChainID
		MinimumInflationPerOutput uint64
		MarginPromille            uint64
		TagAlongAmount            uint64
		MaxDelegationsPerTx       int
		KeepInConsumedListSlots   int
		NoInflationSlots          int
		LoopPeriod                time.Duration
	}

	InflatableOutput struct {
		ledger.OutputWithChainID
		Inflation                  uint64
		Margin                     uint64
		Successor                  *ledger.Output
		PredecessorConstraintIndex byte
		SuccChainConstraintIdx     byte
		UnlockParams               []byte
	}
)

const (
	minimumInflationPerOutput = 50
	marginPromille            = 100
	tagAlongAmount            = 50
	maxDelegationsPerTx       = 100
	keepInConsumedListSlots   = 3
	minimumNoInflationSlots   = 3
	defaultLoopPeriod         = 2 * time.Second
)

func New(env environment, par *Params) *Inflator {
	util.Assertf(par.Enable, "par.Enable")
	return &Inflator{
		cfg:         par,
		environment: env,
		consumed:    make(map[ledger.OutputID]time.Time),
	}
}

func (fl *Inflator) Run() {
	fl.Log().Infof("running inflator..")
	fl.environment.RepeatInBackground(fl.cfg.Name+"_"+Name+"_loop", fl.cfg.LoopPeriod, func() bool {
		fl.doStep(ledger.TimeNow())
		return true
	})
}

// collectInflatableTransitions returns list of outputs which can be inflated for the target timestamp
func (fl *Inflator) collectInflatableTransitions(targetTs ledger.Time, rdr multistate.SugaredStateReader) ([]*InflatableOutput, uint64) {
	if targetTs.IsSlotBoundary() {
		return nil, 0
	}
	ret := make([]*InflatableOutput, 0)
	var totalMargin uint64

	rdr.IterateDelegatedOutputs(fl.cfg.Target, func(oid ledger.OutputID, o *ledger.Output, dLock *ledger.DelegationLock) bool {
		if _, already := fl.consumed[oid]; already {
			return true
		}
		cc, idx := o.ChainConstraint()
		util.Assertf(idx != 0xff, "idx != 0xff")
		chainID := cc.ID
		if cc.IsOrigin() {
			chainID = ledger.MakeOriginChainID(&oid)
		}
		if !ledger.IsOpenDelegationSlot(chainID, targetTs.Slot()) {
			// only considering delegated outputs which can be consumed in the target slo
			return true
		}
		inflation := ledger.L().CalcChainInflationAmount(oid.Timestamp(), targetTs, o.Amount(), 0)
		if inflation < fl.cfg.MinimumInflationPerOutput ||
			ledger.DiffTicks(targetTs, oid.Timestamp())/int64(ledger.TicksPerSlot) < int64(fl.cfg.NoInflationSlots) {
			// only consider outputs with enough inflation or (usually) older than half of the inflation opportunity window
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
			Inflation:                  inflation,
			Margin:                     (inflation * fl.cfg.MarginPromille) / 1000,
			PredecessorConstraintIndex: idx,
		})
		return true
	})
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Timestamp().Before(ret[j].Timestamp())
	})
	ret = util.TrimSlice(ret, fl.cfg.MaxDelegationsPerTx)

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
			o.WithAmount(pred.Output.Amount() + pred.Inflation - pred.Margin)
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

var ErrNoInputs = errors.New("no delegated outputs has been found")

func (fl *Inflator) MakeTransaction(targetTs ledger.Time, rdr multistate.SugaredStateReader) (*transaction.Transaction, []*ledger.OutputID, uint64, error) {
	outs, totalMarginOut := fl.collectInflatableTransitions(targetTs, rdr)
	if len(outs) == 0 {
		return nil, nil, 0, fmt.Errorf("MakeTransaction: target = %s: %w", targetTs.String(), ErrNoInputs)
	}

	txb := txbuilder.New()
	for _, o := range outs {
		inIdx, _ := txb.ConsumeOutput(o.Output, o.ID)
		_, _ = txb.ProduceOutput(o.Successor)
		txb.PutSignatureUnlock(inIdx) // all of them will check signatures -> suboptimal
		txb.PutUnlockParams(inIdx, o.PredecessorConstraintIndex, o.UnlockParams)
	}

	if totalMarginOut >= fl.cfg.TagAlongAmount {
		// enough collected margin for tag along
		tagAlongOut := ledger.NewOutput(func(o *ledger.Output) {
			o.WithAmount(totalMarginOut)
			o.WithLock(fl.cfg.TagAlongSequencer.AsChainLock())
		})
		_, _ = txb.ProduceOutput(tagAlongOut)
	} else {
		// not enough collected margin for tag along. Use own funds
		ownOuts, actualAmount := rdr.GetOutputsLockedInAddressED25519ForAmount(fl.cfg.Target, fl.cfg.TagAlongAmount)
		if actualAmount < fl.cfg.TagAlongAmount {
			return nil, nil, 0, fmt.Errorf("not enough funds for the tag-along of the transaction")
		}
		first := true
		var firstIdx byte
		for _, o := range ownOuts {
			idx, err := txb.ConsumeOutput(o.Output, o.ID)
			if err != nil {
				return nil, nil, 0, err
			}
			if first {
				txb.PutSignatureUnlock(idx)
				first = false
				firstIdx = idx
			} else {
				err = txb.PutUnlockReference(idx, ledger.ConstraintIndexLock, firstIdx)
				if err != nil {
					return nil, nil, 0, err
				}
			}
		}
		tagAlongOut := ledger.NewOutput(func(o *ledger.Output) {
			o.WithAmount(fl.cfg.TagAlongAmount)
			o.WithLock(fl.cfg.TagAlongSequencer.AsChainLock())
		})
		_, _ = txb.ProduceOutput(tagAlongOut)
		consumedTotal := txb.ConsumedAmount()
		producedTotal, inflation := txb.ProducedAmount()
		targetOutTotal := consumedTotal + inflation
		util.Assertf(targetOutTotal >= producedTotal, "targetOutTotal >= producedTotal")

		if remainder := targetOutTotal - producedTotal; remainder > 0 {
			_, err := txb.ProduceOutput(ledger.NewOutput(func(o *ledger.Output) {
				o.WithAmount(remainder) // TODO not completely correct wrt storage deposit constraint
				o.WithLock(fl.cfg.Target)
			}))
			if err != nil {
				return nil, nil, 0, err
			}
		}
	}

	txb.TransactionData.Timestamp = targetTs
	txb.TransactionData.InputCommitment = txb.InputCommitment()
	txb.SignED25519(fl.cfg.PrivateKey)

	txBytes := txb.TransactionData.Bytes()

	tx, err := transaction.FromBytes(txBytes, transaction.MainTxValidationOptions...)
	if err != nil {
		return nil, nil, 0, err
	}
	return tx, txb.TransactionData.InputIDs, totalMarginOut, nil
}

func (fl *Inflator) doStep(targetTs ledger.Time) {
	fl.cleanConsumedList()
	lrb, err := fl.LatestReliableState()
	if err != nil {
		fl.Log().Warnf("[%s] %v", Name, err)
		return
	}
	if targetTs.IsSlotBoundary() {
		fl.Log().Infof("[%s] skip target %s: on slot boundary", Name, targetTs.String())
		return
	}
	tx, outIDs, margin, err := fl.MakeTransaction(targetTs, lrb)
	if errors.Is(err, ErrNoInputs) {
		fl.Log().Infof("[%s] skip target %s: '%v'", Name, targetTs.String(), err)
		return
	}
	if err != nil {
		fl.Log().Errorf("[%s] error while generating transaction: '%v'", Name, err)
		return
	}
	// double check before submitting
	if err = tx.Validate(transaction.ValidateOptionWithFullContext(tx.InputLoaderFromState(lrb))); err != nil {
		fl.Log().Errorf("[%s] error while validating transaction: '%v'", Name, err)
		return
	}
	nowis := time.Now()
	for _, oid := range outIDs {
		fl.consumed[*oid] = nowis
	}
	fl.SubmitTxBytesFromInflator(tx.Bytes())
	fl.Log().Infof("[%s] submitted transaction %s. Inputs: %d, total amount: %s, inflation: %s, margin collected: %s",
		Name, tx.IDShortString(), tx.NumInputs(), util.Th(tx.TotalAmount()), util.Th(tx.InflationAmount()), util.Th(margin))
}

func (fl *Inflator) cleanConsumedList() {
	keep := ledger.L().ID.SlotDuration() * time.Duration(fl.cfg.KeepInConsumedListSlots)
	for oid, when := range fl.consumed {
		if time.Since(when) > keep {
			delete(fl.consumed, oid)
		}
	}
}

func ParamsFromConfig(seqID ledger.ChainID, seqPrivateKey ed25519.PrivateKey) *Params {
	sub := viper.Sub("sequencer.inflator")
	if sub == nil {
		ret := &Params{
			PrivateKey:        seqPrivateKey,
			TagAlongSequencer: seqID,
		}
		ret.adjustDefaults()
		return ret
	}
	ret := &Params{
		Enable:                    viper.GetBool("sequencer.enable") && sub.GetBool("enable"),
		Target:                    ledger.AddressED25519FromPrivateKey(seqPrivateKey),
		PrivateKey:                seqPrivateKey,
		TagAlongSequencer:         seqID,
		MinimumInflationPerOutput: sub.GetUint64("minimum_inflation_per_output"),
		MarginPromille:            sub.GetUint64("margin_promille"),
		TagAlongAmount:            sub.GetUint64("tag_along_amount"),
		MaxDelegationsPerTx:       sub.GetInt("max_delegations_per_tx"),
		KeepInConsumedListSlots:   sub.GetInt("keep_in_consumed_list_slots"),
		NoInflationSlots:          sub.GetInt("no_inflation_slots"),
		LoopPeriod:                time.Duration(sub.GetInt("loop_period_seconds")) * time.Second,
	}
	ret.adjustDefaults()
	return ret
}

func (par *Params) adjustDefaults() {
	if par.MinimumInflationPerOutput < minimumInflationPerOutput {
		par.MinimumInflationPerOutput = minimumInflationPerOutput
	}
	if par.MarginPromille > 100 || par.MarginPromille < 0 {
		par.MarginPromille = marginPromille
	}
	if par.TagAlongAmount < tagAlongAmount {
		par.TagAlongAmount = tagAlongAmount
	}
	if par.MaxDelegationsPerTx > 254 || par.MaxDelegationsPerTx < 1 {
		par.MaxDelegationsPerTx = maxDelegationsPerTx
	}
	if par.KeepInConsumedListSlots < keepInConsumedListSlots {
		par.KeepInConsumedListSlots = keepInConsumedListSlots
	}
	if par.NoInflationSlots < minimumNoInflationSlots || uint64(par.NoInflationSlots) > ledger.L().ID.ChainInflationOpportunitySlots {
		par.KeepInConsumedListSlots = int(ledger.L().ID.ChainInflationOpportunitySlots / 2)
	}
	if par.LoopPeriod < 100*time.Millisecond {
		par.LoopPeriod = defaultLoopPeriod
	}
}
