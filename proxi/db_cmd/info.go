package db_cmd

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/cobra"
)

var slotsBackDBInfo int

func initDBInfoCmd() *cobra.Command {
	dbInfoCmd := &cobra.Command{
		Use:   "info",
		Short: "displays general info of the state DB",
		Args:  cobra.NoArgs,
		Run:   runDbInfoCmd,
	}
	dbInfoCmd.PersistentFlags().IntVarP(&slotsBackDBInfo, "slots", "s", -1, "maximum slots back. Default: all")

	dbInfoCmd.InitDefaultHelpCmd()
	return dbInfoCmd
}

func runDbInfoCmd(_ *cobra.Command, _ []string) {
	glb.InitLedgerFromDB()
	defer glb.CloseDatabases()

	branchData := multistate.FetchLatestBranches(glb.StateStore())
	if len(branchData) == 0 {
		glb.Infof("no branches found")
		return
	}
	glb.Infof("Total %d branches in the latest slot %d", len(branchData), branchData[0].Stem.Timestamp().Slot)

	sort.Slice(branchData, func(i, j int) bool {
		return bytes.Compare(branchData[i].SequencerID[:], branchData[j].SequencerID[:]) < 0
	})

	reader, err := multistate.NewSugaredReadableState(glb.StateStore(), branchData[0].Root)
	glb.AssertNoError(err)

	identityYAML := reader.MustLedgerIdentityBytes()
	_, idParams, err := ledger.ParseLedgerIdYAML(identityYAML)
	glb.AssertNoError(err)

	earliestSlot := multistate.FetchEarliestSlot(glb.StateStore())
	glb.Infof("ledger time now is %s, earliest committed slot is %d", ledger.TimeNow().String(), earliestSlot)

	glb.Verbosef("\n----------------- Ledger state identity ----------------")
	glb.Verbosef("%s", idParams.String())
	glb.Infof("----------------- Global branch data ----------------------")
	DisplayBranchData(branchData)
	glb.Infof("\n------------- Supply and inflation summary -------------")
	summary := multistate.FetchSummarySupply(glb.StateStore(), slotsBackDBInfo)
	glb.Infof("%s", summary.Lines("   ").String())
}

func DisplayBranchData(branches []*multistate.BranchData) {
	for i, br := range branches {
		name := "(no name)"
		if msData := ledger.ParseMilestoneData(br.SequencerOutput.Output); msData != nil {
			name = msData.Name
		}
		name = fmt.Sprintf("%s (%s)", name, br.SequencerID.StringVeryShort())
		glb.Infof(" %2d: %s supply: %s, infl: %s, on chain: %s, coverage: %s, root: %s",
			i, name,
			util.Th(br.Supply),
			util.Th(br.SlotInflation),
			util.Th(br.SequencerOutput.Output.Amount()),
			util.Th(br.LedgerCoverage),
			br.Root.String())
	}
}
