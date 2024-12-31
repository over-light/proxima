package node_cmd

import (
	"os"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/cobra"
)

func initChainsCmd() *cobra.Command {
	chainsCmd := &cobra.Command{
		Use:   "chains",
		Short: `lists chains controlled by the account`,
		Args:  cobra.NoArgs,
		Run:   runChainsCmd,
	}
	chainsCmd.InitDefaultHelpCmd()
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

	listChainedOutputs(wallet.Account, outs)
}

func listChainedOutputs(addr ledger.AddressED25519, outs []*ledger.OutputWithChainID) {
	glb.Infof("list of chains in the account %s\n-------------------------", addr.String())
	var status string
	for i, o := range outs {
		lock := o.Output.Lock()
		switch lock.Name() {
		case ledger.DelegationLockName:
			if ledger.EqualAccountables(lock.Master(), addr) {
				status = "master"
			} else {
				status = "delegated"
			}
		case ledger.AddressED25519Name:
			status = "owned"
		default:
			status = "N/A"
		}
		glb.Infof("  #2%d  %10s  %10s %15s   %s", i, o.ChainID.StringShort(), status, util.Th(o.Output.Amount()), lock.String())
	}
}
