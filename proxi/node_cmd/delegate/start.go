package delegate

import (
	"strconv"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/cobra"
)

func initDelegateStartCmd() *cobra.Command {
	delegateStartCmd := &cobra.Command{
		Use:     "start <amount>",
		Aliases: util.List("send"),
		Short:   `creates delegation output`,
		Args:    cobra.ExactArgs(1),
		Run:     runDelegateStartCmd,
	}

	glb.AddFlagTarget(delegateStartCmd)

	delegateStartCmd.InitDefaultHelpCmd()
	return delegateStartCmd
}

func runDelegateStartCmd(_ *cobra.Command, args []string) {
	glb.InitLedgerFromNode()
	walletData := glb.GetWalletData()

	glb.Infof("wallet account is: %s", walletData.Account.String())
	glb.MustGetTarget()

	amountInt, err := strconv.Atoi(args[0])
	glb.AssertNoError(err)
	amount := uint64(amountInt)
	glb.Assertf(amount >= ledger.MinimumDelegationAmount(), "amount must be >= %d", ledger.MinimumDelegationAmount())

	sum := uint64(0)
	walletOutputs, lrbid, err := glb.GetClient().GetAccountOutputs(walletData.Account, func(oid *ledger.OutputID, o *ledger.Output) bool {
		sum += o.Amount()
		return
		if sum >= amount {
			return false
		}
	})
	glb.AssertNoError(err)
}
