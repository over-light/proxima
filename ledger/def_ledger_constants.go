package ledger

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
)

type IdentityParameters struct {
	// arbitrary string up 255 bytes
	Description string
	// genesis time unix seconds
	GenesisTimeUnix uint32
	// initial supply of tokens
	InitialSupply uint64
	// ED25519 public key of the controller
	GenesisControllerPublicKey ed25519.PublicKey
	// time tick duration in nanoseconds
	TickDuration time.Duration
	// ----------- begin inflation-related
	SlotInflationBase    uint64 // constant C
	LinearInflationSlots uint64 // constant lambda
	// BranchInflationBonusBase inflation bonus
	BranchInflationBonusBase uint64
	// ----------- end inflation-related
	// VBCost
	VBCost uint64
	// number of ticks between non-sequencer transactions
	TransactionPace byte
	// number of ticks between sequencer transactions
	TransactionPaceSequencer byte
	// this limits number of sequencers in the network. Reasonable amount would be few hundreds of sequencers
	MinimumAmountOnSequencer uint64
	// limit maximum number of endorsements. For determinism
	MaxNumberOfEndorsements uint64
	// PreBranchConsolidationTicks enforces endorsement-only constraint for specified amount of ticks
	// before the slot boundary. It means, sequencer transaction can have only one input, its own predecessor
	// for any transaction with timestamp ticks > MaxTickValueInSlot - PreBranchConsolidationTicks
	// value 0 of PreBranchConsolidationTicks effectively means no constraint
	PreBranchConsolidationTicks  uint8
	PostBranchConsolidationTicks uint8
}

// default ledger constants

const (
	DefaultTickDuration = 80 * time.Millisecond

	DustPerProxi         = 1_000_000
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
	defaultDescription                  = "Proxima ledger definitions"
)

func init() {
	util.Assertf(DefaultInitialSupply/DefaultSlotInflationBase == 30_303_030, "wrong constants: DefaultInitialSupply/DefaultSlotInflationBase == 30_303_030")
}

func DefaultIdentityParameters(privateKey ed25519.PrivateKey, genesisTimeUnix uint32, description ...string) *IdentityParameters {
	//genesisTimeUnix := uint32(time.Now().Unix())

	dscr := defaultDescription
	if len(description) > 0 {
		dscr = description[0]
	}
	return &IdentityParameters{
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
		Description:                  dscr,
	}
}

func ConstantsYAMLFromIdentity(id *IdentityParameters) []byte {
	return []byte(fmt.Sprintf(__definitionsLedgerConstantsYAML,
		id.InitialSupply,
		hex.EncodeToString(id.GenesisControllerPublicKey),
		id.GenesisTimeUnix,
		uint64(id.TickDuration),
		base.MaxTickValue,
		id.SlotInflationBase,
		id.LinearInflationSlots,
		id.BranchInflationBonusBase,
		id.MinimumAmountOnSequencer,
		id.MaxNumberOfEndorsements,
		id.PreBranchConsolidationTicks,
		id.PostBranchConsolidationTicks,
		id.TransactionPace,
		id.TransactionPaceSequencer,
		id.VBCost,
		hex.EncodeToString([]byte(id.Description)),
	))
}

const __definitionsLedgerConstantsYAML = `
# definitions of main ledger constants
functions:
   -
      sym: constInitialSupply
      description: Initial number of tokens in the ledger
      source: u64/%d
   -
      sym: constGenesisControllerPublicKey
      description: Public key ED25519 of the genesis controller in hexadecimal format
      source: 0x%s
   -
      sym: constGenesisTimeUnix
      description: Unix time in seconds when ledger was initiated. Timestamp 0|0 corresponds to the genesis time 
      source: u64/%d
   -
      sym: constTickDuration
      description: tick duration in nanoseconds. Default is 80ms
      source: u64/%d
   -
      sym: constMaxTickValuePerSlot
      description: maximum value of ticks in the slot. Usually 127
      source: u64/%d
   -
      sym: ticksPerSlot64
      description: number of ticks in the slot. Usually 128
      source: add(constMaxTickValuePerSlot, u64/1)
# inflation related
   -
      sym: constSlotInflationBase
      description: maximum inflation of the total supply in slot 0. Usually 33000000 
      source: u64/%d
   -
      sym: constLinearInflationSlots
      description: number of slot with linear inflation
      source: u64/%d
   -
      sym: constBranchInflationBonusBase
      description: maximum value of the branch inflation bonus. Usually 5000000
      source: u64/%d
   -
      sym: constMinimumAmountOnSequencer
      description: minimum amount of tokens on the sequencer output. For testnet it is 1000 * PRXI = 1000000000
      source: u64/%d
   -
      sym: constMaxNumberOfEndorsements
      description: up to 8 endorsements
      source: u64/%d
   -
      sym: constPreBranchConsolidationTicks
      description: number of last ticks in a slot when sequencer transaction cannot consume more than 2 UTXOs
      source: u64/%d
   -
      sym: constPostBranchConsolidationTicks
      description: number of first ticks in the timestamp of the sequencer transaction
      source: u64/%d
   -
      sym: constTransactionPace
      description: minimum number of ticks between non-sequencer transaction and its inputs  
      source: u64/%d
   -
      sym: constTransactionPaceSequencer
      description: minimum number of ticks between sequencer transaction and its inputs and endorsed transactions  
      source: u64/%d
   -
      sym: constVBCost16
      description: constant for the storage deposit constraint  
      source: u16/%d
   -
      sym: constDescription
      description: arbitrary binary data  
      source: 0x%s
   -
      sym: timeSlotSizeBytes
      description: constant for the storage deposit constraint  
      source: 4
   -
      sym: timestampByteSize
      description: constant for the storage deposit constraint  
      source: 5
`
