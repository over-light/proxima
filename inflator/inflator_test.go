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
	privateKeyOwner     ed25519.PrivateKey
	privateKeyDelegator ed25519.PrivateKey
	addrOwner           ledger.AddressED25519
	addrDelegator       ledger.AddressED25519
	utxodb              *utxodb.UTXODB
}

func (i *inflatorTestEnvironment) LatestReliableState() (multistate.SugaredStateReader, error) {
	//TODO implement me
	panic("implement me")
}

func (i *inflatorTestEnvironment) SubmitTxBytesFromInflator(txBytes []byte) {
	//TODO implement me
	panic("implement me")
}

func newEnvironment(t *testing.T) *inflatorTestEnvironment {
	ret := &inflatorTestEnvironment{
		t:          t,
		NodeGlobal: global.NewDefault(),
		utxodb:     utxodb.NewUTXODB(genesisPrivateKey, true),
	}
	privKey, _, addr := ret.utxodb.GenerateAddresses(0, 2)
	ret.privateKeyOwner = privKey[0]
	ret.privateKeyDelegator = privKey[1]
	ret.addrOwner = addr[0]
	ret.addrDelegator = addr[1]
	t.Logf("owner address: %s", ret.addrOwner.String())
	t.Logf("delegator address: %s", ret.addrDelegator.String())
	return ret
}

const (
	initOwnerLoad     = 1_000_000_000
	initDelegatorLoad = 1_000_000
)

func TestInflatorBase(t *testing.T) {
	env := newEnvironment(t)
	fl := New(env, Params{
		Target:            env.addrDelegator,
		PrivateKey:        env.privateKeyDelegator,
		TagAlongSequencer: ledger.RandomChainID(),
	})
	err := env.utxodb.TokensFromFaucet(env.addrOwner, initOwnerLoad)
	require.NoError(t, err)
	require.EqualValues(t, initOwnerLoad, env.utxodb.Balance(env.addrOwner))

	err = env.utxodb.TokensFromFaucet(env.addrDelegator, initDelegatorLoad)
	require.NoError(t, err)
	require.EqualValues(t, initDelegatorLoad, env.utxodb.Balance(env.addrDelegator))

	par, err := env.utxodb.MakeTransferInputData(env.privateKeyOwner, nil, ledger.TimeNow())
	_, err = env.utxodb.DoTransferOutputs(par.
		WithAmount(initOwnerLoad).
		WithTargetLock(ledger.NewDelegationLock(env.addrOwner, env.addrDelegator, 2)).
		WithConstraint(ledger.NewChainOrigin()),
	)
	require.NoError(t, err)

	rdr := multistate.MakeSugared(env.utxodb.StateReader())

	outs, err := rdr.GetOutputsDelegatedToAccount(env.addrDelegator)
	require.NoError(t, err)
	require.EqualValues(t, 1, len(outs))
	t.Logf("%s", outs[0].String())

	input := outs[0]
	t.Run("collect inputs", func(t *testing.T) {
		for s := 1; s <= 14; s++ {
			ts := input.Timestamp().AddSlots(ledger.Slot(s))
			lst, margin := fl.CollectInflatableTransitions(ts, rdr)
			t.Logf("+%d slots -- ts = %s, len(lst) = %d, margin = %s", s, ts.String(), len(lst), util.Th(margin))
			if len(lst) > 0 {
				require.True(t, ledger.IsOpenDelegationSlot(lst[0].ChainID, ts.Slot()))
				t.Logf("-------------------------\n%s", lst[0].Successor.Lines("   ").Join("\n"))
			}
			if uint64(s) > ledger.L().ID.ChainInflationOpportunitySlots {
				require.EqualValues(t, 0, margin)
			}
		}
	})
	t.Run("make tx", func(t *testing.T) {
		for s := 1; s <= 14; s++ {
			ts := input.Timestamp().AddSlots(ledger.Slot(s))
			tx, _, err := fl.MakeTransaction(ts, rdr)
			if errors.Is(err, ErrNoInputs) {
				continue
			}
			require.NoError(t, err)
			ctx, err := transaction.TxContextFromState(tx, rdr)
			require.NoError(t, err)
			t.Logf("+%d slots -- ts = %s --------------------\n%s", s, ts.String(), ctx.String())

			err = ctx.Validate()
			require.NoError(t, err)
		}
	})
}
