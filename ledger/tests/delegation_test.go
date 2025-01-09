package tests

import (
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/ledger/txbuilder"
	"github.com/lunfardo314/proxima/ledger/utxodb"
	"github.com/lunfardo314/proxima/util"
	"github.com/stretchr/testify/require"
)

func TestIsOpenDelegationWindow(t *testing.T) {
	chainID := ledger.RandomChainID()
	t.Logf("chainID : %s", chainID.String())
	s := ledger.Slot(1)
	toggle := ledger.IsOpenDelegationSlot(chainID, s)
	for i := 0; i < 10; i++ {
		t.Logf("s = %d, open = %v", s, toggle)
		s += 1
		next := ledger.IsOpenDelegationSlot(chainID, s)
		require.True(t, toggle == !next)
		toggle = next
	}
}

func TestDelegation(t *testing.T) {
	var u *utxodb.UTXODB
	var delegationAddr, ownerAddr ledger.AddressED25519
	var delegationPrivateKey, ownerPrivateKey ed25519.PrivateKey

	const (
		tokensFromFaucet0 = 200_000_000_000
		tokensFromFaucet1 = 200_000_000_001
		delegatedTokens   = 100_000_000 // 150_000_000_000
	)
	var delegationLock *ledger.DelegationLock
	var txBytes []byte
	var delegatedOutput *ledger.OutputWithChainID
	initTest := func() ledger.ChainID {
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

		delegationLock = ledger.NewDelegationLock(addr[0], addr[1], 2, ledger.TimeNow(), delegatedTokens)
		txBytes, err = txbuilder.MakeSimpleTransferTransaction(par.
			WithAmount(delegatedTokens).
			WithTargetLock(delegationLock).
			WithConstraint(ledger.NewChainOrigin()),
		)
		require.NoError(t, err)

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
		chainID, _, _ := delegatedOutput.ExtractChainID()
		return chainID
	}
	transitDelegation := func(ts ledger.Time, inflate bool, nextDelegationAmount uint64, unlockByOwner bool, printtTx ...bool) error {
		cc, idx := delegatedOutput.Output.ChainConstraint()
		require.True(t, idx != 0xff)
		require.True(t, cc.IsOrigin())
		chainID, _, ok := delegatedOutput.ExtractChainID()
		require.True(t, ok)

		var inflation uint64
		if inflate {
			inflation = ledger.L().CalcChainInflationAmount(delegatedOutput.ID.Timestamp(), ts, delegatedOutput.Output.Amount())
		}
		t.Logf("inflation amount: %d", inflation)
		totalProducedAmount := delegatedOutput.Output.Amount() + inflation
		require.True(t, totalProducedAmount >= nextDelegationAmount)
		remainder := totalProducedAmount - nextDelegationAmount

		dl := delegatedOutput.Output.DelegationLock()
		require.True(t, dl != nil)

		txb := txbuilder.New()
		_, err := txb.ConsumeOutput(delegatedOutput.Output, delegatedOutput.ID)
		require.NoError(t, err)

		chainConstraint := ledger.NewChainConstraint(chainID, 0, idx, 0)
		succOut := ledger.NewOutput(func(o *ledger.Output) {
			o.WithAmount(nextDelegationAmount).
				WithLock(delegatedOutput.Output.DelegationLock())
			idx, _ = o.PushConstraint(chainConstraint.Bytes())
			ic := ledger.InflationConstraint{
				InflationAmount:      inflation,
				ChainConstraintIndex: idx,
			}
			if inflate {
				_, _ = o.PushConstraint(ic.Bytes())
			}
		})

		txb.PutUnlockParams(0, idx, ledger.NewChainUnlockParams(0, idx, 0))
		_, err = txb.ProduceOutput(succOut)
		require.NoError(t, err)

		txb.PutSignatureUnlock(0)

		if remainder > 0 {
			remOut := ledger.NewOutput(func(o *ledger.Output) {
				o.WithAmount(remainder)
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
		if len(printtTx) > 0 && printtTx[0] {
			t.Logf("------------------ delegation transition tx --------------\n%s", u.TxToString(txBytes))
		}

		return u.AddTransaction(txBytes)
	}
	t.Run("->delegated even, no inflation (ok)", func(t *testing.T) {
		chainID := initTest()
		t.Logf("delegated output 0:\n%s", delegatedOutput.Lines("      ").String())

		ts := delegatedOutput.ID.Timestamp().AddTicks(int(ledger.L().ID.TransactionPace))
		if !ledger.IsOpenDelegationSlot(chainID, ts.Slot()) {
			ts = ts.AddSlots(1)
		}
		err := transitDelegation(ts, false, delegatedOutput.Output.Amount(), false)
		require.NoError(t, err)

		rdr := multistate.MakeSugared(u.StateReader())
		outs, err := rdr.GetOutputsDelegatedToAccount(delegationAddr)
		require.NoError(t, err)
		require.EqualValues(t, 1, len(outs))

		delegatedOutput = outs[0]
		t.Logf("delegated output 1:\n%s", delegatedOutput.Lines("      ").String())
	})
	t.Run("->owner even no inflation (ok)", func(t *testing.T) {
		chainID := initTest()
		t.Logf("delegated output 0:\n%s", delegatedOutput.Lines("      ").String())

		ts := delegatedOutput.ID.Timestamp().AddTicks(int(ledger.L().ID.TransactionPace))
		if !ledger.IsOpenDelegationSlot(chainID, ts.Slot()) {
			ts = ts.AddSlots(1)
		}
		err := transitDelegation(ts, false, delegatedOutput.Output.Amount(), true)
		require.NoError(t, err)

		rdr := multistate.MakeSugared(u.StateReader())
		outs, err := rdr.GetOutputsDelegatedToAccount(delegationAddr)
		require.NoError(t, err)
		require.EqualValues(t, 1, len(outs))

		delegatedOutput = outs[0]
		t.Logf("delegated output 1:\n%s", delegatedOutput.Lines("      ").String())
	})
	t.Run("->delegated odd slot no inflation (not ok)", func(t *testing.T) {
		chainID := initTest()
		t.Logf("delegated output 0:\n%s", delegatedOutput.Lines("      ").String())

		ts := delegatedOutput.ID.Timestamp().AddTicks(int(ledger.L().ID.TransactionPace))
		if ledger.IsOpenDelegationSlot(chainID, ts.Slot()) {
			ts = ts.AddSlots(1)
		}
		err := transitDelegation(ts, false, delegatedOutput.Output.Amount(), false)
		require.True(t, err != nil && strings.Contains(err.Error(), "failed"))
	})
	t.Run("->owner odd slot no inflation (ok)", func(t *testing.T) {
		chainID := initTest()
		t.Logf("delegated output 0:\n%s", delegatedOutput.Lines("      ").String())

		ts := delegatedOutput.ID.Timestamp().AddTicks(int(ledger.L().ID.TransactionPace))
		if ledger.IsOpenDelegationSlot(chainID, ts.Slot()) {
			ts = ts.AddSlots(1)
		}
		err := transitDelegation(ts, false, delegatedOutput.Output.Amount(), true)
		require.NoError(t, err)
	})
	t.Run("-> delegation steal no inflation (not ok)", func(t *testing.T) {
		chainID := initTest()
		t.Logf("delegated output 0:\n%s", delegatedOutput.Lines("      ").String())

		ts := delegatedOutput.ID.Timestamp().AddTicks(int(ledger.L().ID.TransactionPace))
		if ledger.IsOpenDelegationSlot(chainID, ts.Slot()) {
			ts = ts.AddSlots(1)
		}
		err := transitDelegation(ts, false, delegatedOutput.Output.Amount()-100, false, true)
		require.True(t, err != nil, strings.Contains(err.Error(), "amount should not decrease"))
	})
	t.Run("-> owner not steal no inflation (ok)", func(t *testing.T) {
		chainID := initTest()
		t.Logf("delegated output 0:\n%s", delegatedOutput.Lines("      ").String())

		ts := delegatedOutput.ID.Timestamp().AddTicks(int(ledger.L().ID.TransactionPace))
		if !ledger.IsOpenDelegationSlot(chainID, ts.Slot()) {
			ts = ts.AddSlots(1)
		}
		err := transitDelegation(ts, false, delegatedOutput.Output.Amount()-100, true)
		require.NoError(t, err)
	})
	t.Run("-> delegate inflate1 (ok)", func(t *testing.T) {
		chainID := initTest()
		t.Logf("delegated output 0:\n%s", delegatedOutput.Lines("      ").String())

		tsPrev := delegatedOutput.ID.Timestamp()
		ts := tsPrev.AddTicks(int(ledger.L().ID.TransactionPace))
		ts = ts.AddSlots(10)
		if !ledger.IsOpenDelegationSlot(chainID, ts.Slot()) {
			ts = ts.AddSlots(1)
		}
		expectedInflation := ledger.L().CalcChainInflationAmount(tsPrev, ts, delegatedOutput.Output.Amount())
		t.Logf("tsIn: %s, tsOut: %s, amountiIn: %s -> expected inflation: %d",
			tsPrev.String(), ts.String(), util.Th(delegatedOutput.Output.Amount()), expectedInflation)

		err := transitDelegation(ts, true, delegatedOutput.Output.Amount()+expectedInflation, false)
		require.NoError(t, err)
	})
	t.Run("-> delegate inflate2 (ok)", func(t *testing.T) {
		initTest()
		t.Logf("delegated output 0:\n%s", delegatedOutput.Lines("      ").String())

		tsPrev := delegatedOutput.ID.Timestamp()
		ts := tsPrev.AddTicks(int(ledger.L().ID.TransactionPace))
		ts = ts.AddSlots(12)
		if ts.Slot()%2 != 0 {
			ts = ts.AddSlots(1)
		}
		expectedInflation := ledger.L().CalcChainInflationAmount(tsPrev, ts, delegatedOutput.Output.Amount())
		t.Logf("tsIn: %s, tsOut: %s, amountiIn: %s -> expected inflation: %d",
			tsPrev.String(), ts.String(), util.Th(delegatedOutput.Output.Amount()), expectedInflation)

		err := transitDelegation(ts, true, delegatedOutput.Output.Amount(), false, true)
		require.NoError(t, err)
	})
	t.Run("-> delegate inflate steal (not ok)", func(t *testing.T) {
		initTest()
		t.Logf("delegated output 0:\n%s", delegatedOutput.Lines("      ").String())

		tsPrev := delegatedOutput.ID.Timestamp()
		ts := tsPrev.AddTicks(int(ledger.L().ID.TransactionPace))
		ts = ts.AddSlots(10)
		if ts.Slot()%2 != 0 {
			ts = ts.AddSlots(1)
		}
		expectedInflation := ledger.L().CalcChainInflationAmount(tsPrev, ts, delegatedOutput.Output.Amount())
		t.Logf("tsIn: %s, tsOut: %s, amountiIn: %s -> expected inflation: %d",
			tsPrev.String(), ts.String(), util.Th(delegatedOutput.Output.Amount()), expectedInflation)

		err := transitDelegation(ts, true, delegatedOutput.Output.Amount()-5, false, true)
		t.Logf("failed with error: '%v'", err)
		require.True(t, err != nil && strings.Contains(err.Error(), "amount should not decrease"))
	})
}
