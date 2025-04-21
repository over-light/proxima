package util_cmd

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/spf13/cobra"
)

func initParseBytecode() *cobra.Command {
	validateLedgerIDCmd := &cobra.Command{
		Use:   "parse_bytecode <bytecode hex>",
		Args:  cobra.ExactArgs(1),
		Short: fmt.Sprintf("parses EasyFL bytecode with ledger definitions provided in '%s'", glb.LedgerIDFileName),
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			glb.ReadInConfig()
		},
		Run: runParseBytecode,
	}
	//validateLedgerIDCmd.PersistentFlags().StringP("config", "c", "", "profile name")
	//err := viper.BindPFlag("config", validateLedgerIDCmd.PersistentFlags().Lookup("config"))
	//glb.AssertNoError(err)

	return validateLedgerIDCmd
}

func runParseBytecode(_ *cobra.Command, args []string) {
	ledgerIDData, err := os.ReadFile(glb.LedgerIDFileName)
	glb.AssertNoError(err)
	ledger.MustInitSingleton(ledgerIDData)

	bytecode, err := hex.DecodeString(args[0])
	glb.AssertNoError(err)

	c, err := ledger.ConstraintFromBytes(bytecode)
	glb.AssertNoError(err)

	glb.Infof("Parsed bytecode:\n    string: %s\n    source: %s", c.String(), c.Source())
}
