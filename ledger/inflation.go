package ledger

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/lunfardo314/easyfl"
	"github.com/lunfardo314/proxima/util"
)

const (
	InflationConstraintName = "inflation"
	// (0) chain constraint index, (1) inflation amount or randomness proof
	inflationConstraintTemplate = InflationConstraintName + "(%d, u64/%d)"
)

type InflationConstraint struct {
	// either it is nil, which means 0 inflation, or 8 bytes of chain inflation or VRF randomness proof (on the branch only)
	InflationAmount      uint64
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
	return fmt.Sprintf(inflationConstraintTemplate, i.ChainConstraintIndex, i.InflationAmount)
}

func InflationConstraintFromBytes(data []byte) (*InflationConstraint, error) {
	sym, _, args, err := L().ParseBytecodeOneLevel(data, 2)
	if err != nil {
		return nil, err
	}
	if sym != InflationConstraintName {
		return nil, fmt.Errorf("InflationConstraintFromBytes: not an inflation constraint script")
	}
	cci := easyfl.StripDataPrefix(args[0])
	if len(cci) != 1 || cci[0] == 0xff {
		return nil, fmt.Errorf("InflationConstraintFromBytes: wrong ChainConstraintIndex parameter")
	}
	amountBin := easyfl.StripDataPrefix(args[1])
	var amount uint64
	if len(amountBin) != 0 {
		if len(amountBin) != 8 {
			return nil, fmt.Errorf("InflationConstraintFromBytes: wrong ChainConstraintIndex parameter")
		}
		amount = binary.BigEndian.Uint64(amountBin)
	}
	return &InflationConstraint{
		ChainConstraintIndex: cci[0],
		InflationAmount:      amount,
	}, nil
}

func addInflationConstraint(lib *Library) {
	lib.MustExtendMany(inflationFunctionsSource)
	lib.extendWithConstraint(InflationConstraintName, inflationConstraintSource, 2, func(data []byte) (Constraint, error) {
		return InflationConstraintFromBytes(data)
	}, initTestInflationConstraint)
}

func initTestInflationConstraint() {
	ic := InflationConstraint{
		InflationAmount:      13371337,
		ChainConstraintIndex: 5,
	}
	_, _, bytecode, err := L().CompileExpression(ic.Source())
	util.AssertNoError(err)

	util.Assertf(bytes.Equal(ic.Bytes(), bytecode), "bytes.Equal(ic.Bytes(), bytecode)")
}

const inflationConstraintSource = `
func _producedVRFProof : 
     evalArgumentBytecode(
        producedConstraintByIndex(concat(txStemOutputIndex, lockConstraintIndex)), 
        #stemLock, 
        1
     )

// $0 - chain predecessor input index
func _calcChainInflationAmountForPredecessor :
     calcChainInflationAmount(
	    timestampOfInputByIndex($0), 
        txTimestampBytes,
	    amountValue(consumedOutputByIndex($0)),
	 )

// inflation(<inflation amount>, <chain constraint index>)
// $0 - inflation amount (8 bytes or isZero).  
// $1 - chain constraint index (sibling)
//
func inflation : or(
	selfIsConsumedOutput, // not checked if consumed
	and(
  		selfIsProducedOutput,
        if(
           isBranchTransaction,
                   // branch tx. Enforce inflation is calculated from the VRF proof
           require(
                equal( $0, branchInflationBonusFromRandomnessProof(_producedVRFProof) ),
                !!!invalid_branch_inflation_bonus
           ),
                   // not branch tx. Enforce valid chain inflation amount
           require(
	    		lessOrEqualThan(
                    $0,
                    _calcChainInflationAmountForPredecessor(chainPredecessorInputIndex($1))
			    ),
			    !!!invalid_chain_inflation_amount
		   )
        ),
    )
)
`
