package tests

import (
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/txbuilder"
	"github.com/lunfardo314/proxima/multistate"
	"github.com/lunfardo314/proxima/util/utxodb"
	"github.com/stretchr/testify/require"
)

func TestDelegation(t *testing.T) {
	var u *utxodb.UTXODB
	var delegationAddr, ownerAddr ledger.AddressED25519
	var delegationPrivateKey, ownerPrivateKey ed25519.PrivateKey

	const (
		tokensFromFaucet0 = 200_000_000_000
		tokensFromFaucet1 = 200_000_000_001
		delegatedTokens   = 150_000_000_000
	)
	var delegationLock *ledger.DelegationLock
	var txBytes []byte
	var delegatedOutput *ledger.OutputWithChainID
	initTest := func() {
		u = utxodb.NewUTXODB(genesisPrivateKey, true)

		privKey, _, addr := u.GenerateAddresses(0, 2)
		ownerPrivateKey = privKey[0]
		delegationPrivateKey = privKey[1]
		ownerAddr = addr[0]
		delegationAddr = addr[1]
		t.Logf("\n==== owner    : %s\n==== delegator: %s", ownerAddr.String(), delegationAddr.String())

		err := u.TokensFromFaucet(addr[0], tokensFromFaucet0)
		require.NoError(t, err)
		err = u.TokensFromFaucet(addr[1], tokensFromFaucet1)
		require.NoError(t, err)

		par, err := u.MakeTransferInputData(privKey[0], nil, ledger.NilLedgerTime)
		require.NoError(t, err)

		delegationLock = ledger.NewDelegationLock(addr[0], addr[1], 2)
		txBytes, err = txbuilder.MakeSimpleTransferTransaction(par.
			WithAmount(delegatedTokens).
			WithTargetLock(delegationLock).
			WithConstraint(ledger.NewChainOrigin()),
		)
		require.NoError(t, err)
		//t.Logf(u.TxToString(txBytes))

		err = u.AddTransaction(txBytes)
		require.NoError(t, err)

		require.EqualValues(t, 1, u.NumUTXOs(u.GenesisControllerAddress()))
		require.EqualValues(t, u.Supply()-u.FaucetBalance()-tokensFromFaucet0-tokensFromFaucet1, u.Balance(u.GenesisControllerAddress()))
		require.EqualValues(t, tokensFromFaucet0, u.Balance(ownerAddr))
		require.EqualValues(t, 2, u.NumUTXOs(ownerAddr))
		require.EqualValues(t, 2, u.NumUTXOs(delegationAddr))

		rdr := multistate.MakeSugared(u.StateReader())

		outs, err := rdr.GetOutputsDelegatedToAccount(ownerAddr)
		require.NoError(t, err)
		require.EqualValues(t, 0, len(outs))

		outs, err = rdr.GetOutputsDelegatedToAccount(delegationAddr)
		require.NoError(t, err)
		require.EqualValues(t, 1, len(outs))

		delegatedOutput = outs[0]
	}
	transitDelegation := func(ts ledger.Time, nextAmount uint64, unlockByOwner bool) ([]byte, error) {
		cc, idx := delegatedOutput.Output.ChainConstraint()
		require.True(t, idx != 0xff)
		require.True(t, cc.IsOrigin())
		chainID, _, ok := delegatedOutput.ExtractChainID()
		require.True(t, ok)

		remainder := int64(nextAmount) - int64(delegatedOutput.Output.Amount())

		dl := delegatedOutput.Output.DelegationLock()
		require.True(t, dl != nil)

		txb := txbuilder.NewTransactionBuilder()
		_, err := txb.ConsumeOutput(delegatedOutput.Output, delegatedOutput.ID)
		require.NoError(t, err)

		chainConstraint := ledger.NewChainConstraint(chainID, 0, idx, 0)
		succOut := ledger.NewOutput(func(o *ledger.Output) {
			o.WithAmount(nextAmount).
				WithLock(delegatedOutput.Output.DelegationLock())
			idx, _ = o.PushConstraint(chainConstraint.Bytes())
			if remainder > 0 {
				ic := ledger.InflationConstraint{
					ChainInflation:       uint64(remainder),
					ChainConstraintIndex: idx,
				}
				_, _ = o.PushConstraint(ic.Bytes())
			}
		})

		txb.PutUnlockParams(0, idx, ledger.NewChainUnlockParams(0, idx, 0))
		_, err = txb.ProduceOutput(succOut)
		require.NoError(t, err)

		txb.PutSignatureUnlock(0)

		if remainder < 0 {
			remOut := ledger.NewOutput(func(o *ledger.Output) {
				o.WithAmount(uint64(-remainder))
				if unlockByOwner {
					o.WithLock(dl.OwnerLock.AsLock())
				} else {
					o.WithLock(dl.TargetLock.AsLock())
				}
			})
			_, err = txb.ProduceOutput(remOut)
			require.NoError(t, err)
		}

		txb.TransactionData.Timestamp = ts
		txb.TransactionData.InputCommitment = txb.InputCommitment()
		if unlockByOwner {
			txb.SignED25519(ownerPrivateKey)
		} else {
			txb.SignED25519(delegationPrivateKey)
		}
		txBytes = txb.TransactionData.Bytes()
		//t.Logf("next delegated tx:\n%s", u.TxToString(txBytes))

		return txBytes, u.AddTransaction(txBytes)
	}
	t.Run("->delegated even (ok)", func(t *testing.T) {
		initTest()
		t.Logf("delegated output 0:\n%s", delegatedOutput.Lines("      ").String())

		ts := delegatedOutput.ID.Timestamp().AddTicks(int(ledger.L().ID.TransactionPace))
		if ts.Slot()%2 != 0 {
			ts = ts.AddSlots(1)
		}
		_, err := transitDelegation(ts, delegatedOutput.Output.Amount(), false)
		require.NoError(t, err)

		rdr := multistate.MakeSugared(u.StateReader())
		outs, err := rdr.GetOutputsDelegatedToAccount(delegationAddr)
		require.NoError(t, err)
		require.EqualValues(t, 1, len(outs))

		delegatedOutput = outs[0]
		t.Logf("delegated output 1:\n%s", delegatedOutput.Lines("      ").String())
	})
	t.Run("->owner even (ok)", func(t *testing.T) {
		initTest()
		t.Logf("delegated output 0:\n%s", delegatedOutput.Lines("      ").String())

		ts := delegatedOutput.ID.Timestamp().AddTicks(int(ledger.L().ID.TransactionPace))
		if ts.Slot()%2 != 0 {
			ts = ts.AddSlots(1)
		}
		_, err := transitDelegation(ts, delegatedOutput.Output.Amount(), true)
		require.NoError(t, err)

		rdr := multistate.MakeSugared(u.StateReader())
		outs, err := rdr.GetOutputsDelegatedToAccount(delegationAddr)
		require.NoError(t, err)
		require.EqualValues(t, 1, len(outs))

		delegatedOutput = outs[0]
		t.Logf("delegated output 1:\n%s", delegatedOutput.Lines("      ").String())
	})
	t.Run("->delegated odd slot (not ok)", func(t *testing.T) {
		initTest()
		t.Logf("delegated output 0:\n%s", delegatedOutput.Lines("      ").String())

		ts := delegatedOutput.ID.Timestamp().AddTicks(int(ledger.L().ID.TransactionPace))
		if ts.Slot()%2 == 0 {
			ts = ts.AddSlots(1)
		}
		_, err := transitDelegation(ts, delegatedOutput.Output.Amount(), false)
		require.True(t, err != nil && strings.Contains(err.Error(), "failed"))
	})
	t.Run("->owner odd slot (ok)", func(t *testing.T) {
		initTest()
		t.Logf("delegated output 0:\n%s", delegatedOutput.Lines("      ").String())

		ts := delegatedOutput.ID.Timestamp().AddTicks(int(ledger.L().ID.TransactionPace))
		if ts.Slot()%2 == 0 {
			ts = ts.AddSlots(1)
		}
		_, err := transitDelegation(ts, delegatedOutput.Output.Amount(), true)
		require.NoError(t, err)
	})
	t.Run("-> delegation steal (not ok)", func(t *testing.T) {
		initTest()
		t.Logf("delegated output 0:\n%s", delegatedOutput.Lines("      ").String())

		ts := delegatedOutput.ID.Timestamp().AddTicks(int(ledger.L().ID.TransactionPace))
		if ts.Slot()%2 != 0 {
			ts = ts.AddSlots(1)
		}
		txBytes, err := transitDelegation(ts, delegatedOutput.Output.Amount()-100, false)
		require.True(t, err != nil, strings.Contains(err.Error(), "amount should not decrease"))

		t.Logf("failed with: '%v'\n------------ failed transaction ---------\n%s", err, u.TxToString(txBytes))
	})
	t.Run("-> owner not steal (ok)", func(t *testing.T) {
		initTest()
		t.Logf("delegated output 0:\n%s", delegatedOutput.Lines("      ").String())

		ts := delegatedOutput.ID.Timestamp().AddTicks(int(ledger.L().ID.TransactionPace))
		if ts.Slot()%2 != 0 {
			ts = ts.AddSlots(1)
		}
		_, err := transitDelegation(ts, delegatedOutput.Output.Amount()-100, true)
		require.NoError(t, err)
	})
	t.Run("-> delegate inflate (ok)", func(t *testing.T) {
		initTest()
		t.Logf("delegated output 0:\n%s", delegatedOutput.Lines("      ").String())

		ts := delegatedOutput.ID.Timestamp().AddTicks(int(ledger.L().ID.TransactionPace))
		tsPrev := ts
		ts = ts.AddSlots(10)
		if ts.Slot()%2 != 0 {
			ts = ts.AddSlots(1)
		}
		expectedInflation := ledger.L().CalcChainInflationAmount(tsPrev, ts, delegatedOutput.Output.Amount(), 0)
		t.Logf("expected inflation: %d", expectedInflation)

		txBytes, err := transitDelegation(ts, delegatedOutput.Output.Amount()+expectedInflation, false)
		t.Logf("------------ failed transaction ---------\n%s", u.TxToString(txBytes))
		require.NoError(t, err)
	})
}
