package general

import (
	"github.com/lunfardo314/proxima/core"
	"github.com/lunfardo314/unitrie/common"
)

type (
	StateReader interface {
		GetUTXO(id *core.OutputID) ([]byte, bool)
		HasUTXO(id *core.OutputID) bool
	}

	StateIndexReader interface {
		GetUTXOsLockedInAccount(accountID core.AccountID) ([]*core.OutputDataWithID, error)
		GetUTXOForChainID(id *core.ChainID) (*core.OutputDataWithID, error)
		Root() common.VCommitment
		IdentityBytes() []byte
	}

	// IndexedStateReader state and indexer readers packing together
	IndexedStateReader interface {
		StateReader
		StateIndexReader
	}

	StateStore interface {
		common.KVReader
		common.BatchedUpdatable
		common.Traversable
	}
)