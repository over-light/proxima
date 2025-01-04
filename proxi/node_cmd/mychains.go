package node_cmd

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var delegationOnly bool

func initChainsCmd() *cobra.Command {
	chainsCmd := &cobra.Command{
		Use:   "mychains",
		Short: `lists chains controlled by the wallet account`,
		Args:  cobra.NoArgs,
		Run:   runChainsCmd,
	}
	chainsCmd.InitDefaultHelpCmd()
	chainsCmd.PersistentFlags().BoolVarP(&delegationOnly, "delegation", "d", false, "list delegations only")
	err := viper.BindPFlag("delegation", chainsCmd.PersistentFlags().Lookup("delegation"))
	glb.AssertNoError(err)

	return chainsCmd
}

func runChainsCmd(_ *cobra.Command, _ []string) {
	glb.InitLedgerFromNode()
	wallet := glb.GetWalletData()

	outs, lrbid, err := glb.GetClient().GetChainedOutputs(wallet.Account)
	glb.AssertNoError(err)

	glb.PrintLRB(lrbid)
	if len(outs) == 0 {
		glb.Infof("no chains have been found controlled by %s", wallet.Account.String())
		os.Exit(0)
	}

	sort.Slice(outs, func(i, j int) bool {
		return outs[i].ID.Timestamp().After(outs[j].ID.Timestamp())
	})

	if delegationOnly {
		listDelegations(wallet.Account, outs)
	} else {
		listChainedOutputs(wallet.Account, outs)
	}
}

func listChainedOutputs(addr ledger.AddressED25519, outs []*ledger.OutputWithChainID) {
	glb.Infof("\nlist of %d chain(s) indexed in the account %s",
		len(outs), addr.String())
	for i, o := range outs {
		seq := "NO"
		if o.ID.IsSequencerTransaction() {
			seq = "YES"
			sd, _ := o.Output.SequencerOutputData()
			if md := sd.MilestoneData; md != nil {
				seq = fmt.Sprintf("%s (%d/%d)", md.Name, md.ChainHeight, md.BranchHeight)
			}
		}
		lock := o.Output.Lock()
		glb.Infof("\n%2d: %s -- %s, sequencer: "+seq, i, o.ChainID.String(), o.ID.StringShort())
		glb.Infof("      balance     : %s", util.Th(o.Output.Amount()))
		glb.Infof("      lock        : %s", lock.String())
		thisControls := ""
		if ledger.EqualAccountables(addr, lock.Master()) {
			thisControls = " <- wallet account controls"
		}
		switch l := lock.(type) {
		case ledger.AddressED25519:
			glb.Infof("      master      : %s"+thisControls, l.String())
		case *ledger.DelegationLock:
			delegatedToThis := ""
			if ledger.EqualAccountables(addr, l.TargetLock) {
				delegatedToThis = " <- is delegated to the wallet account"
			}
			glb.Infof("      master      : %s"+thisControls, l.OwnerLock.String())
			glb.Infof("      delegated to: %s"+delegatedToThis, l.TargetLock.String())
		}
	}
}

const yearDuration = time.Hour * 24 * 365

func listDelegations(addr ledger.AddressED25519, outs []*ledger.OutputWithChainID) {
	sort.Slice(outs, func(i, j int) bool {
		return bytes.Compare(outs[i].ChainID[:], outs[j].ChainID[:]) < 0
	})

	total := uint64(0)
	glb.Infof("\nList of delegations in account %s\n", addr.String())
	for _, o := range outs {
		dlock := o.Output.DelegationLock()
		if dlock == nil {
			continue
		}
		if !ledger.EqualAccountables(addr, dlock.Master()) {
			continue
		}
		chainID, _, _ := o.ExtractChainID()
		earned := o.Output.Amount() - dlock.StartAmount
		totalTicks := ledger.DiffTicks(ledger.TimeNow(), dlock.StartTime)
		totalDuration := time.Duration(totalTicks) * ledger.L().ID.TickDuration
		annualAmount := uint64((time.Duration(earned) * yearDuration) / totalDuration)
		annualRate := 100 * earned / annualAmount

		glb.Infof("  %37s   %21s     -->     %s   +%15s, ~%d%% annual",
			chainID.String(), util.Th(o.Output.Amount()), dlock.OwnerLock.String(), util.Th(earned), annualRate)
		total += o.Output.Amount()
	}
	glb.Infof("\nTotal delegated amount: %s", util.Th(total))
}
