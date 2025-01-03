package delegate

import (
	"strconv"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/transaction"
	"github.com/lunfardo314/proxima/ledger/txbuilder"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/cobra"
)

func initDelegateSendCmd() *cobra.Command {
	delegateStartCmd := &cobra.Command{
		Use:     "send <amount>",
		Aliases: util.List("send"),
		Short:   `delegates amount to target ED25519 address by creating delegation output`,
		Args:    cobra.ExactArgs(1),
		Run:     runDelegateSendCmd,
	}

	glb.AddFlagTarget(delegateStartCmd)

	delegateStartCmd.InitDefaultHelpCmd()
	return delegateStartCmd
}

func runDelegateSendCmd(_ *cobra.Command, args []string) {
	glb.InitLedgerFromNode()
	walletData := glb.GetWalletData()

	glb.Infof("wallet account is: %s", walletData.Account.String())
	delegationTarget := glb.MustGetTarget()
	glb.Assertf(delegationTarget.Name() == ledger.AddressED25519Name, "delegation target must be ED25519 address")

	var tagAlongSeqID *ledger.ChainID
	feeAmount := glb.GetTagAlongFee()
	glb.Assertf(feeAmount > 0, "tag-along fee is configured 0. Fee-less option not supported yet")
	tagAlongSeqID = glb.GetTagAlongSequencerID()
	glb.Assertf(tagAlongSeqID != nil, "tag-along sequencer not specified")

	amountInt, err := strconv.Atoi(args[0])
	glb.AssertNoError(err)
	amount := uint64(amountInt)
	glb.Assertf(amount >= ledger.MinimumDelegationAmount(), "amount must be >= %d", ledger.MinimumDelegationAmount())

	client := glb.GetClient()
	walletOutputs, lrbid, err := client.GetAccountOutputsExt(walletData.Account, 100, "asc")
	glb.AssertNoError(err)
	glb.PrintLRB(lrbid)

	sum := uint64(0)
	walletOutputs = util.PurgeSlice(walletOutputs, func(o *ledger.OutputWithID) bool {
		sum += o.Output.Amount()
		return sum < amount+feeAmount
	})

	txb := txbuilder.New()
	totalAmountConsumed, inTs, err := txb.ConsumeOutputs(walletOutputs...)
	glb.AssertNoError(err)

	for i := range walletOutputs {
		if i == 0 {
			txb.PutSignatureUnlock(0)
		} else {
			err = txb.PutUnlockReference(byte(i), ledger.ConstraintIndexLock, 0)
			glb.AssertNoError(err)
		}
	}

	outDelegation := ledger.NewOutput(func(o *ledger.Output) {
		o.WithAmount(amount)
		o.WithLock(ledger.NewDelegationLock(walletData.Account, delegationTarget, 2))
		_, _ = o.PushConstraint(ledger.NewChainOrigin().Bytes())
	})
	_, _ = txb.ProduceOutput(outDelegation)

	outTagAlong := ledger.NewOutput(func(o *ledger.Output) {
		o.WithAmount(feeAmount)
		o.WithLock(tagAlongSeqID.AsChainLock())
	})
	_, _ = txb.ProduceOutput(outTagAlong)

	if totalAmountConsumed > amount+feeAmount {
		remainderOut := ledger.NewOutput(func(o *ledger.Output) {
			o.WithAmount(totalAmountConsumed - amount - feeAmount)
			o.WithLock(walletData.Account)
		})
		_, _ = txb.ProduceOutput(remainderOut)
	}

	txb.TransactionData.Timestamp = ledger.MaximumTime(inTs, ledger.TimeNow())
	txb.TransactionData.InputCommitment = txb.InputCommitment()
	txb.SignED25519(walletData.PrivateKey)

	txBytes := txb.TransactionData.Bytes()
	tx, err := transaction.FromBytes(txBytes, transaction.MainTxValidationOptions...)
	glb.AssertNoError(err)
	err = tx.Validate(transaction.ValidateOptionWithFullContext(txb.LoadInput))
	glb.AssertNoError(err)

	err = client.SubmitTransaction(txBytes)
	glb.AssertNoError(err)
}
