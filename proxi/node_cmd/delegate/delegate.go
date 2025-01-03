package delegate

import (
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/spf13/cobra"
)

func Init() *cobra.Command {
	seqCmd := &cobra.Command{
		Use:   "delegate",
		Short: `defines subcommands for delegation`,
		Args:  cobra.NoArgs,
	}

	glb.AddFlagTarget(seqCmd)

	seqCmd.AddCommand(
		initDelegateSendCmd(),
	)

	seqCmd.InitDefaultHelpCmd()
	return seqCmd
}
