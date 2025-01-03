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
	glb.Infof("\nlist of %d chain(s) indexed in the account %s\n--------------------------------------------------------------------------",
		len(outs), addr.String())
	for i, o := range outs {
		lock := o.Output.Lock()
		glb.Infof("%2d: %s", i, o.ChainID.String())
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
