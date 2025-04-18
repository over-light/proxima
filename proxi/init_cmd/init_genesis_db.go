package init_cmd

import (
	"os"

	"github.com/dgraph-io/badger/v4"
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/unitrie/adaptors/badger_adaptor"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var fetchLedgerID bool

func initGenesisDBCmd() *cobra.Command {
	genesisCmd := &cobra.Command{
		Use:   "genesis_db",
		Short: "creates multi-state DB and initializes genesis ledger state init according ledger id data taken either from (1) 'proxi.genesis.id.yaml' (default) or (2) from another node",
		Args:  cobra.NoArgs,
		Run:   runGenesis,
	}
	genesisCmd.PersistentFlags().BoolVarP(&fetchLedgerID, "remote", "r", false, "fetch ledger identity data from remote API endpoint")
	err := viper.BindPFlag("remote", genesisCmd.PersistentFlags().Lookup("remote"))
	glb.AssertNoError(err)

	return genesisCmd
}

func runGenesis(_ *cobra.Command, _ []string) {
	glb.DirMustNotExistOrBeEmpty(global.MultiStateDBName)

	var err error
	var idDataYAML []byte

	if fetchLedgerID {
		glb.ReadInConfig()
		glb.Infof("retrieving ledger identity data from '%s'", viper.GetString("api.endpoint"))
		idDataYAML, err = glb.GetClient().GetLedgerIdentityData()
		glb.AssertNoError(err)
	} else {
		glb.Infof("reading ledger identity data from file '%s'", glb.LedgerIDFileName)
		// take ledger id data from the 'proxi.genesis.id.yaml'
		idDataYAML, err = os.ReadFile(glb.LedgerIDFileName)
		glb.AssertNoError(err)
	}

	_, idParams, err := ledger.ParseLedgerIdYAML(idDataYAML, base.GetEmbeddedFunctionResolver)
	glb.AssertNoError(err)

	glb.Infof("Will be creating genesis with the following ledger identity parameters:")
	glb.Infof(idParams.Lines("      ").String())
	glb.Infof("Multi-state database name: '%s'", global.MultiStateDBName)

	if !glb.YesNoPrompt("Proceed?", true) {
		glb.Fatalf("exit: genesis database wasn't created")
	}

	// create state store and initialize genesis state
	stateDb := badger_adaptor.MustCreateOrOpenBadgerDB(global.MultiStateDBName, badger.DefaultOptions(global.MultiStateDBName))
	stateStore := badger_adaptor.New(stateDb)
	defer func() { _ = stateStore.Close() }()

	bootstrapChainID, _ := multistate.InitStateStore(idParams, idDataYAML, stateStore)
	glb.Infof("Genesis state DB '%s' has been created successfully.\nBootstrap sequencer chainID: %s", global.MultiStateDBName, bootstrapChainID.String())
}
