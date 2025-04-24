package multistate

import (
	"fmt"
	"sync"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/unitrie/common"
	"github.com/lunfardo314/unitrie/immutable"
)

type (
	// Updatable is an updatable ledger state, with the particular root
	// Suitable for chained updates
	// Not-thread safe, should be used individual instance for each parallel update.
	// DB (store) is updated atomically with all mutations in one DB transaction
	Updatable struct {
		trie  *immutable.TrieUpdatable
		store StateStore
	}

	// Readable is a read-only ledger state, with the particular root
	// It is thread-safe. The state itself is read-only, but trie cache needs write-lock with every call
	Readable struct {
		mutex *sync.Mutex
		trie  *immutable.TrieReader
	}

	// RootRecord is a persistent data stored in the DB partition with each state root
	// It contains deterministic values for that state
	RootRecord struct {
		Root        common.VCommitment
		SequencerID base.ChainID
		// Note: LedgerCoverage, SlotInflation and Supply are deterministic values calculated from the ledger past cone
		// Each node calculates them itself, and they must be equal on each
		CoverageDelta  uint64
		LedgerCoverage uint64
		// SlotInflation: total inflation delta from previous root. It is a sum of individual transaction inflation values
		// of the previous slot/past cone. It includes the branch tx inflation itself and does not include inflation of the previous branch
		SlotInflation uint64
		// Supply: total supply at this root (including the branch itself, excluding prev branch).
		// It is the sum of the Supply of the previous branch and SlotInflation of the current
		Supply uint64
		// Number of new transactions in the slot of the branch
		NumTransactions uint32
		// TODO probably there's a need for other deterministic values, such as total number of outputs, of transactions, of chains
	}

	BranchData struct {
		RootRecord
		Stem            *ledger.OutputWithID
		SequencerOutput *ledger.OutputWithID
	}
)

// partitions of the state store on the trie
const (
	TriePartitionLedgerState = byte(iota)
	TriePartitionAccounts
	TriePartitionChainID
	TriePartitionCommittedTransactionID
)

func PartitionToString(p byte) string {
	switch p {
	case TriePartitionLedgerState:
		return "UTXO"
	case TriePartitionAccounts:
		return "ACCN"
	case TriePartitionChainID:
		return "CHID"
	case TriePartitionCommittedTransactionID:
		return "TXID"
	default:
		return "????"
	}
}

func LedgerIdentityBytesFromStore(store StateStore) []byte {
	rr := FetchAnyLatestRootRecord(store)
	return LedgerIdentityBytesFromRoot(store, rr.Root)
}

func LedgerIdentityBytesFromRoot(store StateStoreReader, root common.VCommitment) []byte {
	trie, err := immutable.NewTrieReader(ledger.CommitmentModel, store, root, 0)
	util.AssertNoError(err)
	return trie.Get(nil)
}

// NewReadable creates read-only ledger state with the given root
func NewReadable(store common.KVReader, root common.VCommitment, clearCacheAtSize ...int) (*Readable, error) {
	trie, err := immutable.NewTrieReader(ledger.CommitmentModel, store, root, clearCacheAtSize...)
	if err != nil {
		return nil, err
	}
	return &Readable{
		mutex: &sync.Mutex{},
		trie:  trie,
	}, nil
}

func MustNewReadable(store common.KVReader, root common.VCommitment, clearCacheAtSize ...int) *Readable {
	ret, err := NewReadable(store, root, clearCacheAtSize...)
	util.AssertNoError(err)
	return ret
}

// NewUpdatable creates updatable state with the given root. After updated, the root changes.
// Suitable for chained updates of the ledger state
func NewUpdatable(store StateStore, root common.VCommitment) (*Updatable, error) {
	trie, err := immutable.NewTrieUpdatable(ledger.CommitmentModel, store, root)
	if err != nil {
		return nil, err
	}
	return &Updatable{
		trie:  trie,
		store: store,
	}, nil
}

func MustNewUpdatable(store StateStore, root common.VCommitment) *Updatable {
	ret, err := NewUpdatable(store, root)
	util.AssertNoError(err)
	return ret
}

func (r *Readable) GetUTXO(oid base.OutputID) ([]byte, bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r._getUTXO(oid)
}

func (r *Readable) _getUTXO(oid base.OutputID, partition ...*common.ReaderPartition) ([]byte, bool) {
	var part *common.ReaderPartition
	if len(partition) > 0 {
		part = partition[0]
	} else {
		part = common.MakeReaderPartition(r.trie, TriePartitionLedgerState)
		defer part.Dispose()
	}

	ret := part.Get(oid[:])
	if len(ret) == 0 {
		return nil, false
	}

	return ret, true
}

func (r *Readable) HasUTXO(oid base.OutputID) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	partition := common.MakeReaderPartition(r.trie, TriePartitionLedgerState)
	defer partition.Dispose()

	return partition.Has(oid[:])
}

// KnowsCommittedTransaction transaction IDs are purged after some time, so the result may be
func (r *Readable) KnowsCommittedTransaction(txid base.TransactionID) bool {
	part := common.MakeReaderPartition(r.trie, TriePartitionCommittedTransactionID)
	defer part.Dispose()

	return part.Has(txid[:])
}

func (r *Readable) GetUTXOIDsInAccount(addr ledger.AccountID) ([]base.OutputID, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if len(addr) > 255 {
		return nil, fmt.Errorf("accountID length should be <= 255")
	}
	ret := make([]base.OutputID, 0)
	var oid base.OutputID
	var err error

	accountPrefix := common.Concat(TriePartitionAccounts, byte(len(addr)), addr)
	r.trie.Iterator(accountPrefix).IterateKeys(func(k []byte) bool {
		oid, err = base.OutputIDFromBytes(k[len(accountPrefix):])
		if err != nil {
			return false
		}
		ret = append(ret, oid)
		return true
	})

	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (r *Readable) GetUTXOsInAccount(addr ledger.AccountID) ([]*ledger.OutputDataWithID, error) {
	partition := common.MakeReaderPartition(r.trie, TriePartitionLedgerState)
	defer partition.Dispose()

	ret := make([]*ledger.OutputDataWithID, 0)
	err := r.IterateUTXOsInAccount(addr, func(oid base.OutputID, odata []byte) bool {
		ret = append(ret, &ledger.OutputDataWithID{
			ID:   oid,
			Data: odata,
		})
		return true
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (r *Readable) IterateUTXOsInAccount(addr ledger.AccountID, fun func(oid base.OutputID, odata []byte) bool) (err error) {
	partition := common.MakeReaderPartition(r.trie, TriePartitionLedgerState)
	defer partition.Dispose()

	return r.IterateUTXOIDsInAccount(addr, func(oid base.OutputID) bool {
		if odata, found := r._getUTXO(oid, partition); found {
			return fun(oid, odata)
		}
		return true
	})
}

func (r *Readable) IterateUTXOIDsInAccount(addr ledger.AccountID, fun func(oid base.OutputID) bool) (err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if len(addr) > 255 {
		return fmt.Errorf("accountID length should be <= 255")
	}
	accountPrefix := common.Concat(TriePartitionAccounts, byte(len(addr)), addr)

	var oid base.OutputID

	partition := common.MakeReaderPartition(r.trie, TriePartitionLedgerState)
	defer partition.Dispose()

	r.trie.Iterator(accountPrefix).IterateKeys(func(k []byte) bool {
		oid, err = base.OutputIDFromBytes(k[len(accountPrefix):])
		if err != nil {
			return false
		}
		return fun(oid)
	})
	return err
}

func (r *Readable) GetUTXOForChainID(id base.ChainID) (*ledger.OutputDataWithID, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r._getUTXOForChainID(id)
}

func (r *Readable) _getUTXOForChainID(id base.ChainID) (*ledger.OutputDataWithID, error) {
	chainPartition := common.MakeReaderPartition(r.trie, TriePartitionChainID)
	outID := chainPartition.Get(id[:])
	defer chainPartition.Dispose()

	if len(outID) == 0 {
		return nil, ErrNotFound
	}
	oid, err := base.OutputIDFromBytes(outID)
	if err != nil {
		return nil, err
	}
	outData, found := r._getUTXO(oid)

	if !found {
		return nil, ErrNotFound
	}
	return &ledger.OutputDataWithID{
		ID:   oid,
		Data: outData,
	}, nil
}

func (r *Readable) GetStem() (base.Slot, []byte) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	accountPrefix := common.Concat(TriePartitionAccounts, byte(len(ledger.StemAccountID)), ledger.StemAccountID)

	var found bool
	var retSlot base.Slot
	var retBytes []byte

	partition := common.MakeReaderPartition(r.trie, TriePartitionLedgerState)
	defer partition.Dispose()

	// we iterate one element. Stem output ust always be present in the state
	count := 0
	r.trie.Iterator(accountPrefix).IterateKeys(func(k []byte) bool {
		util.Assertf(count == 0, "inconsistency: must be exactly 1 index record for stem output")
		count++
		oid, err := base.OutputIDFromBytes(k[len(accountPrefix):])
		util.AssertNoError(err)
		retSlot = oid.Slot()
		retBytes, found = r._getUTXO(oid, partition)
		util.Assertf(found, "can't find stem output")
		return true
	})
	return retSlot, retBytes
}

func (r *Readable) MustLedgerIdentityBytes() []byte {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.trie.Get(nil)
}

func (r *Readable) Iterator(prefix []byte) common.KVIterator {
	return r.trie.Iterator(prefix)
}

// IterateKnownCommittedTransactions iterates transaction IDs in the state. Optionally, iteration is restricted
// for a slot. In that case first iterates non-sequencer transactions, the sequencer transactions
func (r *Readable) IterateKnownCommittedTransactions(fun func(txid *base.TransactionID, slot base.Slot) bool, txidSlot ...base.Slot) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	var iter common.KVIterator
	if len(txidSlot) > 0 {
		iter = common.MakeTraversableReaderPartition(r.trie, TriePartitionCommittedTransactionID).Iterator(txidSlot[0].Bytes())
	} else {
		iter = common.MakeTraversableReaderPartition(r.trie, TriePartitionCommittedTransactionID).Iterator(nil)
	}

	var slot base.Slot

	iter.Iterate(func(k, v []byte) bool {
		txid, err := base.TransactionIDFromBytes(k[1:])
		util.AssertNoError(err)
		slot, err = base.SlotFromBytes(v)
		util.AssertNoError(err)

		return fun(&txid, slot)
	})
}

func (r *Readable) AccountsByLocks() map[string]LockedAccountInfo {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	var oid base.OutputID
	var err error

	ret := make(map[string]LockedAccountInfo)

	partition := common.MakeReaderPartition(r.trie, TriePartitionLedgerState)
	defer partition.Dispose()

	r.trie.Iterator([]byte{TriePartitionAccounts}).IterateKeys(func(k []byte) bool {
		oid, err = base.OutputIDFromBytes(k[2+k[1]:])
		util.AssertNoError(err)

		oData, found := r._getUTXO(oid, partition)
		util.Assertf(found, "can't get output")

		_, amount, lock, err := ledger.OutputFromBytesMain(oData)
		util.AssertNoError(err)

		lockStr := lock.String()
		lockInfo := ret[lockStr]
		lockInfo.Balance += uint64(amount)
		lockInfo.NumOutputs++
		ret[lockStr] = lockInfo

		return true
	})
	return ret
}

func (r *Readable) IterateChainTips(fun func(chainID base.ChainID, oid base.OutputID) bool) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	var chainID base.ChainID
	var oid base.OutputID
	var err error
	r.trie.Iterator([]byte{TriePartitionChainID}).Iterate(func(k []byte, v []byte) bool {
		chainID, err = base.ChainIDFromBytes(k[1:])
		if err != nil {
			return false
		}
		oid, err = base.OutputIDFromBytes(v)
		if err != nil {
			return false
		}
		return fun(chainID, oid)
	})
	return err
}

func (r *Readable) Root() common.VCommitment {
	// non need to lock
	return r.trie.Root()
}

func (r *Readable) IterateUTXOsInSlot(slot base.Slot, fun func(oid base.OutputID, oData []byte) bool) (err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	prefix := common.Concat(TriePartitionLedgerState, slot.Bytes())

	var oid base.OutputID

	partition := common.MakeReaderPartition(r.trie, TriePartitionLedgerState)
	defer partition.Dispose()

	r.trie.Iterator(prefix).Iterate(func(k, v []byte) bool {
		oid, err = base.OutputIDFromBytes(k[len(prefix):])
		if err != nil {
			return false
		}
		return fun(oid, v)
	})
	return err
}

func (u *Updatable) Readable() *Readable {
	return &Readable{
		mutex: &sync.Mutex{},
		trie:  u.trie.TrieReader,
	}
}

func (u *Updatable) Root() common.VCommitment {
	return u.trie.Root()
}

type RootRecordParams struct {
	StemOutputID      base.OutputID
	SeqID             base.ChainID
	CoverageDelta     uint64
	LedgerCoverage    uint64
	SlotInflation     uint64
	Supply            uint64
	NumTransactions   uint32
	WriteEarliestSlot bool
}

// Update updates trie with mutations
// If par.GenesisStemOutputID != nil, also writes root partition record
func (u *Updatable) Update(muts *Mutations, rootRecordParams *RootRecordParams) error {
	err := u.updateUTXOLedgerDB(func(trie *immutable.TrieUpdatable) error {
		return UpdateTrie(u.trie, muts)
	}, rootRecordParams)
	if err != nil {
		err = fmt.Errorf("%w\n-------- mutations --------\n%s", err, muts.Lines("    ").String())
	}
	return err
}

func (u *Updatable) MustUpdate(muts *Mutations, par *RootRecordParams) {
	err := u.Update(muts, par)
	util.AssertNoError(err)
}

func (u *Updatable) updateUTXOLedgerDB(updateFun func(updatable *immutable.TrieUpdatable) error, rootRecordsParams *RootRecordParams) error {
	if err := updateFun(u.trie); err != nil {
		return err
	}
	batch := u.store.BatchedWriter()
	newRoot := u.trie.Commit(batch)
	if rootRecordsParams != nil {
		latestSlot := FetchLatestCommittedSlot(u.store)
		if latestSlot < rootRecordsParams.StemOutputID.Slot() {
			WriteLatestSlotRecord(batch, rootRecordsParams.StemOutputID.Slot())
		}
		if rootRecordsParams.WriteEarliestSlot {
			WriteEarliestSlotRecord(batch, rootRecordsParams.StemOutputID.Slot())
		}
		branchID := rootRecordsParams.StemOutputID.TransactionID()
		WriteRootRecord(batch, branchID, RootRecord{
			Root:            newRoot,
			SequencerID:     rootRecordsParams.SeqID,
			CoverageDelta:   rootRecordsParams.CoverageDelta,
			LedgerCoverage:  rootRecordsParams.LedgerCoverage,
			SlotInflation:   rootRecordsParams.SlotInflation,
			Supply:          rootRecordsParams.Supply,
			NumTransactions: rootRecordsParams.NumTransactions,
		})
	}
	var err error
	if err = batch.Commit(); err != nil {
		return err
	}
	if u.trie, err = immutable.NewTrieUpdatable(ledger.CommitmentModel, u.store, newRoot); err != nil {
		return err
	}
	return nil
}
