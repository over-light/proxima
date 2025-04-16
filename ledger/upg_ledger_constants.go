package ledger

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
)

const _upgradeLedgerConstants = `
# definitions of main ledger constants
functions:
   -
      sym: constInitialSupply
      description: Initial number of tokens in the ledger
      source: u64/1000000000000000
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
      source: u64/80000000
   -
      sym: constMaxTickValuePerSlot
      description: maximum value of ticks in the slot. Usually 127
      source: u64/127
   -
      sym: ticksPerSlot64
      description: number of ticks in the slot. Usually 128
      source: u64/128
# inflation related
   -
      sym: constSlotInflationBase
      description: maximum inflation of the total supply in slot 0
      source: u64/33000000
   -
      sym: constLinearInflationSlots
      description: number of slot with linear inflation
      source: u64/3
   -
      sym: constBranchInflationBonusBase
      description: maximum value of the branch inflation bonus
      source: u64/5000000
   -
      sym: constMinimumAmountOnSequencer
      description: minimum amount of tokens on the sequencer output. For testnet it is 1000 * PRXI
      source: u64/1000000000
   -
      sym: constMaxNumberOfEndorsements
      description: up to 8 endorsements
      source: u64/8
   -
      sym: constPreBranchConsolidationTicks
      description: number of last ticks in a slot when sequencer transaction cannot consume more than 2 outputs
      source: u64/25
   -
      sym: constPostBranchConsolidationTicks
      description: number of first ticks in the timestamp of the sequencer transaction
      source: u64/12
   -
      sym: constTransactionPace
      description: minimum number of ticks between non-sequencer transaction and its inputs  
      source: u64/12
   -
      sym: constTransactionPaceSequencer
      description: minimum number of ticks between sequencer transaction and its inputs and endorsed transactions  
      source: u64/2
   -
      sym: constVBCost16
      description: constant for the storage deposit constraint  
      source: u16/1
   -
      sym: timeSlotSizeBytes
      description: constant for the storage deposit constraint  
      source: 4
   -
      sym: timestampByteSize
      description: constant for the storage deposit constraint  
      source: 5
`

func _upgradeLedgerConstantsYAML(genesisPublicKey ed25519.PublicKey, genesisTime uint64) []byte {
	return []byte(fmt.Sprintf(_upgradeLedgerConstants, hex.EncodeToString(genesisPublicKey), genesisTime))
}
