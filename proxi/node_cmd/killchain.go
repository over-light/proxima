package node_cmd

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/ledger/txbuilder"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util/set"
	"github.com/spf13/cobra"
)

func initKillChainCmd() *cobra.Command {
	deleteChainCmd := &cobra.Command{
		Use:     "killchain <chain id>",
		Aliases: []string{"endchain, delchain"},
		Short:   `ends a chain by destroying chain output. All tokens are converted into the addressED25519-locked output with the same controlling private key`,
		Args:    cobra.ExactArgs(1),
		Run:     runKillChainCmd,
	}
	deleteChainCmd.InitDefaultHelpCmd()

	return deleteChainCmd
}

func runKillChainCmd(_ *cobra.Command, args []string) {
	//cmd.DebugFlags()
	glb.InitLedgerFromNode()

	chainID, err := ledger.ChainIDFromHexString(args[0])
	glb.AssertNoError(err)

	walletData := glb.GetWalletData()

	var tagAlongSeqID ledger.ChainID
	feeAmount := glb.GetTagAlongFee()
	glb.Assertf(feeAmount > 0, "tag-along fee is configured 0. Fee-less option not supported yet")
	clnt := glb.GetClient()

	pTagAlongSeqID := glb.GetTagAlongSequencerID()
	glb.Assertf(pTagAlongSeqID != nil, "tag-along sequencer not specified")
	tagAlongSeqID = *pTagAlongSeqID

	md, err := clnt.GetMilestoneData(tagAlongSeqID)
	glb.AssertNoError(err)

	if md != nil && md.MinimumFee > feeAmount {
		feeAmount = md.MinimumFee
	}

	prompt := fmt.Sprintf("discontinue chain %s?", chainID.String())
	if !glb.YesNoPrompt(prompt, true, glb.BypassYesNoPrompt()) {
		glb.Infof("exit")
		os.Exit(0)
	}

	var ts ledger.Time
	var chainIN *ledger.OutputWithChainID

	for {
		chainIN, _, _, err = clnt.GetChainOutput(chainID)
		glb.AssertNoError(err)

		ts = ledger.MaximumTime(ledger.TimeNow(), chainIN.Timestamp().AddTicks(int(ledger.L().ID.TransactionPace)))
		if ts.IsSlotBoundary() {
			ts.AddTicks(1)
		}
		closedDelegationSlot := ledger.NextClosedDelegationSlot(chainID, ts.Slot())
		if closedDelegationSlot != ts.Slot() {
			ts = ledger.NewLedgerTime(closedDelegationSlot, 1)
		}
		if ts.Slot() <= ledger.TimeNow().Slot() {
			break
		}
		glb.Infof("until suitable time window left %d ticks = ~%0.2f seconds",
			ledger.DiffTicks(ts, ledger.TimeNow()), float64(time.Until(ts.Time()))/float64(time.Second))
		time.Sleep(2 * time.Second)
	}

	tx, err := txbuilder.MakeEndChainTransaction(txbuilder.EndChainParams{
		Timestamp:     ts,
		ChainIn:       chainIN,
		PrivateKey:    walletData.PrivateKey,
		TagAlongSeqID: tagAlongSeqID,
		TagAlongFee:   feeAmount,
	})
	glb.AssertNoError(err)

	err = clnt.SubmitTransaction(tx.Bytes())
	glb.AssertNoError(err)

	glb.Infof("submitted transaction %s", tx.IDString())
	glb.Verbosef("-------------- transaction --------------\n%s", tx.String())
	err = clnt.SubmitTransaction(tx.Bytes())
	glb.AssertNoError(err)

	if ledger.TimeNow().Slot() < tx.Slot() {
		waitUntil := tx.TimestampTime()
		leftWaiting := time.Until(waitUntil)
		glb.Infof("transaction delayed for %d ticks = ~%0.2f seconds",
			ledger.DiffTicks(tx.Timestamp(), ledger.TimeNow()), float64(leftWaiting)/float64(time.Second))
		glb.Infof("sleeping for %v", leftWaiting)
		time.Sleep(leftWaiting)
	}
	glb.ReportTxInclusion(tx.ID(), 2*time.Second)
}

func checkChainLoop(chainID ledger.ChainID, privateKey ed25519.PrivateKey, repeatPeriod time.Duration, ctx context.Context) bool {
	clnt := glb.GetClient()

	for {
		chainIN, _, _, err := clnt.GetChainOutput(chainID)
		if errors.Is(err, multistate.ErrNotFound) {
			glb.Infof("[check] chain %s has been destroyed", chainID.StringShort())
			return true
		}
		glb.AssertNoError(err)

		tx, err := txbuilder.MakeEndChainTransaction(txbuilder.EndChainParams{
			Timestamp:     ts,
			ChainIn:       chainIN,
			PrivateKey:    privateKey,
			TagAlongSeqID: tagAlongSeqID,
			TagAlongFee:   feeAmount,
		})
		glb.AssertNoError(err)

		select {
		case <-ctx.Done():
			glb.Infof("[check] exit")
			return false
		case <-time.After(repeatPeriod):
		}
	}
}

func makeTransactionLoop(chainID ledger.ChainID, repeatPeriod time.Duration, ctx context.Context) {
	clnt := glb.GetClient()
	consumedOutputs := set.New[ledger.OutputID]()

	for {
		o, constrIdx, lrbid, err := clnt.GetChainOutput(chainID)
		if errors.Is(err, multistate.ErrNotFound) {
			glb.Infof("[maketx] chain %s has been destroyed", chainID.StringShort())
			return
		}
		glb.AssertNoError(err)
		if ledger.TimeNow().Slot()-lrbid.Slot() > 2 {
			glb.Infof("[maketx] no sync. Exit")
			return
		}
		if consumedOutputs.Contains(o.ID) {
			continue
		}

		select {
		case <-ctx.Done():
			glb.Infof("[maketx] exit")
			return
		case <-time.After(repeatPeriod):
		}
	}

}
