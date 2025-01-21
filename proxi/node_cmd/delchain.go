package node_cmd

import (
	"os"
	"time"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/txbuilder"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/cobra"
)

func initDeleteChainCmd() *cobra.Command {
	deleteChainCmd := &cobra.Command{
		Use:   "delchain <chain id>",
		Short: `deletes a chain origin (not a sequencer)`,
		Args:  cobra.ExactArgs(1),
		Run:   runDeleteChainCmd,
	}
	deleteChainCmd.InitDefaultHelpCmd()

	return deleteChainCmd
}

func runDeleteChainCmd(_ *cobra.Command, args []string) {
	//cmd.DebugFlags()
	glb.InitLedgerFromNode()

	chainID, err := ledger.ChainIDFromHexString(args[0])
	glb.AssertNoError(err)

	walletData := glb.GetWalletData()
	target := glb.MustGetTarget()

	var tagAlongSeqID ledger.ChainID
	feeAmount := glb.GetTagAlongFee()
	glb.Assertf(feeAmount > 0, "tag-along fee is configured 0. Fee-less option not supported yet")
	clnt := glb.GetClient()

	if feeAmount > 0 {
		pTagAlongSeqID := glb.GetTagAlongSequencerID()
		glb.Assertf(pTagAlongSeqID != nil, "tag-along sequencer not specified")
		tagAlongSeqID = *pTagAlongSeqID

		md, err := clnt.GetMilestoneData(tagAlongSeqID)
		glb.AssertNoError(err)

		if md != nil && md.MinimumFee > feeAmount {
			feeAmount = md.MinimumFee
		}
	}
	chainIN, _, _, err := clnt.GetChainOutput(chainID)
	glb.AssertNoError(err)

	glb.Infof("deleting chain:")
	glb.Infof("   chain id: %s", chainID.String())
	glb.Infof("   chain output: %s", chainIN.ID.String())
	glb.Infof("   chain controller: %s", target)
	glb.Infof("   tag-along fee %s to the sequencer %s", util.Th(feeAmount), tagAlongSeqID.String())
	glb.Infof("   source account: %s", walletData.Account.String())

	if !glb.YesNoPrompt("proceed?:", true, glb.BypassYesNoPrompt()) {
		glb.Infof("exit")
		os.Exit(0)
	}

	var txBytes []byte
	var txid ledger.TransactionID

	for {
		txBytes, txid, err = txbuilder.MakeDeleteChainTransaction(txbuilder.DeleteChainParams{
			ChainIn:                       chainIN,
			PrivateKey:                    walletData.PrivateKey,
			TagAlongSeqID:                 chainID,
			TagAlongFee:                   feeAmount,
			EnforceNoDelegationTransition: true,
		})
		glb.AssertNoError(err)

		leftUntilDelegationClosedSlot := int(ledger.TimeNow().Slot() - txid.Slot())
		if leftUntilDelegationClosedSlot <= 1 {
			glb.Infof("submitting transaction %s", txid.String())
			err = clnt.SubmitTransaction(txBytes)
			glb.AssertNoError(err)
			glb.ReportTxInclusion(txid, 2*time.Second)
			return
		}
		glb.Infof("waiting for the slot which is closed for delegation: ~%v..", time.Duration(leftUntilDelegationClosedSlot)*ledger.L().ID.SlotDuration())
		time.Sleep(ledger.L().ID.SlotDuration())
	}
}
