package db_cmd

import (
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/cobra"
)

var (
	_numBuckets uint64
	_maxRoots   int
)

func initDbChainStatsCmd() *cobra.Command {
	dbStatsCmd := &cobra.Command{
		Use:   "chainstats",
		Short: "runs statistics on branches in the main chain",
		Args:  cobra.NoArgs,
		Run:   runDBChainStatsCmd,
	}
	dbStatsCmd.PersistentFlags().Uint64VarP(&_numBuckets, "buckets", "b", 10, "number of distribution buckets")
	dbStatsCmd.PersistentFlags().IntVarP(&_maxRoots, "roots", "r", 2000, "max number of roots to scan")

	dbStatsCmd.InitDefaultHelpCmd()
	return dbStatsCmd
}

func runDBChainStatsCmd(_ *cobra.Command, _ []string) {
	glb.InitLedgerFromDB()
	defer glb.CloseDatabases()

	runChainStats()
}

type seqStats struct {
	numBranches  int
	sumInflation uint64
	minBalance   uint64
	maxBalance   uint64
}

func runChainStats() {
	maxInflation := ledger.L().BranchInflationBonusBase()
	buckets := make([]int, _numBuckets)
	sequencers := make(map[base.ChainID]*seqStats)
	numBranches := 0
	var maxBib, minBib uint64

	lrb := multistate.FindLatestReliableBranch(glb.StateStore(), global.FractionHealthyBranch)
	glb.Assertf(lrb != nil, "no latest reliable branch found")

	multistate.IterateBranchChainBack(glb.StateStore(), lrb, func(branchID *base.TransactionID, br *multistate.BranchData) bool {
		bib := br.SequencerOutput.Output.Inflation()
		numBranches++
		if bib == 0 {
			return true
		}

		// check consistency
		stemConstraint, ok := br.Stem.Output.StemLock()
		glb.Assertf(ok, "stem lock not found in %s hex=%s", br.Stem.ID.String(), br.Stem.ID.StringHex())

		bibCalc := ledger.L().BranchInflationBonusFromRandomnessProof(stemConstraint.VRFProof)
		glb.Assertf(bib == bibCalc, "provided vs calculated inflation mismatch %s != %s in %s",
			util.Th(bib), util.Th(bibCalc), br.Lines("        ").String())
		bibDirect := ledger.L().BranchInflationBonusDirect(stemConstraint.VRFProof)
		glb.Assertf(bib == bibDirect, "provided vs directly calculated inflation mismatch: %s != %s in %s",
			util.Th(bib), util.Th(bibDirect), br.Lines("        ").String())
		bucketNo := bib * _numBuckets / maxInflation
		buckets[bucketNo]++
		maxBib = max(maxBib, bib)
		if minBib == 0 {
			minBib = bib
		} else {
			minBib = min(minBib, bib)
		}

		seqID, _, ok := br.SequencerOutput.ExtractChainID()
		glb.Assertf(ok, "failed to extract chain id from %s", br.Lines("        ").String())

		seqStatsRec, ok := sequencers[seqID]
		if !ok {
			seqStatsRec = &seqStats{}
			sequencers[seqID] = seqStatsRec
		}
		seqStatsRec.numBranches++
		seqStatsRec.sumInflation += bib
		if seqStatsRec.minBalance == 0 {
			seqStatsRec.minBalance = br.SequencerOutput.Output.Amount()
		} else {
			seqStatsRec.minBalance = min(br.SequencerOutput.Output.Amount(), seqStatsRec.minBalance)
		}
		seqStatsRec.maxBalance = max(br.SequencerOutput.Output.Amount(), seqStatsRec.maxBalance)

		if numBranches >= _maxRoots {
			return false
		}
		return true
	})
	glb.Infof("\ndistribution of branch inflation bonus among %d branch records in the main chain:\n    minimum: %s\n    maximum: %s\nBy buckets:",
		numBranches, util.Th(minBib), util.Th(maxBib))

	for i, n := range buckets {
		glb.Infof("%d: %d (%.1f%%)", i, n, (float64(n)*100)/float64(numBranches))
	}
	glb.Infof("\nstats by sequencer:")
	seqIDs := util.KeysSorted(sequencers, func(id1, id2 base.ChainID) bool {
		return sequencers[id1].numBranches > sequencers[id2].numBranches
	})

	for _, seqID := range seqIDs {
		seqStatsRec := sequencers[seqID]
		glb.Infof("   %s  %6d (%.1f%%)  avg BIB: %s, balance: %s - %s",
			seqID.String(),
			seqStatsRec.numBranches,
			float64(seqStatsRec.numBranches)*100/float64(numBranches),
			util.Th(seqStatsRec.sumInflation/uint64(seqStatsRec.numBranches)),
			util.Th(seqStatsRec.minBalance),
			util.Th(seqStatsRec.maxBalance),
		)
	}
}
