package ledger

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/lunfardo314/easyfl"
)

// Inflation constraint script, when added to the chain-constrained output, adds inflation the transaction
// It enforces
// - valid inflation value on the chain inside slot (proportional capital an time)
// - valid branch inflation bonus for branches.
//   It is enforced to be provably random, generated by VRF for the sequencer's private key and slot number

const (
	InflationConstraintName = "inflation"
	// (0) chain constraint index, (1) inflation amount or randomness proof
	inflationConstraintTemplate = InflationConstraintName + "(%d, %s, %s, %s)"
)

type InflationConstraint struct {
	// ChainConstraintIndex must point to the sibling chain constraint
	ChainConstraintIndex byte
	// ChainInflation inflation amount calculated according to chain inflation rule. It is used inside slot and delayed on slot boundary
	// and can be added to the inflation of the next transaction in the chain
	ChainInflation uint64
	// VRFProof VRF randomness proof, used to proof VRF and calculate inflation amount on branch
	// nil for non-branch transactions
	VRFProof []byte
	// DelayedInflationIndex
	// Used only if branch successor to enforce correct ChainInflation which will sum of delayed inflation and current inflation
	// If not used, must be 0xff
	DelayedInflationIndex byte
}

// NewInflationConstraintInsideSlot inflation constraint for chain output inside slot
func NewInflationConstraintInsideSlot(chainConstraintIndex byte, chainInflation uint64, delayedInflationIndex byte) *InflationConstraint {
	return &InflationConstraint{
		ChainConstraintIndex:  chainConstraintIndex,
		ChainInflation:        chainInflation,
		DelayedInflationIndex: delayedInflationIndex,
	}
}

// NewInflationConstraintOnSlotBoundary inflation constraint for chain output on the slot boundary
func NewInflationConstraintOnSlotBoundary(chainConstraintIndex byte, chainInflation uint64, vrfProof []byte) *InflationConstraint {
	return &InflationConstraint{
		ChainConstraintIndex: chainConstraintIndex,
		ChainInflation:       chainInflation,
		VRFProof:             vrfProof,
	}
}

func (i *InflationConstraint) Name() string {
	return InflationConstraintName
}

func (i *InflationConstraint) Bytes() []byte {
	return mustBinFromSource(i.source())
}

func (i *InflationConstraint) String() string {
	return i.source()
}

func (i *InflationConstraint) source() string {
	var chainInflationBin [8]byte
	binary.BigEndian.PutUint64(chainInflationBin[:], i.ChainInflation)
	chainInflationStr := "0x" + hex.EncodeToString(chainInflationBin[:])

	vrfProofStr := "0x" + hex.EncodeToString(i.VRFProof)
	delayedInflationIndexStr := "0x"
	if i.DelayedInflationIndex != 0xff {
		delayedInflationIndexStr = fmt.Sprintf("%d", i.DelayedInflationIndex)
	}
	return fmt.Sprintf(inflationConstraintTemplate, i.ChainConstraintIndex, chainInflationStr, vrfProofStr, delayedInflationIndexStr)
}

// InflationAmount calculates inflation amount either inside slot, or on the slot boundary
func (i *InflationConstraint) InflationAmount(slotBoundary bool) uint64 {
	if slotBoundary {
		// the ChainInflation is interpreted as delayed inflation
		return L().ID.BranchInflationBonusFromRandomnessProof(i.VRFProof)
	}
	return i.ChainInflation
}

func InflationConstraintFromBytes(data []byte) (*InflationConstraint, error) {
	sym, _, args, err := L().ParseBytecodeOneLevel(data, 4)
	if err != nil {
		return nil, err
	}
	if sym != InflationConstraintName {
		return nil, fmt.Errorf("InflationConstraintFromBytes: not a inflation constraint script")
	}
	cciBin := easyfl.StripDataPrefix(args[0])
	if len(cciBin) != 1 {
		return nil, fmt.Errorf("InflationConstraintFromBytes: wrong chainConstraintIndex parameter")
	}
	cci := cciBin[0]

	var amount uint64
	amountBin := easyfl.StripDataPrefix(args[1])
	if len(amountBin) != 8 {
		return nil, fmt.Errorf("InflationConstraintFromBytes: wrong chainInflation parameter")
	}
	amount = binary.BigEndian.Uint64(amountBin)

	delayedInflationIndex := byte(0xff)
	idxBin := easyfl.StripDataPrefix(args[3])
	switch {
	case len(idxBin) == 1:
		delayedInflationIndex = idxBin[0]
	case len(idxBin) > 1:
		return nil, fmt.Errorf("InflationConstraintFromBytes: wrong delayed inflation index parameter")
	}

	return &InflationConstraint{
		ChainConstraintIndex:  cci,
		ChainInflation:        amount,
		VRFProof:              easyfl.StripDataPrefix(args[3]),
		DelayedInflationIndex: delayedInflationIndex,
	}, nil
}

func addInflationConstraint(lib *Library) {
	lib.MustExtendMany(inflationFunctionsSource)
	lib.extendWithConstraint(InflationConstraintName, inflationConstraintSource, 4, func(data []byte) (Constraint, error) {
		return InflationConstraintFromBytes(data)
	}, initTestInflationConstraint)
}

func initTestInflationConstraint() {
	//data := []byte("123")
	//example := NewInflationConstraint(4, data)
	//sym, _, args, err := L().ParseBytecodeOneLevel(example.Bytes(), 2)
	//util.AssertNoError(err)
	//util.Assertf(sym == InflationConstraintName, "sym == InflationConstraintName")
	//
	//cciBin := easyfl.StripDataPrefix(args[0])
	//util.Assertf(len(cciBin) == 1, "len(cciBin) == 1")
	//util.Assertf(cciBin[0] == 4, "cciBin[0] == 4")
	//
	//amountBin := easyfl.StripDataPrefix(args[1])
	//util.Assertf(bytes.Equal(amountBin, data), "bytes.Equal(amountBin, data)")
}

const inflationConstraintSource = `

// $0 - predecessor input index
// $1 - inflation value
// checks if inflation value is below cap, calculated for the chain constrained output
// from time delta and amount on predecessor
func _validChainInflationValue : or(
	isZero($1), // zero always ok
    lessOrEqualThan(
       $1,
       maxChainInflationAmount(
			timestampOfInputByIndex($0), 
			txTimestampBytes, 
			amountValue(consumedOutputByIndex($0))
		)
    )
)

// $0 - chain constraint index (sibling)
// $1 - inflation amount 8 bytes
// $2 - delayed inflation index
// checks if inflation amount is valid for non-branch transaction

func _checkChainInflation :

TODO 
	_validChainInflationValue(
		predecessorInputIndexFromChainData(
			evalArgumentBytecode( selfSiblingConstraint($0), #chain, 0)
		),
		$1
	)

// $0 - randomness proof
// checks inflation data is a randomness proof, valid for the stem predecessor (as message) and with public key of the sender
// randomness proof will be used to calculate branch inflation bonus in the range between 0 and constBranchBonusBase + 1
// 
// Stem predecessor is used as a message to make same sequencer have different random inflation on different forks.
// Using slot as a message makes some inflation of same slot for different branches. This may lead to 'nothing-at-stake'
// situation

func _checkBranchInflationBonus :
	require(
		vrfVerify(publicKeyED25519(txSignature), $0, predStemOutputIDOfSelf),
		!!!VRF_verification_failed
	)


// inflation(<chain constraint index>, <inflation amount>, <VRF proof>, <delayed inflation index>)
// $0 - chain constraint index (sibling)
// $1 - chain inflation amount (8 bytes). On slot boundary interpreted as delayed inflation 
// $2 - on slot boundary interpreted as vrf proof. Interpreted only on branch transactions
// $3 - inflation constraint index in the predecessor, only checked on branch successor
//
// Enforces:
// - the output is chain-constrained
// - if $1 is nil, always ok zero inflation
// - correct amount of inflation inside slot (non-branch) for all chains
// - correct branch inflation bonus. It must be provably random data for the slot and the sender's public key 
func inflation : or(
	selfIsConsumedOutput, // not checked if consumed
	isZero($1),           // zero inflation always ok
	and(
  		selfIsProducedOutput,
		_checkChainInflation($0, $1, $3),
		if(
			isBranchTransaction,
			_checkBranchInflationBonus($2),
		)
    )
)
`
