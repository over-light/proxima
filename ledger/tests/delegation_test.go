package tests

import (
	"crypto/ed25519"
	"testing"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/txbuilder"
	"github.com/lunfardo314/proxima/multistate"
	"github.com/lunfardo314/proxima/util/utxodb"
	"github.com/stretchr/testify/require"
)

func TestDelegation(t *testing.T) {
	var privKey []ed25519.PrivateKey
	var u *utxodb.UTXODB
	var addr []ledger.AddressED25519

	const (
		tokensFromFaucet0 = 10_000_000
		tokensFromFaucet1 = 10_000_001
		delegatedTokens   = 2_000_000
	)
	initTest := func() {
		u = utxodb.NewUTXODB(genesisPrivateKey, true)
		privKey, _, addr = u.GenerateAddresses(0, 2)
		err := u.TokensFromFaucet(addr[0], tokensFromFaucet0)
		require.NoError(t, err)
		err = u.TokensFromFaucet(addr[1], tokensFromFaucet1)
		require.NoError(t, err)
	}

	t.Run("1", func(t *testing.T) {
		initTest()
		par, err := u.MakeTransferInputData(privKey[0], nil, ledger.NilLedgerTime)
		require.NoError(t, err)

		lock := ledger.NewDelegationLock(addr[0], addr[1], 2)
		txBytes, err := txbuilder.MakeSimpleTransferTransaction(par.
			WithAmount(delegatedTokens).
			WithTargetLock(lock).
			WithConstraint(ledger.NewChainOrigin()),
		)
		require.NoError(t, err)
		t.Logf(u.TxToString(txBytes))

		err = u.AddTransaction(txBytes)
		require.NoError(t, err)

		require.EqualValues(t, 1, u.NumUTXOs(u.GenesisControllerAddress()))
		require.EqualValues(t, u.Supply()-u.FaucetBalance()-tokensFromFaucet0-tokensFromFaucet1, u.Balance(u.GenesisControllerAddress()))
		require.EqualValues(t, tokensFromFaucet0, u.Balance(addr[0]))
		require.EqualValues(t, 2, u.NumUTXOs(addr[0]))
		require.EqualValues(t, 2, u.NumUTXOs(addr[1]))

		rdr := multistate.MakeSugared(u.StateReader())

		outs, err := rdr.GetOutputsDelegatedToAccount(addr[0])
		require.NoError(t, err)
		require.EqualValues(t, 0, len(outs))

		outs, err = rdr.GetOutputsDelegatedToAccount(addr[1])
		require.NoError(t, err)
		require.EqualValues(t, 1, len(outs))

		t.Logf("delegated output to addr %s:\n%s", addr[1].Short(), outs[0].Lines("      ").String())

		ts := outs[0].ID.Timestamp().AddTicks(int(ledger.L().ID.TransactionPace))
		if ts.Slot()%2 != 0 {
			ts = ts.AddSlots(1)
		}

		cc, idx := outs[0].Output.ChainConstraint()
		require.True(t, idx != 0xff)
		require.True(t, cc.IsOrigin())
		chainID, _, ok := outs[0].ExtractChainID()
		require.True(t, ok)

		chainConstraint := ledger.NewChainConstraint(chainID, 0, idx, 0)
		succOut := ledger.NewOutput(func(o *ledger.Output) {
			o.WithAmount(outs[0].Output.Amount()).
				WithLock(outs[0].Output.DelegationLock())
			idx, _ = o.PushConstraint(chainConstraint.Bytes())
		})
		require.NoError(t, err)

		txb := txbuilder.NewTransactionBuilder()
		_, err = txb.ConsumeOutput(outs[0].Output, outs[0].ID)
		require.NoError(t, err)
		txb.PutUnlockParams(0, idx, ledger.NewChainUnlockParams(0, idx, 0))
		_, err = txb.ProduceOutput(succOut)
		require.NoError(t, err)

		txb.PutSignatureUnlock(0)

		txb.TransactionData.Timestamp = ts
		txb.TransactionData.InputCommitment = txb.InputCommitment()
		txb.SignED25519(privKey[1])
		txBytes = txb.TransactionData.Bytes()
		t.Logf("next delegated tx:\n%s", u.TxToString(txBytes))

		err = u.AddTransaction(txBytes)
		require.NoError(t, err)
	})
}
