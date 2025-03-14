package node_cmd

import (
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/cobra"
)

func initGetOutputsCmd() *cobra.Command {
	getOutputsCmd := &cobra.Command{
		Use:   "outputs",
		Short: `returns all outputs locked in the accountable from the heaviest state of the latest epoch`,
		Args:  cobra.NoArgs,
		Run:   runGetOutputsCmd,
	}

	getOutputsCmd.InitDefaultHelpCmd()
	return getOutputsCmd
}

func runGetOutputsCmd(_ *cobra.Command, _ []string) {
	glb.InitLedgerFromNode()

	accountable := glb.MustGetTarget()

	outs, err := glb.GetClient().GetAccountParsedOutputs(accountable, 100)
	glb.AssertNoError(err)

	if outs == nil || len(outs.Outputs) == 0 {
		glb.Infof("no outputs found")
		return
	}
	lrbid, err := ledger.TransactionIDFromHexString(outs.LRBID)
	glb.AssertNoError(err)
	glb.PrintLRB(&lrbid)

	count := 0
	for id, o := range outs.Outputs {
		glb.Infof("\n-- output %d --", count)
		count++
		oid, err := ledger.OutputIDFromHexString(id)
		glb.AssertNoError(err)
		glb.Infof("   id %s, hex = %s", oid.String(), id)
		glb.Infof("   amount: %s, lock name: '%s'", util.Th(o.Amount), o.LockName)
		if o.ChainID != "" {
			glb.Verbosef("   chain id: %s", o.ChainID)
		}
		glb.Verbosef("   raw data: %s (%d bytes) ", o.Data, len(o.Data)/2)
		glb.Verbosef("   parsed constraints:")
		if glb.IsVerbose() {
			for _, constraint := range o.Constraints {
				glb.Infof("        - %s", constraint)
			}
		}
	}
	//
	//if glb.IsVerbose() {
	//	outs, lrbid, err := glb.GetClient().GetAccountOutputs(accountable)
	//	glb.AssertNoError(err)
	//
	//	glb.PrintLRB(lrbid)
	//	glb.Infof("%d outputs locked in the account %s", len(outs), accountable.String())
	//	for i, o := range outs {
	//		glb.Infof("-- output %d --", i)
	//		glb.Infof(o.String())
	//		glb.Verbosef("Raw bytes: %s", hex.EncodeToString(o.Output.Bytes()))
	//	}
	//	glb.Infof("TOTALS:")
	//	displayTotals(outs)
	//} else {
	//
	//}
}
