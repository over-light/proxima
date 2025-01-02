package inflator

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/ledger/transaction"
	"github.com/lunfardo314/proxima/ledger/utxodb"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/lines"
	"github.com/stretchr/testify/require"
)

// initializes ledger.Library singleton for all tests and creates testing genesis private key
var genesisPrivateKey ed25519.PrivateKey

func init() {
	genesisPrivateKey = ledger.InitWithTestingLedgerIDData()
}

type inflatorTestEnvironment struct {
	t *testing.T
	global.NodeGlobal
	privateKeyOwner     []ed25519.PrivateKey
	addrOwner           []ledger.AddressED25519
	privateKeyDelegator ed25519.PrivateKey
	addrDelegator       ledger.AddressED25519
	utxodb              *utxodb.UTXODB
	fl                  *Inflator
}

func (env *inflatorTestEnvironment) LatestReliableState() (multistate.SugaredStateReader, error) {
	return multistate.MakeSugared(env.utxodb.StateReader()), nil
}

func (env *inflatorTestEnvironment) SubmitTxBytesFromInflator(txBytes []byte) {
	err := env.utxodb.AddTransaction(txBytes)
	require.NoError(env.t, err)
}

const (
	initOwnerLoad     = 2_000_000_000
	initDelegatorLoad = 1_000_000
)

func newEnvironment(t *testing.T, nOwners int, timeStepTicks int) (*inflatorTestEnvironment, ledger.Time) {
	ret := &inflatorTestEnvironment{
		t:          t,
		NodeGlobal: global.NewDefault(),
		utxodb:     utxodb.NewUTXODB(genesisPrivateKey, true),
	}

	privKey, _, addr := ret.utxodb.GenerateAddresses(0, nOwners+1)
	ret.privateKeyDelegator = privKey[0]
	ret.addrDelegator = addr[0]
	ret.privateKeyOwner = privKey[1:]
	ret.addrOwner = addr[1:]
	t.Logf("delegator address: %s", ret.addrDelegator.String())
	err := ret.utxodb.TokensFromFaucet(ret.addrDelegator, initDelegatorLoad)
	require.NoError(t, err)
	require.EqualValues(t, initDelegatorLoad, ret.utxodb.Balance(ret.addrDelegator))

	ts := ledger.TimeNow()
	for i := range ret.privateKeyOwner {
		err := ret.utxodb.TokensFromFaucet(ret.addrOwner[i], initOwnerLoad)
		require.NoError(t, err)
		require.EqualValues(t, initOwnerLoad, ret.utxodb.Balance(ret.addrOwner[i]))

		par, err := ret.utxodb.MakeTransferInputData(ret.privateKeyOwner[i], nil, ts)
		_, err = ret.utxodb.DoTransferOutputs(par.
			WithAmount(initOwnerLoad).
			WithTargetLock(ledger.NewDelegationLock(ret.addrOwner[i], ret.addrDelegator, 2)).
			WithConstraint(ledger.NewChainOrigin()),
		)
		require.NoError(t, err)
		ts = ts.AddTicks(timeStepTicks)
	}
	ret.fl = New(ret, Params{
		Target:            ret.addrDelegator,
		PrivateKey:        ret.privateKeyDelegator,
		TagAlongSequencer: ledger.RandomChainID(),
	})

	rdr := multistate.MakeSugared(ret.utxodb.StateReader())

	outs, err := rdr.GetOutputsDelegatedToAccount(ret.addrDelegator)
	require.NoError(t, err)
	require.EqualValues(t, nOwners, len(outs))
	t.Logf("%s", outs[0].String())

	return ret, outs[0].Timestamp()
}

func TestInflatorBase(t *testing.T) {
	t.Run("collect inputs", func(t *testing.T) {
		const nOwners = 5
		env, ts := newEnvironment(t, nOwners, 0)

		rdr := multistate.MakeSugared(env.utxodb.StateReader())

		for s := 1; s <= 14; s++ {
			tsTarget := ts.AddSlots(ledger.Slot(s))
			lst, margin := env.fl.collectInflatableTransitions(tsTarget, rdr)
			t.Logf("+%d slots -- ts = %s, len(lst) = %d, margin = %s", s, ts.String(), len(lst), util.Th(margin))
			if len(lst) > 0 {
				require.True(t, ledger.IsOpenDelegationSlot(lst[0].ChainID, tsTarget.Slot()))
				t.Logf("-------------------------\n%s", lst[0].Successor.Lines("   ").Join("\n"))
			}
		}
	})
	t.Run("make tx 1", func(t *testing.T) {
		const nOwners = 20
		env, ts := newEnvironment(t, nOwners, 0)

		const printtx = false
		maxMargin := uint64(0)
		var maxMarginTx *transaction.Transaction
		var maxCtx *transaction.TxContext

		rdr := multistate.MakeSugared(env.utxodb.StateReader())

		for s := 1; s <= 14; s++ {
			tsTarget := ts.AddSlots(ledger.Slot(s))
			tx, _, margin, err := env.fl.MakeTransaction(tsTarget, rdr)
			if errors.Is(err, ErrNoInputs) {
				continue
			}
			require.NoError(t, err)
			ctx, err := transaction.TxContextFromState(tx, rdr)
			require.NoError(t, err)
			if margin > maxMargin {
				maxMargin = margin
				maxMarginTx = tx
				maxCtx = ctx
			}

			if printtx {
				t.Logf("+%d slots -- ts = %s, marging collected: %s -- %s\n--------------------- %s",
					s, ts.String(), util.Th(margin), tx.IDShortString(), ctx.String())
			} else {
				t.Logf("+%d slots -- ts = %s, marging collected: %s -- %s", s, ts.String(), util.Th(margin), tx.IDShortString())
			}

			err = ctx.Validate()
			require.NoError(t, err)
		}
		if maxMarginTx != nil {
			err := env.utxodb.AddTransaction(maxMarginTx.Bytes())
			require.NoError(t, err)
			t.Logf("============================================\n%s", maxCtx.String())
		}
	})
	t.Run("make tx 2", func(t *testing.T) {
		const nOwners = 20
		env, ts := newEnvironment(t, nOwners, 30)

		const printtx = false
		maxMargin := uint64(0)
		var maxMarginTx *transaction.Transaction
		var maxCtx *transaction.TxContext

		rdr := multistate.MakeSugared(env.utxodb.StateReader())

		for s := 1; s <= 14; s++ {
			tsTarget := ts.AddSlots(ledger.Slot(s))
			tx, _, margin, err := env.fl.MakeTransaction(tsTarget, rdr)
			if errors.Is(err, ErrNoInputs) {
				continue
			}
			require.NoError(t, err)
			ctx, err := transaction.TxContextFromState(tx, rdr)
			require.NoError(t, err)
			if margin > maxMargin {
				maxMargin = margin
				maxMarginTx = tx
				maxCtx = ctx
			}

			if printtx {
				t.Logf("+%d slots -- ts = %s, marging collected: %s -- %s\n--------------------- %s",
					s, ts.String(), util.Th(margin), tx.IDShortString(), ctx.String())
			} else {
				t.Logf("+%d slots -- ts = %s, marging collected: %s -- %s", s, ts.String(), util.Th(margin), tx.IDShortString())
			}

			err = ctx.Validate()
			require.NoError(t, err)
		}
		if maxMarginTx != nil {
			err := env.utxodb.AddTransaction(maxMarginTx.Bytes())
			require.NoError(t, err)
			t.Logf("============================================\n%s", maxCtx.String())
		}
	})
}

func TestInflatorRun(t *testing.T) {
	t.Run("several steps", func(t *testing.T) {
		const (
			nOwners = 20
			steps   = 100
		)
		env, _ := newEnvironment(t, nOwners, 30)

		targetTs := ledger.TimeNow()
		for s := 1; s <= steps; s++ {
			env.fl.doStep(targetTs)
			targetTs = targetTs.AddTicks(260)
		}

		lrb := multistate.MakeSugared(env.utxodb.StateReader())
		outs, err := lrb.GetOutputsDelegatedToAccount(env.addrDelegator)
		require.NoError(t, err)
		ln := lines.New("     ")
		for _, o := range outs {
			ln.Add("%s   %s   %s", o.ChainID.StringShort(), o.ID.StringShort(), util.Th(o.Output.Amount()))
		}
		t.Logf("----------------------- final delegated outputs:\n%s", ln.String())
	})
}
