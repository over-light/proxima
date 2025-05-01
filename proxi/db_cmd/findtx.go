package db_cmd

import (
	"strings"

	"github.com/lunfardo314/proxima/core/core_module/branches"
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/unitrie/common"
	"github.com/spf13/cobra"
)

func initFindTxCmd() *cobra.Command {
	findTxCmd := &cobra.Command{
		Use:   "findtx",
		Short: "finds transaction IDs in slot and/or with hex fragment",
		Args:  cobra.NoArgs,
		Run:   runFindTxCmd,
	}

	findTxCmd.PersistentFlags().Uint32VarP(&findInSlot, "slot", "s", 0, "slot prefix")
	findTxCmd.PersistentFlags().StringVarP(&findWithHexFragment, "hex_fragment", "x", "", "hex fragment")
	findTxCmd.PersistentFlags().BoolVarP(&findFirst, "find_first", "1", false, "break when first found")
	findTxCmd.PersistentFlags().StringVarP(&branchIDStr, "branch", "b", "", "tip branch id hex")
	findTxCmd.InitDefaultHelpCmd()

	return findTxCmd
}

var (
	findInSlot          uint32
	findWithHexFragment string
	findFirst           bool
	branchIDStr         string
)

func runFindTxCmd(_ *cobra.Command, _ []string) {
	glb.InitLedgerFromDB()
	glb.InitTxStoreDB()
	defer glb.CloseDatabases()

	glb.Assertf(findInSlot != 0 || findWithHexFragment != "", "at least one of slot or fragment must be specified")

	var filterSlots []base.Slot
	if findInSlot != 0 {
		filterSlots = []base.Slot{base.Slot(findInSlot)}
	}

	var root common.VCommitment
	if branchIDStr != "" {
		b, err := base.TransactionIDFromHexString(branchIDStr)
		glb.AssertNoError(err)
		rr, found := branches.FetchBranchData(glb.StateStore(), b)
		glb.Assertf(found, "didn't find branch %s", b.String())
		glb.Infof("branch id: %s", b.String())
		root = rr.Root
	} else {
		lrb := multistate.FindLatestReliableBranch(glb.StateStore(), global.FractionHealthyBranch)
		glb.Assertf(lrb != nil, "can't find latest reliable branch (LRB)")
		root = lrb.Root
		glb.Infof("latest reliable branch will be used")
	}

	if findInSlot > 0 {
		glb.Infof("find in slot %d", findInSlot)
	} else {
		glb.Infof("find in slot: N/A")
	}
	if findWithHexFragment != "" {
		glb.Infof("find with hex fragment '%s'", findWithHexFragment)
	} else {
		glb.Infof("find with hex fragment: N/A")
	}

	rdr := multistate.MustNewReadable(glb.StateStore(), root)
	nTx := 0
	nFound := 0
	rdr.IterateKnownCommittedTransactions(func(txid *base.TransactionID, _ base.Slot) bool {
		if findWithHexFragment == "" || strings.Contains(txid.String(), findWithHexFragment) {
			glb.Infof("%6d   %s    %s", nFound, txid.StringHex(), txid.String())
			nFound++
		}
		nTx++
		return !findFirst || nFound == 0
	}, filterSlots...)

	glb.Infof("---------\ntotal: %d transaction IDs found", nFound)
	glb.Infof("---------\ntotal: %d transaction scanned", nTx)
}
