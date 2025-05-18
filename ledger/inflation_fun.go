package ledger

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync/atomic"

	"github.com/lunfardo314/easyfl"
	"github.com/lunfardo314/easyfl/easyfl_util"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
)

// This file contains definitions of the inflation calculation functions in EasyFL (on-ledger)
// The Go functions interprets EasyFL function to guarantee consistent values

var calcChainInflationAmountExpression atomic.Pointer[easyfl.Expression]

func __precompiledChainInflation() (ret *easyfl.Expression) {
	if ret = calcChainInflationAmountExpression.Load(); ret == nil {
		var err error
		ret, _, _, err = L().CompileExpression("calcChainInflationAmount($0,$1,$2)")
		util.AssertNoError(err)
		calcChainInflationAmountExpression.Store(ret)
	}
	return ret
}

// CalcChainInflationAmount interprets EasyFl formula. Return chain inflation amount for given in and out ledger times,
// input amount of tokens and delayed
func (lib *Library) CalcChainInflationAmount(inTs, outTs base.LedgerTime, inAmount uint64) uint64 {
	var amountBin [8]byte
	binary.BigEndian.PutUint64(amountBin[:], inAmount)
	ret := easyfl.EvalExpression(nil, __precompiledChainInflation(), inTs.Bytes(), outTs.Bytes(), amountBin[:])
	return binary.BigEndian.Uint64(ret)
}

func (lib *Library) BranchInflationBonusBase() uint64 {
	res, err := lib.EvalFromSource(nil, "constBranchInflationBonusBase")
	util.AssertNoError(err)
	return easyfl_util.MustUint64FromBytes(res)

}

// wrong. Was replaced with hash scaling
//func (lib *Library) BranchInflationBonusDirect(proof []byte) uint64 {
//	h := blake2b.Sum256(proof)
//	num := binary.BigEndian.Uint64(h[:8])
//	denom := lib.BranchInflationBonusBase() + 1
//	ret := num % denom
//	return ret
//}

func (lib *Library) BranchInflationBonusDirect(proof []byte) uint64 {
	return base.RandomFromSeed(proof, lib.BranchInflationBonusBase()) + 1
}

// BranchInflationBonusFromRandomnessProof makes uint64 in the range from 0 to BranchInflationBonusBase (incl)
func (lib *Library) BranchInflationBonusFromRandomnessProof(proof []byte) uint64 {
	src := fmt.Sprintf("branchInflationBonusFromRandomnessProof(0x%s)", hex.EncodeToString(proof))
	res, err := lib.EvalFromSource(nil, src)
	util.AssertNoError(err)
	return binary.BigEndian.Uint64(res)
}

const _inflationFunctionsSource = `

// aux value
// $0 predecessor timestamp bytes
// $1 successor timestamp bytes
func _adjustedDiffSlots :
	add(
       sub(first4Bytes($1), first4Bytes($0)),
       if (isTimestampBytesOnSlotBoundary($0), u64/1, u64/0)
    )

// $0 - ledger time (timestamp bytes) of the predecessor
// $1 - amount on predecessor
func _baseInflation : div($1, add(div(constInitialSupply,constSlotInflationBase), first4Bytes($0)))

// $0 - ledger time (timestamp) of the predecessor
// $1 - adjusted diff slots
// $2 - amount on predecessor
func _calcChainInflationAmount : 
    if(
       lessThan(constLinearInflationSlots, $1),
       mul(constLinearInflationSlots, _baseInflation($0, $2)),
       mul($1, _baseInflation($0, $2))
    )

// $0 - ledger time (timestamp) of the predecessor
// $1 - ledger time (timestamp) of the successor
// $2 - amount on predecessor
func calcChainInflationAmount : 
    if(
        not(lessThan($0, $1)),
        !!!calcChainInflationAmount_failed_wrong_timestamps,
   	    if(
           isTimestampBytesOnSlotBoundary($1),
           u64/0,
           _calcChainInflationAmount($0, _adjustedDiffSlots($0, $1), $2)
        )
    )

// $0 - VRF proof
// returns 8 bytes of big-endian uint64 value in the range from 1 to constBranchInflationBonusBase (inclusive)
// taken from the VRF proof
func branchInflationBonusFromRandomnessProof :
    add(randomFromSeed($0, constBranchInflationBonusBase), 1)
`

// This is wrong as it introduces statistical bias towards small values
// Instead of modulus operation, BigInt scaling of the blake2b-256 hash should be used

//func branchInflationBonusFromRandomnessProof :
//mod(
//   slice(blake2b($0),0,7),
//   add(constBranchInflationBonusBase, u64/1)
//)
