package node_cmd

import (
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/cobra"
)

func initAllChainsCmd() *cobra.Command {
	chainsCmd := &cobra.Command{
		Use:   "allchains",
		Short: `lists all chains in the latest reliable branch`,
		Args:  cobra.NoArgs,
		Run:   runAllChainsCmd,
	}
	chainsCmd.InitDefaultHelpCmd()
	return chainsCmd
}

func runAllChainsCmd(_ *cobra.Command, _ []string) {
	glb.InitLedgerFromNode()
	chains, lrbid, err := glb.GetClient().GetAllChains()
	glb.AssertNoError(err)

	listChains(chains, lrbid)
}

func listChains(chains []*ledger.OutputWithChainID, lrbid *ledger.TransactionID) {
	glb.Infof("\nlist of all chains (%d) in the LRB %s\n--------------------------------------------------------------------------",
		len(chains), lrbid.String())

	for i, o := range chains {
		lock := o.Output.Lock()
		glb.Infof("%2d: %s", i, o.ChainID.String())
		glb.Infof("      balance         : %s", util.Th(o.Output.Amount()))
		glb.Infof("      controller lock : %s", lock.String())
		glb.Infof("      output          : %s", o.ID.StringShort())
	}
}
