package db_cmd

import (
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/spf13/cobra"
)

func initScanDBCmd() *cobra.Command {
	dbScanCmd := &cobra.Command{
		Use:   "scan",
		Short: "scans multistate DB and check consistency",
		Args:  cobra.NoArgs,
		Run:   runScanDBCmd,
	}
	//dbInfoCmd.PersistentFlags().IntVarP(&slotsBackDBInfo, "slots", "s", -1, "maximum slots back. Default: all")

	dbScanCmd.InitDefaultHelpCmd()
	return dbScanCmd
}

func runScanDBCmd(_ *cobra.Command, _ []string) {
	glb.InitLedgerFromDB()
	defer glb.CloseDatabases()

	multistate.IterateSlotsBack(glb.StateStore(), func(slot base.Slot, roots []multistate.RootRecord) bool {
		branches := multistate.FetchBranchDataMulti(glb.StateStore(), roots...)
		glb.Infof("----------- slot %d: %d branches", slot, len(branches))

		for i, br := range branches {
			rdr, err := multistate.NewReadable(glb.StateStore(), br.Root)
			glb.AssertNoError(err)

			glb.Infof("%3d  %s", i, br.LinesShort().Join(", "))

			scanned := rdr.ScanState()
			if len(scanned.Inconsistencies) > 0 || scanned.Supply != br.Supply {
				glb.Infof("   inconsistencies found:\n%s", scanned.Lines("        ").String())
			}
		}
		return true
	})

}
