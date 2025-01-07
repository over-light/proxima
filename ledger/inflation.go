package ledger

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/lunfardo314/easyfl"
)

const (
	InflationConstraintName = "inflation"
	// (0) chain constraint index, (1) inflation amount or randomness proof
	inflationConstraintTemplate = InflationConstraintName + "(%d, 0x%s)"
)

type InflationConstraint struct {
	// either it is nil, which means 0 inflation, or 8 bytes of chain inflation or VRF randomness proof (on the branch only)
	InflationData        []byte
	ChainConstraintIndex byte
}

func (i *InflationConstraint) Name() string {
	return InflationConstraintName
}

func (i *InflationConstraint) Bytes() []byte {
	return mustBinFromSource(i.Source())
}

func (i *InflationConstraint) String() string {
	return i.Source()
	//return fmt.Sprintf("%s(%s, 0x%s, %d, %d)",
	//	InflationConstraintName,
	//	util.Th(i.ChainInflation),
	//	hex.EncodeToString(i.VRFProof),
	//	i.ChainConstraintIndex,
	//	i.DelayedInflationIndex,
	//)
}

func (i *InflationConstraint) Source() string {
	return fmt.Sprintf(inflationConstraintTemplate, i.ChainConstraintIndex, hex.EncodeToString(i.InflationData))
}

// InflationAmount calculates inflation amount either inside slot, or on the slot boundary
func (i *InflationConstraint) InflationAmount(slotBoundary bool) uint64 {
	if slotBoundary {
		// the ChainInflation is interpreted as delayed inflation
		return L().BranchInflationBonusFromRandomnessProof(i.InflationData)
	}
	return binary.BigEndian.Uint64(i.InflationData)
}

func InflationConstraintFromBytes(data []byte) (*InflationConstraint, error) {
	sym, _, args, err := L().ParseBytecodeOneLevel(data, 2)
	if err != nil {
		return nil, err
	}
	if sym != InflationConstraintName {
		return nil, fmt.Errorf("InflationConstraintFromBytes: not a inflation constraint script")
	}
	cci := easyfl.StripDataPrefix(args[0])
	if len(cci) != 1 || cci[0] == 0xff {
		return nil, fmt.Errorf("InflationConstraintFromBytes: wrong ChainConstraintIndex parameter")
	}
	return &InflationConstraint{
		ChainConstraintIndex: cci[0],
		InflationData:        easyfl.StripDataPrefix(args[1]),
	}, nil
}

func addInflationConstraint(lib *Library) {
	lib.MustExtendMany(inflationFunctionsSource)
	lib.extendWithConstraint(InflationConstraintName, inflationConstraintSource, 4, func(data []byte) (Constraint, error) {
		return InflationConstraintFromBytes(data)
	})
}

const inflationConstraintSource = `
// $0 - chain constraint index
// $1 - index with the delayed inflation in the predecessor
// 
// returns:
// - chain inflation in the predecessor branch transaction
// - 0 if delayed inflation not specified or predecessor is not branch
//
func delayedInflationValue : 
if(
	equal($1, 0xff),  
	u64/0,  // delayed inflation index on predecessor is not specified ->  not delayed inflation is zero.
	if(
		isBranchOutputID(inputIDByIndex(chainPredecessorInputIndex($0))),
		// previous is branch -> parse first argument from the inflation constraint there 
		evalArgumentBytecode(
			consumedConstraintByIndex(concat(chainPredecessorInputIndex($0), $1)),
			selfBytecodePrefix,
			0
		),
		// previous is not a branch -> nothing is delayed
		u64/0
	)
)

// inflation(<inflation amount>, <VRF proof>, <chain constraint index>, <delayed inflation index>)
// $0 - chain inflation amount (8 bytes or isZero). On slot boundary interpreted as delayed inflation. Inflation either 0 or precise amount 
// $1 - vrf proof. Interpreted only on branch transactions
// $2 - chain constraint index (sibling)
// $3 - delayed inflation index. Inflation constraint index in the predecessor, 0xff means not specified
//
func inflation : or(
	selfIsConsumedOutput, // not checked if consumed
	isZero($0),           // zero inflation always ok
	and(
  		selfIsProducedOutput,
		require(equalUint(len($3), 1), !!!delayed_inflation_index_must_be_1_byte),
		require(
			equalUint(
				calcChainInflationAmount(
					timestampOfInputByIndex(chainPredecessorInputIndex($2)), 
					ticksBefore(
                       timestampOfInputByIndex(chainPredecessorInputIndex($2)),
                       txTimestampBytes
                    ), 
					amountValue(consumedOutputByIndex(chainPredecessorInputIndex($2))),
					delayedInflationValue($2, $3)
				),				
				$0
			),
			!!!invalid_chain_inflation_amount
		),
		require(
			or(
				not(isBranchTransaction),
				vrfVerify(publicKeyED25519(txSignature), $1, predStemOutputIDOfSelf)
			),
			!!!VRF_verification_failed
		)
    )
)
`
