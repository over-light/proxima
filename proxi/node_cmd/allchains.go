package node_cmd

import (
	"fmt"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	showSequencersOnly      bool
	showDelegationsOnly     bool
	groupByDelegationTarget bool
)

func initAllChainsCmd() *cobra.Command {
	allChainsCmd := &cobra.Command{
		Use:   "allchains",
		Short: `lists all chains in the latest reliable branch`,
		Args:  cobra.NoArgs,
		Run:   runAllChainsCmd,
	}
	allChainsCmd.InitDefaultHelpCmd()

	allChainsCmd.PersistentFlags().BoolVarP(&showSequencersOnly, "seq", "q", false, "show sequencer chains only")
	err := viper.BindPFlag("seq", allChainsCmd.PersistentFlags().Lookup("seq"))
	glb.AssertNoError(err)

	allChainsCmd.PersistentFlags().BoolVarP(&showDelegationsOnly, "delegations", "d", false, "show delegation chains only")
	err = viper.BindPFlag("delegations", allChainsCmd.PersistentFlags().Lookup("delegations"))
	glb.AssertNoError(err)

	allChainsCmd.PersistentFlags().BoolVarP(&groupByDelegationTarget, "group", "g", false, "show all delegations grouped by delegation target")
	err = viper.BindPFlag("group", allChainsCmd.PersistentFlags().Lookup("group"))
	glb.AssertNoError(err)

	return allChainsCmd
}

func runAllChainsCmd(_ *cobra.Command, _ []string) {
	glb.InitLedgerFromNode()

	clnt := glb.GetClient()
	rr, lrbid, err := clnt.GetLatestReliableBranch()
	glb.AssertNoError(err)

	glb.PrintLRB(&lrbid)

	if showDelegationsOnly && showSequencersOnly {
		listSequencerDelegationInfo(rr.Supply)
	} else {
		chains, _, err := clnt.GetAllChains()
		glb.AssertNoError(err)
		if groupByDelegationTarget {
			listGrouped(chains)
		} else {
			listChains(chains)
		}
	}
}

func listChains(chains []*ledger.OutputWithChainID) {
	glb.Infof("\nshow sequencers only = %v", showSequencersOnly)
	glb.Infof("show delegations only = %v", showDelegationsOnly)
	glb.Infof("------------------------------")

	count := 0
	for i, o := range chains {
		lock := o.Output.Lock()
		seq := "NO"
		sd, _ := o.Output.SequencerOutputData()
		if sd != nil {
			if showDelegationsOnly {
				continue
			}
			seq = "YES"
			if md := sd.MilestoneData; md != nil {
				seq = fmt.Sprintf("%s (%d/%d)", md.Name, md.ChainHeight, md.BranchHeight)
			}
		}

		if o.Output.Lock().Name() == ledger.DelegationLockName {
			if showSequencersOnly {
				continue
			}
		}

		glb.Infof("\n%2d: %s, sequencer: "+seq, i, o.ChainID.String())
		glb.Infof("      balance         : %s", util.Th(o.Output.Amount()))
		glb.Infof("      controller lock : %s", lock.String())
		glb.Infof("      output          : %s", o.ID.String())
		count++
	}
	glb.Infof("\ntotal %d chains", count)
}

func listGrouped(chains []*ledger.OutputWithChainID) {
	glb.Infof("\ndelegations grouped by target lock")
	m := make(map[string][]*ledger.OutputWithChainID)

	total := uint64(0)
	count := 0
	for _, o := range chains {
		lock := o.Output.Lock()
		if lock.Name() != ledger.DelegationLockName {
			continue
		}
		dl := lock.(*ledger.DelegationLock)
		dls := dl.TargetLock.String()
		lst := m[dls]
		if len(lst) == 0 {
			lst = make([]*ledger.OutputWithChainID, 0)
		}
		lst = append(lst, o)
		m[dls] = lst
	}

	for tl := range m {
		glb.Infof("\ntarget lock: %s, total delegations: %d", tl, len(m[tl]))
		totalForTarget := uint64(0)
		for _, o := range m[tl] {
			glb.Infof("\n      id              : %s", o.ChainID.String())
			glb.Infof("      balance         : %s", util.Th(o.Output.Amount()))
			glb.Infof("      controller lock : %s", o.Output.Lock().String())
			glb.Infof("      output          : %s", o.ID.String())
			totalForTarget += o.Output.Amount()
			count++
		}
		glb.Infof("\n------ Amount delegated to the target lock: %s", util.Th(totalForTarget))
		total += totalForTarget
	}
	glb.Infof("\nTOTAL delegations: %d, delegated amount: %s", count, util.Th(total))
}

type _seqDelegationInfo struct {
	name            string
	numDelegations  int
	delegatedAmount uint64
	lastActive      ledger.Slot
}

func listSequencerDelegationInfo(supply uint64) {
	bySeq, _, err := glb.GetClient().GetDelegationsBySequencer()
	glb.AssertNoError(err)

	keys := util.KeysSorted(bySeq, func(k1, k2 string) bool {
		switch {
		case len(bySeq[k1].Delegations) > len(bySeq[k2].Delegations):
			return true
		case len(bySeq[k1].Delegations) == len(bySeq[k2].Delegations):
			return bySeq[k1].SequencerName < bySeq[k2].SequencerName
		}
		return false
	})

	glb.Infof("\nSequencers with delegation totals:")
	totalDelegated := uint64(0)
	totalDelegations := 0

	for i, seqIDHex := range keys {
		seqData := bySeq[seqIDHex]

		delegatedAmount := uint64(0)
		for _, dd := range seqData.Delegations {
			delegatedAmount += dd.Amount
		}
		doid, err := ledger.OutputIDFromHexString(seqData.SequencerOutputID)
		glb.AssertNoError(err)
		glb.Infof("%2d.   %s %8s   delegated: %20s (%2d outputs),    last active: %d slots ago",
			i, seqIDHex, seqData.SequencerName, util.Th(delegatedAmount), len(seqData.Delegations), ledger.TimeNow().Slot()-doid.Slot())
		totalDelegated += delegatedAmount
		totalDelegations += len(seqData.Delegations)
	}

	glb.Infof("---------------")
	glb.Infof("TOTAL SUPPLY          :  %s", util.Th(supply))
	glb.Infof("TOTAL DELEGATIONS     :  %d", totalDelegations)
	glb.Infof("TOTAL DELEGATED AMOUNT:  %s (%.2f%% of supply)", util.Th(totalDelegated), 100*float64(totalDelegated)/float64(supply))
}
