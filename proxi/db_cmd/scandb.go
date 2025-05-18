package db_cmd

import (
	"bytes"
	"sort"

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

	branchData := multistate.FetchLatestBranches(glb.StateStore())
	if len(branchData) == 0 {
		glb.Infof("no branches found")
		return
	}
	glb.Infof("Total %d branches in the latest slot %d", len(branchData), branchData[0].Stem.Timestamp().Slot)

	// for determinism
	sort.Slice(branchData, func(i, j int) bool {
		return bytes.Compare(branchData[i].SequencerID[:], branchData[j].SequencerID[:]) < 0
	})

	for i, br := range branchData {
		rdr, err := multistate.NewReadable(glb.StateStore(), branchData[0].Root)
		glb.AssertNoError(err)

		glb.Infof("%3d  %s", i, br.LinesShort().Join(", "))
		scanned := rdr.ScanState()
		glb.Infof("   scanned: %d", scanned.Lines("    ").String())
	}
}
