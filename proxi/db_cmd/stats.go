package db_cmd

import (
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/cobra"
)

func initDbStatsCmd() *cobra.Command {
	dbStatsCmd := &cobra.Command{
		Use:   "stats",
		Short: "runs statistics on branches",
		Args:  cobra.NoArgs,
		Run:   runDBStatsCmd,
	}
	//dbInfoCmd.PersistentFlags().IntVarP(&slotsBackDBInfo, "slots", "s", -1, "maximum slots back. Default: all")

	dbStatsCmd.InitDefaultHelpCmd()
	return dbStatsCmd
}

func runDBStatsCmd(_ *cobra.Command, _ []string) {
	glb.InitLedgerFromDB()
	defer glb.CloseDatabases()

	runBranchInflationBousStats()
}

func runBranchInflationBousStats() {
	maxInflation := ledger.L().BranchInflationBonusBase()
	const numBuckets = 100
	buckets := make([]int, numBuckets)
	numBranches := 0
	var maxBib, minBin uint64

	multistate.IterateSlotsBack(glb.StateStore(), func(slot base.Slot, roots []multistate.RootRecord) bool {
		for _, br := range multistate.FetchBranchDataMulti(glb.StateStore(), roots...) {
			bib := br.SequencerOutput.Output.Inflation()
			bucketNo := bib * numBuckets / maxInflation
			buckets[bucketNo]++
			maxBib = max(maxBib, bib)
			if minBin == 0 {
				maxBib = bib
			} else {
				minBin = min(minBin, bib)
			}
			numBranches++
		}
		return true
	})
	glb.Infof("distribution of branch inflation bonus among %d branch records:\n    minimum: %s\n    maximum: %s\nBuckets:",
		numBranches, util.Th(minBin), util.Th(maxBib))

	for i, n := range buckets {
		glb.Infof("%d: %d (%2f%%)", i, n, float64(n)/float64(len(buckets))*100)
	}
}
