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
	"github.com/spf13/viper"
)

var targetChainIDStr string

func initDelegateCmd() *cobra.Command {
	delegateCmd := &cobra.Command{
		Use:     "delegate <amount> -q <target sequencer ID hex encoded>",
		Aliases: util.List("send"),
		Short:   `delegates amount to target sequencer by creating delegation chain output`,
		Args:    cobra.ExactArgs(1),
		Run:     runDelegateCmd,
	}

	glb.AddFlagTarget(delegateCmd)

	delegateCmd.PersistentFlags().StringVarP(&targetChainIDStr, "seq", "q", "", "target sequencer ID")
	err := viper.BindPFlag("seq", delegateCmd.PersistentFlags().Lookup("seq"))
	glb.AssertNoError(err)

	delegateCmd.InitDefaultHelpCmd()
	return delegateCmd
}

func runDelegateCmd(_ *cobra.Command, args []string) {
	glb.InitLedgerFromNode()
	walletData := glb.GetWalletData()

	glb.Infof("wallet account is: %s", walletData.Account.String())

	glb.Assertf(targetChainIDStr != "", "target sequencer ID not specified")

	targetSeqID, err := ledger.ChainIDFromHexString(targetChainIDStr)
	glb.Assertf(err == nil, "failed parsing target chainID: %v", err)

	seqOut, _, _, err := glb.GetClient().GetChainOutput(targetSeqID)
	glb.Assertf(err == nil, "can't find sequencer ID %s: %v", targetSeqID.StringShort(), err)
	glb.Assertf(seqOut.ID.IsSequencerTransaction(), "chainID %s does not represent a sequencer", targetSeqID.StringShort())

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
	walletOutputs, lrbid, err := client.GetSimpleSigLockedOutputs(walletData.Account, 100, "asc")
	glb.AssertNoError(err)
	glb.PrintLRB(lrbid)

	sumIn := uint64(0)
	walletOutputs = util.PurgeSlice(walletOutputs, func(o *ledger.OutputWithID) bool {
		if sumIn >= amount+feeAmount {
			return false
		}
		sumIn += o.Output.Amount()
		return true
	})
	glb.Assertf(sumIn >= amount+feeAmount, "not enough tokens. Needed %s, got %s", util.Th(amount+feeAmount), util.Th(sumIn))

	txb := txbuilder.New()
	_, inTs, err := txb.ConsumeOutputs(walletOutputs...)
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
		o.WithLock(ledger.NewDelegationLock(walletData.Account, targetSeqID.AsChainLock(), 2, ts, amount))
		_, _ = o.PushConstraint(ledger.NewChainOrigin().Bytes())
	})
	delegationOutputIdx, _ := txb.ProduceOutput(outDelegation)

	outTagAlong := ledger.NewOutput(func(o *ledger.Output) {
		o.WithAmount(feeAmount)
		o.WithLock(tagAlongSeqID.AsChainLock())
	})
	_, _ = txb.ProduceOutput(outTagAlong)

	totalAmountConsumed := txb.ConsumedAmount()
	totalAmountProduced, _ := txb.ProducedAmount()

	if totalAmountConsumed > totalAmountProduced {
		remainderOut := ledger.NewOutput(func(o *ledger.Output) {
			o.WithAmount(totalAmountConsumed - totalAmountProduced)
			o.WithLock(walletData.Account)
		})
		_, _ = txb.ProduceOutput(remainderOut)
	}

	totalAmountProduced, _ = txb.ProducedAmount()
	glb.Assertf(totalAmountConsumed == totalAmountProduced, "totalAmountConsumed==totalAmountProduced")

	txb.TransactionData.Timestamp = ts
	txb.TransactionData.InputCommitment = txb.InputCommitment()
	txb.SignED25519(walletData.PrivateKey)

	txBytes, txid, failedTx, err := txb.BytesWithValidation()
	glb.Assertf(err == nil, "transaction invalid: %v\n------------------\n%s", err, failedTx)

	prompt := fmt.Sprintf("delegate amount %s to sequencer %s (plus tag-along fee %s)?",
		util.Th(amount), targetSeqID.String(), util.Th(feeAmount))

	if !glb.YesNoPrompt(prompt, true) {
		glb.Infof("exit")
		os.Exit(0)
	}

	delegationOid, err := ledger.NewOutputID(&txid, delegationOutputIdx)
	glb.AssertNoError(err)

	delegationID := ledger.MakeOriginChainID(&delegationOid)
	glb.Infof("\ndelegation ID: %s\n", delegationID.String())

	err = client.SubmitTransaction(txBytes)
	glb.AssertNoError(err)

	glb.ReportTxInclusion(txid, 2*time.Second)
}
