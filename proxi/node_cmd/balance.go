package node_cmd

import (
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/cobra"
)

var delegationsBalance bool

func initBalanceCmd() *cobra.Command {
	getBalanceCmd := &cobra.Command{
		Use:     "balance",
		Aliases: []string{"bal"},
		Short:   `displays account totals`,
		Args:    cobra.NoArgs,
		Run:     runBalanceCmd,
	}
	glb.AddFlagTarget(getBalanceCmd)
	getBalanceCmd.InitDefaultHelpCmd()
	return getBalanceCmd
}

func runBalanceCmd(_ *cobra.Command, _ []string) {
	glb.InitLedgerFromNode()
	accountable := glb.MustGetTarget()

	outs, lrbid, err := glb.GetClient().GetAccountOutputs(accountable)
	glb.AssertNoError(err)
	glb.PrintLRB(lrbid)
	displayBalanceTotals(outs, accountable)
}

type _delegation struct {
	amount     uint64
	inflation  uint64
	sinceSlot  ledger.Slot
	lastActive ledger.Slot
}

func displayBalanceTotals(outs []*ledger.OutputWithID, target ledger.Accountable) {
	var sumOnChains, sumOutsideChains, sumDelegation uint64
	var numChains, numNonChains, numDelegation int

	delegations := make(map[ledger.ChainID]_delegation)

	for _, o := range outs {
		_, ccIdx := o.Output.ChainConstraint()
		if dl := o.Output.DelegationLock(); dl != nil {
			glb.Assertf(ccIdx != 0xff, "ccIdx!=0xff")

			if !ledger.EqualAccountables(dl.OwnerLock, target) {
				// for delegation locks only count those which are owned by the target
				continue
			}
			numDelegation++
			sumDelegation += o.Output.Amount()
			delegationID, _, ok := o.ExtractChainID()
			glb.Assertf(ok, "extractChainID")
			delegations[delegationID] = _delegation{
				amount:     o.Output.Amount(),
				inflation:  o.Output.Amount() - dl.StartAmount,
				sinceSlot:  dl.StartTime.Slot(),
				lastActive: o.ID.Slot(),
			}
		}
		if ccIdx != 0xff {
			numChains++
			sumOnChains += o.Output.Amount()

		} else {
			numNonChains++
			sumOutsideChains += o.Output.Amount()
		}
	}
	glb.Infof("Total amounts controlled on:")
	glb.Infof("    %d non-chain outputs:                    %s", numNonChains, util.Th(sumOutsideChains))
	glb.Infof("    %d chain outputs (including delegation): %s", numChains, util.Th(sumOnChains))
	glb.Infof("    %d delegation outputs:                   %s", numDelegation, util.Th(sumDelegation))
	glb.Infof("-----------------\nTOTAL controlled on %d outputs: %s", numChains+numNonChains, util.Th(sumOnChains+sumOutsideChains))
	if len(delegations) == 0 {
		glb.Infof("\nNO DELEGATIONS")
		return
	}
	glb.Infof("\nDELEGATIONS:")
	ids := util.KeysSorted(delegations, func(k1, k2 ledger.ChainID) bool {
		return delegations[k1].sinceSlot < delegations[k2].sinceSlot
	})

	totalDelegated := uint64(0)
	for _, id := range ids {
		d := delegations[id]

		slots := ledger.TimeNow().Slot() - d.sinceSlot
		perSlot := d.inflation / uint64(slots)
		annualExtrapolationEarnings := uint64(ledger.L().ID.SlotsPerYear()) * perSlot
		annualRate := 100 * float64(annualExtrapolationEarnings) / float64(d.amount-d.inflation)

		glb.Infof("     %s   %20s (+%s, %.1f%% annual) since/last active slot: %d/%d",
			id.String(), util.Th(d.amount), util.Th(d.inflation), annualRate, d.sinceSlot, d.lastActive)
		totalDelegated += d.amount
	}
	glb.Infof("----------------\nTOTAL DELEGATED AMOUNT: %s", util.Th(totalDelegated))
}
