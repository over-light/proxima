package snapshot_cmd

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
	multistate2 "github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/unitrie/adaptors/badger_adaptor"
	"github.com/lunfardo314/unitrie/common"
	"github.com/lunfardo314/unitrie/immutable"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	fname     string
	batchSize int
)

func initRestoreCmd() *cobra.Command {
	restoreCmd := &cobra.Command{
		Use:   "restore [<batch size>]",
		Short: "creates multi-state db from snapshot",
		Args:  cobra.MaximumNArgs(1),
		Run:   runRestoreCmd,
	}

	restoreCmd.PersistentFlags().StringVarP(&fname, "snapshot_file", "s", "", "snapshot file")
	err := viper.BindPFlag("snapshot_file", restoreCmd.PersistentFlags().Lookup("snapshot_file"))
	glb.AssertNoError(err)

	restoreCmd.PersistentFlags().IntVarP(&batchSize, "batch_size", "b", defaultBatchSize, "commit batch size (records)")
	err = viper.BindPFlag("batch_size", restoreCmd.PersistentFlags().Lookup("batch_size"))
	glb.AssertNoError(err)

	restoreCmd.InitDefaultHelpCmd()
	return restoreCmd
}

const (
	trieCacheSize    = 10_000
	defaultBatchSize = 4_000
)

func runRestoreCmd(_ *cobra.Command, args []string) {
	if fname == "" {
		var ok bool
		fname, ok = findLatestSnapshotFile()
		glb.Assertf(ok, "can't find snapshot file")
	}
	glb.Infof("snapshot file: %s", fname)
	glb.Infof("batch size is %d", batchSize)

	kvStream, err := multistate2.OpenSnapshotFileStream(fname)
	glb.AssertNoError(err)
	defer kvStream.Close()

	glb.Infof("Verbosity level: %d", glb.VerbosityLevel())
	glb.Infof("snapshot file: %s", fname)
	glb.Infof("format version: %s", kvStream.Header.Version)
	glb.Infof("branch ID: %s", kvStream.BranchID.String())
	glb.Infof("root record:\n%s", kvStream.RootRecord.Lines("    ").String())
	glb.Infof("ledger id:\n%s", kvStream.LedgerID.Lines("    ").String())

	start := time.Now()

	stateDb := badger_adaptor.MustCreateOrOpenBadgerDB(global.MultiStateDBName, badger.DefaultOptions(global.MultiStateDBName))
	stateStore := badger_adaptor.New(stateDb)
	defer func() { _ = stateStore.Close() }()

	emptyRoot, err := multistate2.CommitEmptyRootWithLedgerIdentity(*kvStream.LedgerID, stateStore)
	glb.AssertNoError(err)

	trieUpdatable, err := immutable.NewTrieUpdatable(ledger.CommitmentModel, stateStore, emptyRoot, trieCacheSize)
	glb.AssertNoError(err)

	var batch common.KVBatchedWriter
	var inBatch int
	var lastRoot common.VCommitment

	console := io.Discard
	if glb.IsVerbose() {
		console = os.Stdout
	}
	total := 0
	verbosityLevel := glb.VerbosityLevel()
	counters := make(map[byte]int)

	begin := time.Now()

	for pair := range kvStream.InChan {
		if util.IsNil(batch) {
			batch = stateStore.BatchedWriter()
		}
		already := trieUpdatable.Update(pair.Key, pair.Value)
		glb.Assertf(!already, "repeating key %s", hex.EncodeToString(pair.Key))
		inBatch++

		if verbosityLevel > 1 {
			_outKVPair(pair.Key, pair.Value, total, console)
		}
		total++
		counters[pair.Key[0]] = counters[pair.Key[0]] + 1

		if inBatch == batchSize {
			lastRoot = trieUpdatable.Commit(batch)
			err = batch.Commit()
			util.AssertNoError(err)
			inBatch = 0
			batch = nil
			trieUpdatable, err = immutable.NewTrieUpdatable(ledger.CommitmentModel, stateStore, lastRoot, trieCacheSize)
			util.AssertNoError(err)
			_, _ = fmt.Fprintf(console, "--- committed %d (+%d) records in %v, %.0f records/sec\n",
				total, batchSize, time.Since(begin), float64(total)/(float64(time.Since(begin))/float64(time.Second)))
		}
	}
	if !util.IsNil(batch) {
		lastRoot = trieUpdatable.Commit(batch)
		err = batch.Commit()
		util.AssertNoError(err)
		_, _ = fmt.Fprintf(console, "--- commit remaining ---\n")
	}
	// write meta-records
	batch = stateStore.BatchedWriter()
	multistate2.WriteLatestSlotRecord(batch, kvStream.BranchID.Slot())
	multistate2.WriteEarliestSlotRecord(batch, kvStream.BranchID.Slot())
	multistate2.WriteRootRecord(batch, kvStream.BranchID, kvStream.RootRecord)

	err = batch.Commit()
	glb.AssertNoError(err)

	glb.Assertf(ledger.CommitmentModel.EqualCommitments(lastRoot, kvStream.RootRecord.Root),
		"inconsistency: final root %s is not equal to the root in the root record %s",
		lastRoot.String(), kvStream.RootRecord.Root.String())

	glb.Infof("Success\nTotal %d records. By type:", total)
	for _, k := range util.KeysSorted(counters, func(k1, k2 byte) bool { return k1 < k2 }) {
		glb.Infof("    %s: %d", multistate2.PartitionToString(k), counters[k])
	}
	glb.Infof("it took %v, %d records/sec", time.Since(start), time.Duration(total)*time.Second/time.Since(start))
}
