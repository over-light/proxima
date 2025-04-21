package txstore

import (
	"os"

	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/spf13/cobra"
)

var (
	txStoreParse bool
	txStoreSave  bool
)

func initGetCmd() *cobra.Command {
	getCmd := &cobra.Command{
		Use:   "get <transaction id hex>",
		Short: "retrieves transaction from the txStore, optionally parses and saves it",
		Args:  cobra.ExactArgs(1),
		Run:   runGetCmd,
	}
	getCmd.PersistentFlags().BoolVarP(&txStoreParse, "parse", "p", false, "parse and display transaction with metadata")
	getCmd.PersistentFlags().BoolVarP(&txStoreSave, "save", "s", false, "save transaction with metadata as file")
	getCmd.InitDefaultHelpCmd()
	return getCmd
}

func runGetCmd(_ *cobra.Command, args []string) {
	glb.InitLedgerFromDB()
	glb.InitTxStoreDB()
	defer glb.CloseDatabases()

	txid, err := base.TransactionIDFromHexString(args[0])
	glb.AssertNoError(err)

	txBytesWithMetadata := glb.TxBytesStore().GetTxBytesWithMetadata(&txid)
	if len(txBytesWithMetadata) == 0 {
		glb.Infof("NOT FOUND transaction %s in the txStore", txid.String())
		os.Exit(1)
	}

	glb.Infof("FOUND transaction %s in the txStore\n%d bytes including metadata", txid.String(), len(txBytesWithMetadata))
	if txStoreParse {
		glb.ParseAndDisplayTx(txBytesWithMetadata)
	}

	if txStoreSave {
		saveTx(&txid, txBytesWithMetadata)
	}
}

func saveTx(txid *base.TransactionID, txBytesWithMetadata []byte) {
	err := os.WriteFile(txid.AsFileName(), txBytesWithMetadata, 0666)
	glb.AssertNoError(err)
}
