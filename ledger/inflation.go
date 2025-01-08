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

func NewInflationConstraint(amount uint64, chainConstraintIndex byte) *InflationConstraint {
	idata := make([]byte, 8)
	binary.BigEndian.PutUint64(idata, amount)
	return &InflationConstraint{
		InflationData:        idata,
		ChainConstraintIndex: chainConstraintIndex,
	}
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
	lib.extendWithConstraint(InflationConstraintName, inflationConstraintSource, 2, func(data []byte) (Constraint, error) {
		return InflationConstraintFromBytes(data)
	})
}

const inflationConstraintSource = `
func _producedVRFProof : 
     evalArgumentBytecode(
        producedConstraintByIndex(concat(txStemOutputIndex, lockConstraintIndex)), 
        #stemLock, 
        1
     )

// inflation(<inflation amount>, <chain constraint index>)
// $0 - inflation amount (8 bytes or isZero).  
// $1 - chain constraint index (sibling)
//
func inflation : or(
	selfIsConsumedOutput, // not checked if consumed
	isZero($0),           // zero inflation always ok
	and(
  		selfIsProducedOutput,
        if(
           isBranchTransaction,
                   // branch tx
           require(
                equal( $0, branchInflationBonusFromRandomnessProof(_producedVRFProof) ),
                !!!wrong_branch_inflation_bonus
           ),
                   // not branch tx
           require(
	    		lessOrEqualThan(
                    $0,
		    		calcChainInflationAmount(
			    		timestampOfInputByIndex(chainPredecessorInputIndex($1)), 
                        txTimestampBytes,
					    amountValue(consumedOutputByIndex(chainPredecessorInputIndex($1))),
				   )
			    ),
			    !!!invalid_chain_inflation_amount
		   )
        ),
		require(
			equalUint(
				calcChainInflationAmount(
					timestampOfInputByIndex(chainPredecessorInputIndex($1)), 
                    txTimestampBytes,
					amountValue(consumedOutputByIndex(chainPredecessorInputIndex($1))),
				),				
				$0
			),
			!!!invalid_chain_inflation_amount
		)
    )
)
`
