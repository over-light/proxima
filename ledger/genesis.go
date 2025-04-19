package ledger

import (
	"encoding/hex"

	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
	"golang.org/x/crypto/blake2b"
)

const (
	BootstrapSequencerName = "boot"
	// BoostrapSequencerIDHex is a constant
	BoostrapSequencerIDHex = "8739faa34a6902e49bc16455bbd642fd3c649e8959d97089e43f214ca57ea0e5"
)

// BoostrapSequencerID is a constant
var BoostrapSequencerID base.ChainID

// init BoostrapSequencerID constant and check consistency

func init() {
	data, err := hex.DecodeString(BoostrapSequencerIDHex)
	util.AssertNoError(err)
	BoostrapSequencerID, err = base.ChainIDFromBytes(data)
	util.AssertNoError(err)
	// calculate directly and check
	var zero33 [33]byte
	zero33[0] = 0b10000000
	genesisOutputID := base.GenesisOutputID()
	bootSeqIDDirect := blake2b.Sum256(genesisOutputID[:])
	util.Assertf(BoostrapSequencerID == bootSeqIDDirect, "BoostrapSequencerID must be equal to the blake2b hash of genesis output id, got %s", hex.EncodeToString(bootSeqIDDirect[:]))
	// more checks
	oid := base.GenesisOutputID()
	util.Assertf(base.MakeOriginChainID(oid) == BoostrapSequencerID, "MakeOriginChainID(&oid) == BoostrapSequencerID")
}

func GenesisOutput(initialSupply uint64, controllerAddress AddressED25519) *OutputWithChainID {
	oid := base.GenesisOutputID()
	return &OutputWithChainID{
		OutputWithID: OutputWithID{
			ID: oid,
			Output: NewOutput(func(o *Output) {
				o.WithAmount(initialSupply).WithLock(controllerAddress)
				chainIdx, err := o.PushConstraint(NewChainOrigin().Bytes())
				util.AssertNoError(err)
				_, err = o.PushConstraint(NewSequencerConstraint(chainIdx, initialSupply).Bytes())
				util.AssertNoError(err)

				msData := MilestoneData{Name: BootstrapSequencerName}
				idxMsData, err := o.PushConstraint(msData.AsConstraint().Bytes())
				util.AssertNoError(err)
				util.Assertf(idxMsData == MilestoneDataFixedIndex, "idxMsData == MilestoneDataFixedIndex")
			}),
		},
		ChainID: BoostrapSequencerID,
	}
}

func GenesisStemOutput() *OutputWithID {
	return &OutputWithID{
		ID: base.GenesisStemOutputID(),
		Output: NewOutput(func(o *Output) {
			o.WithAmount(0).
				WithLock(&StemLock{
					PredecessorOutputID: base.OutputID{},
				})
		}),
	}
}
