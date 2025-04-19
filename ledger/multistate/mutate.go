package multistate

import (
	"fmt"
	"slices"
	"sort"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/lines"
	"github.com/lunfardo314/unitrie/common"
	"github.com/lunfardo314/unitrie/immutable"
)

type (
	mutationCmd interface {
		mutate(trie *immutable.TrieUpdatable) error
		text() string
		sortOrder() byte
		timestamp() base.LedgerTime
	}

	mutationAddOutput struct {
		ID     base.OutputID
		Output *ledger.Output // nil means delete
	}

	mutationDelOutput struct {
		ID base.OutputID
	}

	mutationAddTx struct {
		ID              base.TransactionID
		TimeSlot        base.Slot
		LastOutputIndex byte
	}

	mutationDelChain struct {
		ChainID base.ChainID
	}

	Mutations struct {
		mut []mutationCmd
	}
)

func (m *mutationDelOutput) mutate(trie *immutable.TrieUpdatable) error {
	return deleteOutputFromTrie(trie, m.ID)
}

func (m *mutationDelOutput) text() string {
	return fmt.Sprintf("DEL   %s", m.ID.StringShort())
}

func (m *mutationDelOutput) sortOrder() byte {
	return 0
}

func (m *mutationDelOutput) timestamp() base.LedgerTime {
	return m.ID.Timestamp()
}

func (m *mutationAddOutput) mutate(trie *immutable.TrieUpdatable) error {
	return addOutputToTrie(trie, m.ID, m.Output)
}

func (m *mutationAddOutput) text() string {
	return fmt.Sprintf("ADD   %s", m.ID.StringShort())
}

func (m *mutationAddOutput) sortOrder() byte {
	return 1
}

func (m *mutationAddOutput) timestamp() base.LedgerTime {
	return m.ID.Timestamp()
}

func (m *mutationAddTx) mutate(trie *immutable.TrieUpdatable) error {
	return addTxToTrie(trie, &m.ID, m.TimeSlot, m.LastOutputIndex)
}

func (m *mutationAddTx) text() string {
	return fmt.Sprintf("ADDTX %s : slot %d", m.ID.StringShort(), m.TimeSlot)
}

func (m *mutationAddTx) sortOrder() byte {
	return 2
}

func (m *mutationAddTx) timestamp() base.LedgerTime {
	return m.ID.Timestamp()
}

func (m *mutationDelChain) mutate(trie *immutable.TrieUpdatable) error {
	return deleteChainFromTrie(trie, m.ChainID)
}

func (m *mutationDelChain) text() string {
	return fmt.Sprintf("DELCH %s", m.ChainID.StringShort())
}

func (m *mutationDelChain) sortOrder() byte {
	return 3
}

func (m *mutationDelChain) timestamp() base.LedgerTime {
	return base.NewLedgerTime(0xffffffff, 0xff)
}

func NewMutations() *Mutations {
	return &Mutations{
		mut: make([]mutationCmd, 0),
	}
}

func (mut *Mutations) Len() int {
	return len(mut.mut)
}

func (mut *Mutations) Sort() *Mutations {
	sort.Slice(mut.mut, func(i, j int) bool {
		return mut.mut[i].sortOrder() < mut.mut[j].sortOrder()
	})
	return mut
}

func (mut *Mutations) InsertAddOutputMutation(id base.OutputID, o *ledger.Output) {
	mut.mut = append(mut.mut, &mutationAddOutput{
		ID:     id,
		Output: o.Clone(),
	})
}

func (mut *Mutations) InsertDelOutputMutation(id base.OutputID) {
	mut.mut = append(mut.mut, &mutationDelOutput{ID: id})
}

func (mut *Mutations) InsertAddTxMutation(id base.TransactionID, slot base.Slot, lastOutputIndex byte) {
	mut.mut = append(mut.mut, &mutationAddTx{
		ID:              id,
		TimeSlot:        slot,
		LastOutputIndex: lastOutputIndex,
	})
}

func (mut *Mutations) InsertDelChainMutation(id base.ChainID) {
	mut.mut = append(mut.mut, &mutationDelChain{ChainID: id})
}

func (mut *Mutations) Lines(prefix ...string) *lines.Lines {
	ret := lines.New(prefix...)
	mutClone := slices.Clone(mut.mut)
	sort.Slice(mutClone, func(i, j int) bool {
		if mutClone[i].sortOrder() < mutClone[j].sortOrder() {
			return true
		}
		if mutClone[i].sortOrder() == mutClone[j].sortOrder() {
			return mutClone[i].timestamp().Before(mutClone[j].timestamp())
		}
		return false
	})
	for _, m := range mutClone {
		ret.Add(m.text())
	}
	return ret
}

func deleteOutputFromTrie(trie *immutable.TrieUpdatable, oid base.OutputID) error {
	var stateKey [1 + base.OutputIDLength]byte
	stateKey[0] = TriePartitionLedgerState
	copy(stateKey[1:], oid[:])

	oData := trie.Get(stateKey[:])
	if len(oData) == 0 {
		return fmt.Errorf("deleteOutputFromTrie: output not found: %s", oid.StringShort())
	}

	o, err := ledger.OutputFromBytesReadOnly(oData)
	util.AssertNoError(err)

	var existed bool
	existed = trie.Delete(stateKey[:])
	util.Assertf(existed, "deleteOutputFromTrie: inconsistency while deleting output %s", oid.StringShort())

	for _, accountable := range o.Lock().Accounts() {
		existed = trie.Delete(makeAccountKey(accountable.AccountID(), oid))
		// must exist
		util.Assertf(existed, "deleteOutputFromTrie: account record for %s wasn't found as expected: output %s", accountable.String(), oid.StringShort())
	}
	return nil
}

func addOutputToTrie(trie *immutable.TrieUpdatable, oid base.OutputID, out *ledger.Output) error {
	var stateKey [1 + base.OutputIDLength]byte
	stateKey[0] = TriePartitionLedgerState
	copy(stateKey[1:], oid[:])
	if trie.Update(stateKey[:], out.Bytes()) {
		// key should not exist
		return fmt.Errorf("addOutputToTrie: UTXO key should not exist: %s", oid.StringShort())
	}
	for _, accountable := range out.Lock().Accounts() {
		if trie.Update(makeAccountKey(accountable.AccountID(), oid), []byte{0xff}) {
			// key should not exist
			return fmt.Errorf("addOutputToTrie: index key should not exist: %s", oid.StringShort())
		}
	}
	chainConstraint, _ := out.ChainConstraint()
	if chainConstraint == nil {
		// not a chain output
		return nil
	}
	// update chain output records
	var chainID base.ChainID
	if chainConstraint.IsOrigin() {
		chainID = base.MakeOriginChainID(oid)
	} else {
		chainID = chainConstraint.ID
	}
	chainKey := makeChainIDKey(&chainID)

	if chainConstraint.IsOrigin() {
		if existed := trie.Update(chainKey, oid[:]); existed {
			return fmt.Errorf("addOutputToTrie: unexpected chain origin in the state: %s", chainID.StringShort())
		}
	} else {
		const assertChainRecordsConsistency = false
		if assertChainRecordsConsistency {
			// previous chain record may or may not exist
			// enforcing timestamp consistency
			if prevBin := trie.TrieReader.Get(chainKey); len(prevBin) > 0 {
				prevOutputID, err := base.OutputIDFromBytes(prevBin)
				util.AssertNoError(err)
				if !oid.Timestamp().After(prevOutputID.Timestamp()) {
					return fmt.Errorf("addOutputToTrie: chain output id violates time constraint:\n   previous: %s\n   next: %s",
						prevOutputID.StringShort(), oid.StringShort())
				}
			}
		}
		trie.Update(chainKey, oid[:])
	}

	// TODO terminating the chain
	return nil
}

func addTxToTrie(trie *immutable.TrieUpdatable, txid *base.TransactionID, slot base.Slot, lastOutputIndex byte) error {
	var stateKey [1 + base.TransactionIDLength]byte
	stateKey[0] = TriePartitionCommittedTransactionID
	copy(stateKey[1:], txid[:])

	if trie.Update(stateKey[:], slot.Bytes()) {
		// key should not exist
		return fmt.Errorf("addTxToTrie: transaction key should not exist: %s", txid.StringShort())
	}
	return nil
}

func deleteChainFromTrie(trie *immutable.TrieUpdatable, chainID base.ChainID) error {
	var stateKey [1 + base.ChainIDLength]byte
	stateKey[0] = TriePartitionChainID
	copy(stateKey[1:], chainID[:])

	if existed := trie.Delete(stateKey[:]); !existed {
		// only deleting existing chainIDs
		return fmt.Errorf("deleteChainFromTrie: chain id does not exist: %s", chainID.String())
	}
	return nil
}

func makeAccountKey(id ledger.AccountID, oid base.OutputID) []byte {
	return common.Concat([]byte{TriePartitionAccounts, byte(len(id))}, id[:], oid[:])
}

func makeChainIDKey(chainID *base.ChainID) []byte {
	return common.Concat([]byte{TriePartitionChainID}, chainID[:])
}

func UpdateTrie(trie *immutable.TrieUpdatable, mut *Mutations) (err error) {
	for _, m := range mut.mut {
		if err = m.mutate(trie); err != nil {
			return
		}
	}
	return
}
