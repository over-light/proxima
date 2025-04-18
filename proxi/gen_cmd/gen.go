package gen_cmd

import (
	"github.com/spf13/cobra"
)

func Init() *cobra.Command {
	genCmd := &cobra.Command{
		Use:   "gen",
		Args:  cobra.NoArgs,
		Short: "utility functions for data generation and validation",
		Run: func(cmd *cobra.Command, args []string) {
		},
	}
	genCmd.AddCommand(
		genEd25519Cmd(),
		genHostIDCmd(),
		genIDCmd(),
		validateIDCmd(),
		compileIDCmd(),
	)
	genCmd.InitDefaultHelpCmd()
	return genCmd
}
