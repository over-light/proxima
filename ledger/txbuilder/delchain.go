package txbuilder

import (
	"crypto/ed25519"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/transaction"
	"github.com/lunfardo314/proxima/util"
)

type DeleteChainParams struct {
	ChainIn                       *ledger.OutputWithChainID
	PrivateKey                    ed25519.PrivateKey
	TagAlongSeqID                 ledger.ChainID
	TagAlongFee                   uint64 // 0 means no fee output will be produced
	EnforceNoDelegationTransition bool
}

func MakeDeleteChainTransaction(par DeleteChainParams) (*transaction.Transaction, error) {
	chainID, _, _ := par.ChainIn.ExtractChainID()
	// adjust timestamp
	ts := ledger.MaximumTime(ledger.TimeNow(), par.ChainIn.Timestamp().AddTicks(int(ledger.L().ID.TransactionPace)))
	if ts.IsSlotBoundary() {
		ts.AddTicks(1)
	}
	if par.ChainIn.Output.Lock().Name() == ledger.DelegationLockName && par.EnforceNoDelegationTransition {
		ts1 := ledger.NextClosedDelegationTimestamp(chainID, ts)
		if ts1.Slot() != ts.Slot() {
			ts = ledger.NewLedgerTime(ts1.Slot(), 1)
		}
	}

	_, predecessorConstraintIndex := par.ChainIn.Output.ChainConstraint()
	txb := New()

	consumedIndex, err := txb.ConsumeOutput(par.ChainIn.Output, par.ChainIn.ID)
	util.AssertNoError(err)

	feeAmount := par.TagAlongFee

	outNonChain := ledger.NewOutput(func(o *ledger.Output) {
		o.WithAmount(par.ChainIn.Output.Amount() - feeAmount).
			WithLock(ledger.AddressED25519FromPrivateKey(par.PrivateKey))
	})
	_, err = txb.ProduceOutput(outNonChain)
	util.AssertNoError(err)

	if feeAmount > 0 {
		tagAlongFeeOut := ledger.NewOutput(func(o *ledger.Output) {
			o.WithAmount(feeAmount).
				WithLock(ledger.ChainLockFromChainID(par.TagAlongSeqID))
		})
		if _, err = txb.ProduceOutput(tagAlongFeeOut); err != nil {
			return nil, err
		}
	}

	txb.PutUnlockParams(consumedIndex, predecessorConstraintIndex, []byte{0xff, 0xff, 0xff})
	txb.PutSignatureUnlock(consumedIndex)

	// finalize the transaction
	txb.TransactionData.Timestamp = ts
	txb.TransactionData.InputCommitment = txb.InputCommitment()
	txb.SignED25519(par.PrivateKey)

	tx, err := txb.Transaction()
	if err != nil {
		return nil, err
	}
	return tx, nil
}
