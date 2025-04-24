package db_cmd

import (
	"math"
	"os"
	"strconv"

	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/spf13/cobra"
)

func initListUTXOsCmd() *cobra.Command {
	listUTXOsCmd := &cobra.Command{
		Use:   "list_utxos",
		Short: "display outputs in slot",
		Args:  cobra.ExactArgs(1),
		Run:   runListUTXOs,
	}

	listUTXOsCmd.PersistentFlags().StringVarP(&branchIDStr, "branch", "b", "", "tip branch id hex")
	listUTXOsCmd.InitDefaultHelpCmd()

	return listUTXOsCmd
}

func runListUTXOs(_ *cobra.Command, args []string) {
	slotInt, err := strconv.Atoi(args[0])
	glb.AssertNoError(err)
	glb.Assertf(slotInt < math.MaxUint32, "wrong slot number")
	slot := base.Slot(slotInt)

	glb.InitLedgerFromDB()
	defer glb.CloseDatabases()

	var branchID base.TransactionID
	var branchData *multistate.BranchData

	if branchIDStr != "" {
		branchID, err = base.TransactionIDFromHexString(branchIDStr)
		glb.AssertNoError(err)
		bd, ok := multistate.FetchBranchData(glb.StateStore(), branchID)
		if !ok {
			glb.Infof("can't find branch %s", branchIDStr)
			os.Exit(1)
		}
		branchData = &bd
	} else {
		branchData = multistate.FindLatestReliableBranch(glb.StateStore(), global.FractionHealthyBranch)
		if branchData == nil {
			glb.Infof("latest reliable branch has not been found")
			os.Exit(1)
		}
	}
	branchID = branchData.Stem.ID.TransactionID()
	glb.Infof("baseline branch is %s", branchID.String())

	rdr, err := multistate.NewReadable(glb.StateStore(), branchData.Root)
	glb.AssertNoError(err)

	var o *ledger.Output
	var err1 error
	err = rdr.IterateUTXOsInSlot(slot, func(oid base.OutputID, oData []byte) bool {
		o, err1 = ledger.OutputFromBytesReadOnly(oData)
		glb.AssertNoError(err1)
		glb.Infof("%s", oid.String())
		glb.Infof("%s", o.Lines("     "))
		return true
	})
	glb.AssertNoError(err)
}
