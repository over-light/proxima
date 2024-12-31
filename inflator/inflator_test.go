package inflator

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/ledger/utxodb"
	"github.com/stretchr/testify/require"
)

// initializes ledger.Library singleton for all tests and creates testing genesis private key

var genesisPrivateKey ed25519.PrivateKey

func init() {
	genesisPrivateKey = ledger.InitWithTestingLedgerIDData()
}

type inflatorTestEnvironment struct {
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

func newEnvironment() *inflatorTestEnvironment {
	ret := &inflatorTestEnvironment{
		NodeGlobal: global.NewDefault(),
		utxodb:     utxodb.NewUTXODB(genesisPrivateKey, true),
	}
	privKey, _, addr := ret.utxodb.GenerateAddresses(0, 2)
	ret.privateKeyOwner = privKey[0]
	ret.privateKeyDelegator = privKey[1]
	ret.addrOwner = addr[0]
	ret.addrDelegator = addr[1]
	return ret
}

func TestBase(t *testing.T) {
	env := newEnvironment()
	fl := New(env, Params{
		Target:            env.addrDelegator,
		PrivateKey:        env.privateKeyDelegator,
		TagAlongSequencer: ledger.ChainID{},
	})
	rdr := multistate.MakeSugared(env.utxodb.StateReader())
	_, _, err := fl.MakeTransaction(ledger.TimeNow().AddSlots(1), rdr)
	require.True(t, errors.Is(err, ErrNoInputs))
}
