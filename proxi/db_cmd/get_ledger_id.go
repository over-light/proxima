package db_cmd

import (
	"fmt"
	"os"

	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/unitrie/adaptors/badger_adaptor"
	"github.com/spf13/cobra"
)

func initDbGetLedgerIDCmd() *cobra.Command {
	retCmd := &cobra.Command{
		Use:   "get_ledger_definitions",
		Short: fmt.Sprintf("retrieves ledger definitions from DB and saves in file '%s'", glb.LedgerIDFileName),
		Args:  cobra.NoArgs,
		Run:   dbGetLedgerIDCmd,
	}
	retCmd.InitDefaultHelpCmd()
	return retCmd
}

func dbGetLedgerIDCmd(_ *cobra.Command, _ []string) {
	dbName := global.MultiStateDBName
	glb.FileMustExist(dbName)
	glb.Infof("Multi-state database: %s", dbName)
	stateDB := badger_adaptor.MustCreateOrOpenBadgerDB(dbName)
	stateStore := badger_adaptor.New(stateDB)
	yamlData := multistate.LedgerIdentityBytesFromStore(stateStore)
	defer glb.CloseDatabases()

	if glb.FileExists(glb.LedgerIDFileName) {
		if !glb.YesNoPrompt(fmt.Sprintf("file '%s' already exists. Owerwrite", glb.LedgerIDFileName), false) {
			glb.Infof("exit")
			return
		}
	}

	err := os.WriteFile(glb.LedgerIDFileName, yamlData, 0644)
	glb.AssertNoError(err)
}
