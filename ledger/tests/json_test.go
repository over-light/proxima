package tests

import (
	"encoding/json"
	"testing"

	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
	"github.com/stretchr/testify/require"
)

func TestMarshaling(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		txids := []base.TransactionID{
			base.RandomTransactionID(true, 1),
			base.RandomTransactionID(true, 1),
			base.RandomTransactionID(false, 1),
		}
		data, err := json.Marshal(txids)
		require.NoError(t, err)
		t.Logf("txid JSON: %s", string(data))
		var txidsBack []base.TransactionID
		err = json.Unmarshal(data, &txidsBack)
		require.NoError(t, err)
		require.True(t, util.EqualSlices(txids, txidsBack))
	})
	t.Run("1", func(t *testing.T) {
		chainIDs := []base.ChainID{
			base.RandomChainID(),
			base.RandomChainID(),
			base.RandomChainID(),
			base.RandomChainID(),
		}
		data, err := json.Marshal(chainIDs)
		require.NoError(t, err)
		t.Logf("chainid JSON: %s", string(data))
		var chainIDsBack []base.ChainID
		err = json.Unmarshal(data, &chainIDsBack)
		require.NoError(t, err)
		require.True(t, util.EqualSlices(chainIDs, chainIDsBack))
	})
}
