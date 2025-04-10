package ledger

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/lunfardo314/easyfl"
	"github.com/lunfardo314/proxima/util"
)

const amountSource = `
// $0 - amount up to 8 bytes big-endian. Will be expanded to 8 bytes by padding
func amount: 
if(
   and(equal(selfBlockIndex,0), lessOrEqualThan(len($0), u64/8)),
   uint8Bytes($0),
   !!!amount_constraint_must_be_at_index_0
)

// utility function which extracts amount value from the output by evaluating it
// $0 - output bytes
func amountValue : evalArgumentBytecode(@Array8($0, amountConstraintIndex), #amount,0)

func selfAmountValue: amountValue(selfOutputBytes)

// utility function
func selfMustAmountAtLeast : if(
	lessThan(selfAmountValue, $0),
	!!!amount_on_output_is_smaller_than_allowed_minimum,
	true
)

// $0 number of output bytes
func storageDeposit : mul(constVBCost16,$0)

func selfMustStandardAmount: selfMustAmountAtLeast(
    storageDeposit(len(selfOutputBytes))
)

`

const (
	AmountConstraintName = "amount"
	amountTemplate       = AmountConstraintName + "(0x%s)"
)

type Amount uint64

func (a Amount) Name() string {
	return AmountConstraintName
}

func (a Amount) Source() string {
	var bin [8]byte
	binary.BigEndian.PutUint64(bin[:], uint64(a))
	return fmt.Sprintf(amountTemplate, hex.EncodeToString(TrimPrefixZeroBytes(bin[:])))
}

func (a Amount) Bytes() []byte {
	return mustBinFromSource(a.Source())
}

func (a Amount) String() string {
	return a.Source()
	//return fmt.Sprintf("%s(%s)", AmountConstraintName, util.Th(int(a)))
}

func NewAmount(a uint64) Amount {
	return Amount(a)
}

func addAmountConstraint(lib *Library) {
	lib.extendWithConstraint(AmountConstraintName, amountSource, 1, func(data []byte) (Constraint, error) {
		return AmountFromBytes(data)
	}, initTestAmountConstraint)
}

func initTestAmountConstraint() {
	example := NewAmount(1337)
	sym, _, args, err := L().ParseBytecodeOneLevel(example.Bytes(), 1)
	util.AssertNoError(err)
	amountBin := easyfl.StripDataPrefix(args[0])
	util.Assertf(sym == AmountConstraintName && len(amountBin) <= 8, "'amount' consistency check 1 failed")
	value, err := Uint64FromBytes(amountBin)
	util.AssertNoError(err)
	util.Assertf(value == 1337, "amount' consistency check 2 failed")
}

func AmountFromBytes(data []byte) (Amount, error) {
	sym, _, args, err := L().ParseBytecodeOneLevel(data)
	if err != nil {
		return 0, err
	}
	if sym != AmountConstraintName {
		return 0, fmt.Errorf("not an 'amount' constraint")
	}
	amountBin := easyfl.StripDataPrefix(args[0])
	ret, err := Uint64FromBytes(amountBin)
	if err != nil {
		return 0, err
	}
	return Amount(ret), nil
}

func (a Amount) Amount() uint64 {
	return uint64(a)
}
