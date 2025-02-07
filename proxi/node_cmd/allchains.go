package node_cmd

import (
	"fmt"
	"sort"
	"strings"

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
	chains, lrbid, err := glb.GetClient().GetAllChains()
	glb.AssertNoError(err)

	glb.PrintLRB(lrbid)
	sort.Slice(chains, func(i, j int) bool {
		return chains[i].ID.Timestamp().After(chains[j].ID.Timestamp())
	})
	if showSequencersOnly {
		chains = util.PurgeSlice(chains, func(o *ledger.OutputWithChainID) bool {
			return o.ID.IsSequencerTransaction()
		})
	}
	switch {
	case groupByDelegationTarget:
		listGrouped(chains)
	case showDelegationsOnly && showSequencersOnly:
		listSequencerDelegationInfo(chains)
	default:
		listChains(chains)
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

func listSequencerDelegationInfo(chains []*ledger.OutputWithChainID) {
	m := make(map[ledger.ChainID]_seqDelegationInfo)
	// collect all sequencers
	for _, o := range chains {
		sd, _ := o.Output.SequencerOutputData()
		if sd == nil {
			continue
		}
		var name string
		if md := sd.MilestoneData; md != nil {
			name, _, _ = strings.Cut(md.Name, ".")
		}
		m[sd.ChainConstraint.ID] = _seqDelegationInfo{
			lastActive: o.ID.Slot(),
			name:       name,
		}
	}

	// collect all sequencers with active delegations
	for _, o := range chains {
		dl := o.Output.DelegationLock()
		if dl == nil {
			continue
		}
		if dl.TargetLock.Name() != ledger.ChainLockName {
			continue
		}
		cl, ok := dl.TargetLock.(ledger.ChainLock)
		glb.Assertf(ok, "target lock %s is not a chain lock:\n%s", dl.TargetLock.String(), o.String())

		chainID := cl.ChainID()

		seqData := m[chainID]
		seqData.numDelegations++
		seqData.delegatedAmount += o.Output.Amount()
		if o.ID.Slot() > seqData.lastActive {
			seqData.lastActive = o.ID.Slot()
		}
		m[chainID] = seqData
	}

	keys := util.KeysSorted(m, func(k1, k2 ledger.ChainID) bool {
		return m[k1].numDelegations > m[k2].numDelegations
	})

	glb.Infof("\nSequencers with delegation totals:")
	totalDelegated := uint64(0)
	totalDelegations := 0

	for _, seqID := range keys {
		seqData := m[seqID]
		glb.Infof("   %s %8s   # delegations: %3d,    total delegated amount: %20s,    last active: %d slots ago",
			seqID.String(), seqData.name, seqData.numDelegations, util.Th(seqData.delegatedAmount), ledger.TimeNow().Slot()-seqData.lastActive)
		totalDelegated += seqData.delegatedAmount
		totalDelegations += seqData.numDelegations
	}
	glb.Infof("---------------\nTOTAL DELEGATIONS     :  %d", totalDelegations)
	glb.Infof("TOTAL DELEGATED AMOUNT:  %s", util.Th(totalDelegated))
}
