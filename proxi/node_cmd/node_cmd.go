package node_cmd

import (
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/proxi/node_cmd/seq_cmd"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func Init() *cobra.Command {
	nodeCmd := &cobra.Command{
		Use:   "node [<subcommand>]",
		Short: "specifies node API subcommand",
		Args:  cobra.NoArgs,
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			glb.ReadInConfig()
		},
	}

	nodeCmd.PersistentFlags().StringP("config", "c", "", "proxi config profile name")
	err := viper.BindPFlag("config", nodeCmd.PersistentFlags().Lookup("config"))
	glb.AssertNoError(err)

	nodeCmd.PersistentFlags().String("private_key", "", "ED25519 private key (hex encoded)")
	err = viper.BindPFlag("private_key", nodeCmd.PersistentFlags().Lookup("private_key"))
	glb.AssertNoError(err)

	nodeCmd.PersistentFlags().String("api.endpoint", "", "<DNS name>:port")
	err = viper.BindPFlag("api.endpoint", nodeCmd.PersistentFlags().Lookup("api.endpoint"))
	glb.AssertNoError(err)

	nodeCmd.PersistentFlags().BoolP("nowait", "n", false, "do not wait for inclusion")
	err = viper.BindPFlag("nowait", nodeCmd.PersistentFlags().Lookup("nowait"))
	glb.AssertNoError(err)

	nodeCmd.PersistentFlags().BoolVarP(&glb.UseAlternativeTagAlongSequencer, "tag_along.alt", "a", false, "use alternative tag-along sequencer")
	err = viper.BindPFlag("tag_along.alt", nodeCmd.PersistentFlags().Lookup("tag_along.alt"))
	glb.AssertNoError(err)

	nodeCmd.PersistentFlags().IntVarP(&glb.TargetInclusionDepth, "depth", "e", 2, "target inclusion depth")
	err = viper.BindPFlag("depth", nodeCmd.PersistentFlags().Lookup("depth"))
	glb.AssertNoError(err)

	nodeCmd.InitDefaultHelpCmd()
	nodeCmd.AddCommand(
		initGetOutputsCmd(),
		initGetChainOutputCmd(),
		initCompactOutputsCmd(),
		initBalanceCmd(),
		initTransferCmd(),
		initSpamCmd(),
		initMakeChainCmd(),
		//initDeleteChainCmd(),
		initKillChainCmd(),
		initChainsCmd(),
		initNodeInfoCmd(),
		seq_cmd.Init(),
		initSeqSetupCmd(),
		initSyncInfoCmd(),
		initPeersInfoCmd(),
		initReliableBranchCmd(),
		initInflateChainCmd(),
		initFaucetServerCmd(),
		initGetFundsCmd(),
		initLastSeqCmd(),
		initDelegateCmd(),
		initAllChainsCmd(),
	)
	return nodeCmd
}

func displayTotals(outs []*ledger.OutputWithID) {
	var sumOnChains, sumOutsideChains uint64
	var numChains, numNonChains int

	for _, o := range outs {
		if _, idx := o.Output.ChainConstraint(); idx != 0xff {
			numChains++
			sumOnChains += o.Output.Amount()
		} else {
			numNonChains++
			sumOutsideChains += o.Output.Amount()
		}
	}
	if numNonChains > 0 {
		glb.Infof("amount controlled on %d non-chain outputs: %s", numNonChains, util.Th(sumOutsideChains))
	}
	if numChains > 0 {
		glb.Infof("amount controlled on %d chain outputs: %s", numChains, util.Th(sumOnChains))
	}
	glb.Infof("TOTAL controlled on %d outputs: %s", numChains+numNonChains, util.Th(sumOnChains+sumOutsideChains))
}
