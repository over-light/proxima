package db_cmd

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/cobra"
)

var (
	numBuckets   uint64
	maxRoots     int
	outVrfProofs bool
)

const fname = "vrf.txt"

func initDbStatsCmd() *cobra.Command {
	dbStatsCmd := &cobra.Command{
		Use:   "stats",
		Short: "runs statistics on branches",
		Args:  cobra.NoArgs,
		Run:   runDBStatsCmd,
	}
	dbStatsCmd.PersistentFlags().Uint64VarP(&numBuckets, "buckets", "b", 10, "number of distribution buckets")
	dbStatsCmd.PersistentFlags().IntVarP(&maxRoots, "roots", "r", 2000, "max number of roots to scan")
	dbStatsCmd.PersistentFlags().BoolVarP(&outVrfProofs, "out_vrf_proof", "o", false, fmt.Sprintf("write VRF proofs to file %s (for debugging only)", fname))

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
	buckets := make([]int, numBuckets)
	numBranches := 0
	var maxBib, minBib uint64

	var fout *os.File
	var err error

	if outVrfProofs {
		fout, err = os.OpenFile(fname, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		glb.AssertNoError(err)
		defer fout.Close()
	}

	multistate.IterateSlotsBack(glb.StateStore(), func(slot base.Slot, roots []multistate.RootRecord) bool {
		for _, br := range multistate.FetchBranchDataMulti(glb.StateStore(), roots...) {
			bib := br.SequencerOutput.Output.Inflation()

			// check consistency
			stemConstraint, ok := br.Stem.Output.StemLock()
			glb.Assertf(ok, "stem lock not found in %s hex=%s", br.Stem.ID.String(), br.Stem.ID.StringHex())

			if outVrfProofs {
				_, _ = fout.WriteString(hex.EncodeToString(stemConstraint.VRFProof) + "\n")
			}

			bibCalc := ledger.L().BranchInflationBonusFromRandomnessProof(stemConstraint.VRFProof)
			glb.Assertf(bib == bibCalc, "provided vs calculated inflation mismatch %s != %s in %s",
				util.Th(bib), util.Th(bibCalc), br.Lines("        ").String())
			bibDirect := ledger.L().BranchInflationBonusDirect(stemConstraint.VRFProof)
			glb.Assertf(bib == bibDirect, "provided vs directly calculated inflation mismatch: %s != %s in %s",
				util.Th(bib), util.Th(bibDirect), br.Lines("        ").String())

			bucketNo := bib * numBuckets / maxInflation
			buckets[bucketNo]++
			maxBib = max(maxBib, bib)
			if minBib == 0 {
				minBib = bib
			} else {
				minBib = min(minBib, bib)
			}
			numBranches++
			if numBranches >= maxRoots {
				return false
			}
		}
		return true
	})
	glb.Infof("distribution of branch inflation bonus among %d branch records:\n    minimum: %s\n    maximum: %s\nBuckets:",
		numBranches, util.Th(minBib), util.Th(maxBib))

	for i, n := range buckets {
		glb.Infof("%d: %d (%.1f%%)", i, n, (float64(n)*100)/float64(numBranches))
	}
}
