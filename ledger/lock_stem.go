package ledger

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/lunfardo314/easyfl"
	"github.com/lunfardo314/proxima/util"
)

const (
	StemLockName = "stemLock"
	stemTemplate = StemLockName + "(0x%s,0x%s,%d)"
)

type (
	StemLock struct {
		PredecessorOutputID  OutputID
		VRFProof             []byte
		StemPredecessorIndex byte
	}
)

var StemAccountID = AccountID([]byte{0})

func (st *StemLock) AccountID() AccountID {
	return StemAccountID
}

func (st *StemLock) AsLock() Lock {
	return st
}

func (st *StemLock) Name() string {
	return StemLockName
}

func (st *StemLock) Source() string {
	return fmt.Sprintf(stemTemplate,
		hex.EncodeToString(st.PredecessorOutputID[:]),
		hex.EncodeToString(st.VRFProof),
		st.StemPredecessorIndex,
	)
}

func (st *StemLock) Bytes() []byte {
	return mustBinFromSource(st.Source())
}

func (st *StemLock) String() string {
	return st.Source()
	//return fmt.Sprintf("stem(%s)", st.PredecessorOutputID.StringShort())
}

func (st *StemLock) Accounts() []Accountable {
	return []Accountable{st}
}

func (st *StemLock) Master() Accountable {
	return nil
}

func addStemLockConstraint(lib *Library) {
	lib.extendWithConstraint(StemLockName, stemLockSource, 1, func(data []byte) (Constraint, error) {
		return StemLockFromBytes(data)
	}, initTestStemLockConstraint)
}

func initTestStemLockConstraint() {
	txid := RandomTransactionID(true)
	predID := MustNewOutputID(&txid, byte(txid.NumProducedOutputs()-1))
	example := StemLock{
		PredecessorOutputID:  predID,
		VRFProof:             []byte{0x01, 0x02, 0x03},
		StemPredecessorIndex: 2,
	}
	exampleBack, err := StemLockFromBytes(example.Bytes())
	util.AssertNoError(err)
	util.Assertf(bytes.Equal(example.Bytes(), exampleBack.Bytes()), "bytes.Equal(example.Bytes(), exampleBack.Bytes())")
	_, err = L().ParsePrefixBytecode(example.Bytes())
	util.AssertNoError(err)
}

func StemLockFromBytes(data []byte) (*StemLock, error) {
	sym, _, args, err := L().ParseBytecodeOneLevel(data, 3)
	if err != nil {
		return nil, err
	}
	if sym != StemLockName {
		return nil, fmt.Errorf("not a 'stem' constraint")
	}
	predIDBin := easyfl.StripDataPrefix(args[0])

	oid, err := OutputIDFromBytes(predIDBin)
	if err != nil {
		return nil, err
	}

	if len(args[2]) != 1 {
		return nil, fmt.Errorf("wrong stem predecessor index")
	}
	return &StemLock{
		PredecessorOutputID:  oid,
		VRFProof:             easyfl.StripDataPrefix(args[1]),
		StemPredecessorIndex: args[2][0],
	}, nil
}

const stemLockSource = `
func producedStemLockOfSelfTx : lockConstraint(producedOutputByIndex(txStemOutputIndex))

func _predOutputID : evalArgumentBytecode(producedStemLockOfSelfTx, selfBytecodePrefix, 0)

// $0 - stem predecessor index
func _predVRFProof : evalArgumentBytecode(
    consumedConstraintByIndex(concat($0,1)), 
    selfBytecodePrefix, 
    1
)

// $0 - predecessor output ID
// $1 - VRF proof (signed data is concatenation of VRF proof from stem predecessor and slot of the transaction)
// $2 - stem predecessor input index
// does not require unlock parameters
func stemLock: and(
	require(isBranchTransaction, !!!must_be_a_branch_transaction),
    require(equal(selfNumConstraints, 2), !!!stem_output_must_contain_exactly_2_constraints),
	require(equal(selfBlockIndex,1), !!!locks_must_be_at_block_1), 
	require(isZero(selfAmountValue), !!!amount_must_be_zero),
	mustSize($0, 33),
	mustSize($1, 1),
    or(
       and(
          selfIsConsumedOutput,
             // enforce correct predecessor output
          require(equal(inputIDByIndex(selfOutputIndex), _predOutputID), !!!wrong_predecessor_output_ID),
       ),
       and(
          selfIsProducedOutput,
            // must be consistent with the transaction level data
          equal(selfOutputIndex, txStemOutputIndex) 
             // enforce correct VRF proof
		  require(
             vrfVerify(publicKeyED25519(txSignature), $1, concat(_predVRFProof($2), txTimeSlot)), 
             !!!VRF_proof_check_failed
         )
       )
    )
)

// utility function to get stem predecessor. Does not use 'selfBytecodePrefix''
func predStemOutputIDOfSelf : evalArgumentBytecode(producedStemLockOfSelfTx, #stemLock, 0)
`
