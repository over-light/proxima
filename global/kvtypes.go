package global

import (
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/unitrie/common"
)

// access to the state

type (
	StateReader interface {
		GetUTXO(id *ledger.OutputID) ([]byte, bool)
		HasUTXO(id *ledger.OutputID) bool
		KnowsCommittedTransaction(txid *ledger.TransactionID) bool // all txids are kept in the state for some time
	}

	StateIndexReader interface {
		IterateUTXOIDsInAccount(addr ledger.AccountID, fun func(oid ledger.OutputID) bool) (err error)
		IterateUTXOsInAccount(addr ledger.AccountID, fun func(oid ledger.OutputID, odata []byte) bool) (err error)

		GetUTXOIDsInAccount(addr ledger.AccountID) ([]ledger.OutputID, error)
		GetUTXOsInAccount(accountID ledger.AccountID) ([]*ledger.OutputDataWithID, error) // TODO leave Iterate.. only?

		GetUTXOForChainID(id *ledger.ChainID) (*ledger.OutputDataWithID, error)
		Root() common.VCommitment
		MustLedgerIdentityBytes() []byte // either state identity consistent or panic
	}

	// IndexedStateReader state and indexer readers packing together
	IndexedStateReader interface {
		StateReader
		StateIndexReader
	}

	StateStoreReader interface {
		common.KVReader
		common.Traversable
		IsClosed() bool
	}

	StateStore interface {
		StateStoreReader
		common.BatchedUpdatable
	}
)
