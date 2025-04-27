package txstore

import (
	"strconv"

	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/spf13/cobra"
)

var listAll bool

func initIDListCmd() *cobra.Command {
	idlistCmd := &cobra.Command{
		Use:   "idlist <slot>",
		Short: "lists IDs of transactions in slot from the raw txstore",
		Args:  cobra.ExactArgs(1),
		Run:   runIdListCmd,
	}
	idlistCmd.PersistentFlags().BoolVarP(&listAll, "all", "a", false, "list all keys in txstore")
	return idlistCmd
}

func runIdListCmd(_ *cobra.Command, args []string) {
	db := glb.InitDBRaw(global.TxStoreDBName)
	defer db.Close()

	sint, err := strconv.Atoi(args[0])
	glb.AssertNoError(err)
	glb.Assertf(sint <= base.MaxSlot, "wrong slot number")
	slot := base.Slot(sint)

	var txid base.TransactionID
	count := 0

	var prefix []byte
	if !listAll {
		prefix = slot.Bytes()
	}
	db.Iterator(prefix).IterateKeys(func(k []byte) bool {
		txid, err = base.TransactionIDFromBytes(k)
		glb.AssertNoError(err)
		glb.Infof("%s    hex = %s", txid.String(), txid.StringHex())
		count++
		return true
	})

	glb.Infof("total: %d transactions", count)
}
