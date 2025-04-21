package util_cmd

import (
	"github.com/spf13/cobra"
)

func Init() *cobra.Command {
	genCmd := &cobra.Command{
		Use:   "util",
		Args:  cobra.NoArgs,
		Short: "utility functions",
		Run: func(cmd *cobra.Command, args []string) {
		},
	}
	genCmd.AddCommand(
		genEd25519Cmd(),
		genHostIDCmd(),
		genIDCmd(),
		validateIDCmd(),
		compileIDCmd(),
		initParseTx(),
		initParseBytecode(),
	)
	genCmd.InitDefaultHelpCmd()
	return genCmd
}
