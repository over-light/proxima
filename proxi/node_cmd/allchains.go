package node_cmd

import (
	"fmt"
	"sort"

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

	glb.PrintLRB(lrbid)
	sort.Slice(chains, func(i, j int) bool {
		return chains[i].ID.Timestamp().After(chains[j].ID.Timestamp())
	})
	listChains(chains)
}

func listChains(chains []*ledger.OutputWithChainID) {
	glb.Infof("\nlist of all chains (%d)", len(chains))

	for i, o := range chains {
		lock := o.Output.Lock()
		seq := "NO"
		if o.ID.IsSequencerTransaction() {
			seq = "YES"
			sd, _ := o.Output.SequencerOutputData()
			if md := sd.MilestoneData; md != nil {
				seq = fmt.Sprintf("%s (%d/%d)", md.Name, md.ChainHeight, md.BranchHeight)
			}
		}
		glb.Infof("\n%2d: %s, sequencer: "+seq, i, o.ChainID.String())
		glb.Infof("      balance         : %s", util.Th(o.Output.Amount()))
		glb.Infof("      controller lock : %s", lock.String())
		glb.Infof("      output          : %s", o.ID.StringShort())
	}
}
