package gen_cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func genIDCmd() *cobra.Command {
	initLedgerIDCmd := &cobra.Command{
		Use:   "ledger_id",
		Args:  cobra.NoArgs,
		Short: fmt.Sprintf("creates identity data of the ledger with genesis controller taken from proxi wallet. Saves it to the file '%s'", glb.LedgerIDFileName),
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			glb.ReadInConfig()
		},
		Run: runGenLedgerIDCommand,
	}
	initLedgerIDCmd.PersistentFlags().StringP("config", "c", "", "profile name")
	err := viper.BindPFlag("config", initLedgerIDCmd.PersistentFlags().Lookup("config"))
	glb.AssertNoError(err)

	return initLedgerIDCmd
}

func runGenLedgerIDCommand(_ *cobra.Command, _ []string) {
	if glb.FileExists(glb.LedgerIDFileName) {
		if !glb.YesNoPrompt(fmt.Sprintf("file '%s' already exists. Overwrite?", glb.LedgerIDFileName), false) {
			os.Exit(0)
		}
	}
	privKey := glb.MustGetPrivateKey()

	// create ledger identity
	id := ledger.DefaultIdentityParameters(privKey, uint32(time.Now().Unix()))

	yamlData := ledger.LibraryYAMLFromIdentityParameters(id, true)
	err := os.WriteFile(glb.LedgerIDFileName, yamlData, 0666)
	glb.AssertNoError(err)
	glb.Infof("new ledger identity data has been stored in the file '%s':", glb.LedgerIDFileName)
	glb.Infof("--------------\n%s--------------\n", string(yamlData))
}
