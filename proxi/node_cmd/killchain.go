package node_cmd

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"sync"
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

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	wg.Add(2)
	go func() {
		if checkChainLoop(chainID, 2*time.Second, ctx) {
			glb.Infof("success")
		} else {
			glb.Infof("failed")
		}
		cancel()
		wg.Done()
	}()

	go func() {
		makeTransactionLoop(killChainParams{
			chainID:       chainID,
			privateKey:    walletData.PrivateKey,
			tagAlongSeqID: tagAlongSeqID,
			tagAlongFee:   feeAmount,
			repeatPeriod:  2 * time.Second,
			ctx:           ctx,
		})
		wg.Done()
	}()
	wg.Wait()
}

type killChainParams struct {
	chainID       ledger.ChainID
	privateKey    ed25519.PrivateKey
	tagAlongSeqID ledger.ChainID
	tagAlongFee   uint64
	repeatPeriod  time.Duration
	ctx           context.Context
}

func checkChainLoop(chainID ledger.ChainID, repeatPeriod time.Duration, ctx context.Context) bool {
	clnt := glb.GetClient()

	for {
		_, _, _, err := clnt.GetChainOutput(chainID)
		if errors.Is(err, multistate.ErrNotFound) {
			glb.Infof("[check] chain %s has been converted into ordinary output", chainID.StringShort())
			return true
		}
		glb.AssertNoError(err)

		select {
		case <-ctx.Done():
			glb.Infof("[check] exit")
			return false
		case <-time.After(repeatPeriod):
		}
	}
}

func makeTransactionLoop(par killChainParams) {
	clnt := glb.GetClient()
	consumedOutputs := set.New[ledger.OutputID]()

	for {
		o, _, lrbid, err := clnt.GetChainOutput(par.chainID)
		if errors.Is(err, multistate.ErrNotFound) {
			glb.Infof("[maketx] chain %s has been destroyed", par.chainID.StringShort())
			return
		}
		glb.AssertNoError(err)
		if ledger.TimeNow().Slot()-lrbid.Slot() > 2 {
			glb.Infof("[maketx] LRB is %d slots behind. Exit", ledger.TimeNow().Slot()-lrbid.Slot())
			return
		}
		if !consumedOutputs.Contains(o.ID) {
			ts := ledger.NextClosedDelegationTimestamp(par.chainID, o.Timestamp())

			tx, err := txbuilder.MakeEndChainTransaction(txbuilder.EndChainParams{
				Timestamp:     ts,
				ChainIn:       o,
				PrivateKey:    par.privateKey,
				TagAlongSeqID: par.tagAlongSeqID,
				TagAlongFee:   par.tagAlongFee,
			})
			glb.AssertNoError(err)

			err = clnt.SubmitTransaction(tx.Bytes())
			glb.AssertNoError(err)

			err = clnt.SubmitTransaction(tx.Bytes())
			glb.AssertNoError(err)

			glb.Infof("[maketx] submitted transaction %s. Ticks relative to now: %d", tx.IDString(), ledger.DiffTicks(tx.Timestamp(), ledger.TimeNow()))
			glb.Verbosef("-------------- transaction --------------\n%s", tx.String())

			consumedOutputs.Insert(o.ID)
		}

		select {
		case <-par.ctx.Done():
			glb.Infof("[maketx] exit")
			return
		case <-time.After(par.repeatPeriod):
		}
	}

}
