package node_cmd

import (
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util/lines"
	"github.com/spf13/cobra"
)

func initNodeInfoCmd() *cobra.Command {
	getNodeInfoCmd := &cobra.Command{
		Use:   "info",
		Short: `retrieves node info from the node`,
		Args:  cobra.NoArgs,
		Run:   runNodeInfoCmd,
	}

	getNodeInfoCmd.InitDefaultHelpCmd()
	return getNodeInfoCmd
}

func runNodeInfoCmd(_ *cobra.Command, _ []string) {
	glb.InitLedgerFromNode()

	nodeInfo, err := glb.GetClient().GetNodeInfo()
	glb.AssertNoError(err)
	glb.Infof("\nNode:")
	glb.Infof(nodeInfo.Lines("    ").String())

	rootRecord, branchID, err := glb.GetClient().GetLatestReliableBranch()
	glb.AssertNoError(err)
	glb.Infof("\nLatest reliable branch (LRB):")

	ln := lines.New("    ")
	ln.Add("branch id: %s", branchID.String()).
		Add("root record:").
		Append(rootRecord.Lines("    "))
	glb.Infof(ln.String())

	glb.Infof("\nLedger id (ledger constants):")
	glb.Infof(ledger.L().ID.Lines("    ").String())
}
