package node_cmd

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/txbuilder"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/cobra"
)

func initDelegateCmd() *cobra.Command {
	delegateStartCmd := &cobra.Command{
		Use:     "delegate <amount> -t <ed25519 address>",
		Aliases: util.List("send"),
		Short:   `delegates amount to target ED25519 address by creating delegation chain output`,
		Args:    cobra.ExactArgs(1),
		Run:     runDelegateCmd,
	}

	glb.AddFlagTarget(delegateStartCmd)

	delegateStartCmd.InitDefaultHelpCmd()
	return delegateStartCmd
}

func runDelegateCmd(_ *cobra.Command, args []string) {
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
		if sum >= amount+feeAmount {
			return false
		}
		sum += o.Output.Amount()
		return true
	})

	txb := txbuilder.New()
	totalAmountConsumed, inTs, err := txb.ConsumeOutputs(walletOutputs...)
	glb.AssertNoError(err)

	ts := ledger.MaximumTime(inTs, ledger.TimeNow())

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
		o.WithLock(ledger.NewDelegationLock(walletData.Account, delegationTarget, 2, ts, amount))
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

	txb.TransactionData.Timestamp = ts
	txb.TransactionData.InputCommitment = txb.InputCommitment()
	txb.SignED25519(walletData.PrivateKey)

	txBytes, txid, failedTx, err := txb.BytesWithValidation()
	glb.Assertf(err == nil, "transaction invalid: %v\n------------------\n%s", err, failedTx)

	prompt := fmt.Sprintf("delegate amount %s to controller %s (plus tag-along fee %s)?", util.Th(amount), delegationTarget, util.Th(feeAmount))
	if !glb.YesNoPrompt(prompt, true) {
		glb.Infof("exit")
		os.Exit(0)
	}

	err = client.SubmitTransaction(txBytes)
	glb.AssertNoError(err)

	glb.ReportTxInclusion(txid, 2*time.Second)
}
