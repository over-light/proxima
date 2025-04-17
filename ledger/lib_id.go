package ledger

import (
	"crypto/ed25519"
	"encoding/binary"
	"math"
	"time"

	"github.com/lunfardo314/easyfl"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/testutil"
)

type (
	Library struct {
		*easyfl.Library
		ID                 *IdentityData
		constraintByPrefix map[string]*constraintRecord
		constraintNames    map[string]struct{}
		inlineTests        []func()
	}

	LibraryConst struct {
		*Library
	}
)

// default ledger constants

const (
	DefaultTickDuration = 80 * time.Millisecond

	DustPerProxi = 1_000_000
	//BaseTokenName        = "Proxi"
	//BaseTokenNameTicker  = "PRXI"
	//DustTokenName        = "dust"
	PRXI                 = DustPerProxi
	InitialSupplyProxi   = 1_000_000_000
	DefaultInitialSupply = InitialSupplyProxi * PRXI

	// -------------- begin inflation-related
	// default inflation constants adjusted to the annual inflation cap of approx 12-13% first year

	DefaultSlotInflationBase        = 33_000_000
	DefaultLinearInflationSlots     = 3
	DefaultBranchInflationBonusBase = 5_000_000
	// used to enforce approx validity of defaults

	// -------------- end inflation-related

	DefaultVBCost                   = 1
	DefaultTransactionPace          = 12
	DefaultTransactionPaceSequencer = 2
	// DefaultMinimumAmountOnSequencer Reasonable limit could be 1/1000 of initial supply
	DefaultMinimumAmountOnSequencer     = 1_000 * PRXI // this is testnet default
	DefaultMaxNumberOfEndorsements      = 8
	DefaultPreBranchConsolidationTicks  = 25
	DefaultPostBranchConsolidationTicks = 12
)

func init() {
	util.Assertf(DefaultInitialSupply/DefaultSlotInflationBase == 30_303_030, "wrong constants: DefaultInitialSupply/DefaultSlotInflationBase == 30_303_030")
}

func DefaultIdentityData(privateKey ed25519.PrivateKey) *IdentityData {
	genesisTimeUnix := uint32(time.Now().Unix())

	return &IdentityData{
		GenesisTimeUnix:              genesisTimeUnix,
		GenesisControllerPublicKey:   privateKey.Public().(ed25519.PublicKey),
		InitialSupply:                DefaultInitialSupply,
		TickDuration:                 DefaultTickDuration,
		VBCost:                       DefaultVBCost,
		TransactionPace:              DefaultTransactionPace,
		TransactionPaceSequencer:     DefaultTransactionPaceSequencer,
		BranchInflationBonusBase:     DefaultBranchInflationBonusBase,
		SlotInflationBase:            DefaultSlotInflationBase,
		LinearInflationSlots:         DefaultLinearInflationSlots,
		MinimumAmountOnSequencer:     DefaultMinimumAmountOnSequencer,
		MaxNumberOfEndorsements:      DefaultMaxNumberOfEndorsements,
		PreBranchConsolidationTicks:  DefaultPreBranchConsolidationTicks,
		PostBranchConsolidationTicks: DefaultPostBranchConsolidationTicks,
		Description:                  "Proxima test ledger",
	}
}

func newBaseLibrary() *Library {
	ret := &Library{
		Library:            easyfl.NewBaseLibrary(),
		constraintByPrefix: make(map[string]*constraintRecord),
		constraintNames:    make(map[string]struct{}),
		inlineTests:        make([]func(), 0),
	}
	return ret
}

func (lib *Library) Const() LibraryConst {
	return LibraryConst{lib}
}

func GetTestingIdentityData(seed ...int) (*IdentityData, ed25519.PrivateKey) {
	s := 10000
	if len(seed) > 0 {
		s = seed[0]
	}
	pk := testutil.GetTestingPrivateKey(1, s)
	return DefaultIdentityData(pk), pk
}

func (id *IdentityData) SetTickDuration(d time.Duration) {
	id.TickDuration = d
}

// Library constants

func (lib LibraryConst) TicksPerSlot() byte {
	bin, err := lib.EvalFromSource(nil, "ticksPerSlot")
	util.AssertNoError(err)
	return bin[0]
}

func (lib LibraryConst) MinimumAmountOnSequencer() uint64 {
	bin, err := lib.EvalFromSource(nil, "constMinimumAmountOnSequencer")
	util.AssertNoError(err)
	ret := binary.BigEndian.Uint64(bin)
	util.Assertf(ret < math.MaxUint32, "ret < math.MaxUint32")
	return ret
}

func (lib LibraryConst) TicksPerInflationEpoch() uint64 {
	bin, err := lib.EvalFromSource(nil, "ticksPerInflationEpoch")
	util.AssertNoError(err)
	return binary.BigEndian.Uint64(bin)
}
