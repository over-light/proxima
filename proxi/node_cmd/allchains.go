package node_cmd

import (
	"fmt"
	"sort"

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
	if groupByDelegationTarget {
		listGrouped(chains)
	} else {
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
		glb.Infof("\ntarget lock: %s", tl)
		totalForTarget := uint64(0)
		for _, o := range m[tl] {
			glb.Infof("\n      id              : %s", o.ChainID.String())
			glb.Infof("      balance         : %s", util.Th(o.Output.Amount()))
			glb.Infof("      controller lock : %s", o.Output.Lock().String())
			glb.Infof("      output          : %s", o.ID.String())
			totalForTarget += o.Output.Amount()
		}
		glb.Infof("\n------ total delegated to the target lock: %s", util.Th(totalForTarget))
		total += totalForTarget
	}
	glb.Infof("\nTOTAL delegated amount: %s", util.Th(total))
}
