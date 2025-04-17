package txbuilder

import (
	"crypto/ed25519"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/ledger/transaction"
	"github.com/lunfardo314/proxima/util"
)

type EndChainParams struct {
	// transaction timestamp. Not adjusted
	Timestamp base.LedgerTime
	// chain output data
	ChainIn *ledger.OutputWithChainID
	// controlling private key
	PrivateKey ed25519.PrivateKey
	// tag-along sequencer and fee amount
	TagAlongSeqID ledger.ChainID
	TagAlongFee   uint64 // 0 means no fee output will be produced
}

func MakeEndChainTransaction(par EndChainParams) (*transaction.Transaction, error) {
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
	txb.TransactionData.Timestamp = par.Timestamp
	txb.TransactionData.InputCommitment = txb.InputCommitment()
	txb.SignED25519(par.PrivateKey)

	tx, err := txb.Transaction()
	if err != nil {
		return nil, err
	}
	return tx, nil
}
