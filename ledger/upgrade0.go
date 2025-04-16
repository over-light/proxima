package ledger

import (
	"fmt"

	"github.com/lunfardo314/easyfl"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/lazybytes"
	"github.com/lunfardo314/unitrie/common"
)

// This file contains all upgrade prescriptions to the base ledger provided by the EasyFL. It is the "version 0" of the ledger.
// Ledger definition can be upgraded by adding new embedded and extended function with new binary codes.
// That will make ledger upgrades backwards compatible, because all past transactions and EasyFL constraint bytecodes
// outputs will be interpreted exactly the same way

/*
The following defines Proxima transaction model, library of constraints and other functions
in addition to the base library provided by EasyFL

All integers are treated big-endian. This way lexicographical order coincides with the arithmetic order.

The validation context is a tree-like data structure which is validated by evaluating all constraints in it
consumed and produced outputs. The rest of the validation should be done by the logic outside the data itself.
The tree-like data structure is a lazybytes.Array, treated as a tree.

Constants which define validation context data tree branches. Structure of the data tree:

(root)
  -- TransactionBranch = 0x00
       -- TxUnlockData = 0x00 (path 0x0000)  -- contains unlock params for each input
       -- TxInputIDs = 0x01     (path 0x0001)  -- contains up to 256 inputs, the IDs of consumed outputs
       -- TxOutputBranch = 0x02       (path 0x0002)  -- contains up to 256 produced outputs
       -- TxSignature = 0x03          (path 0x0003)  -- contains the only signature of the essence. It is mandatory
       -- TxTimestamp = 0x04          (path 0x0004)  -- mandatory timestamp of the transaction
       -- TxInputCommitment = 0x05    (path 0x0005)  -- blake2b hash of the all consumed outputs (which are under path 0x1000)
       -- TxEndorsements = 0x06       (path 0x0006)  -- list of transaction IDs of endorsed transaction
       -- TxLocalLibraries = 0x07     (path 0x0007)  -- list of local libraries in its binary form
  -- ConsumedBranch = 0x01
       -- ConsumedOutputsBranch = 0x00 (path 0x0100) -- all consumed outputs, up to 256

All consumed outputs ar contained in the tree element under path 0x0100
A input id is at path 0x0001ii, where (ii) is 1-byte index of the consumed input in the transaction
This way:
	- the corresponding consumed output is located at path 0x0100ii (replacing 2 byte path prefix with 0x0100)
	- the corresponding unlock-parameters is located at path 0x0000ii (replacing 2 byte path prefix with 0x0000)
*/

// Top level branches
const (
	TransactionBranch = byte(iota)
	ConsumedBranch
)

// Transaction tree
const (
	TxInputIDs = byte(iota)
	TxUnlockData
	TxOutputs
	TxSignature
	TxSequencerAndStemOutputIndices
	TxTimestamp
	TxTotalProducedAmount
	TxInputCommitment
	TxEndorsements
	TxExplicitBaseline
	TxLocalLibraries
	TxTreeIndexMax
)

const ConsumedOutputsBranch = byte(0)

var (
	PathToConsumedOutputs               = lazybytes.Path(ConsumedBranch, ConsumedOutputsBranch)
	PathToProducedOutputs               = lazybytes.Path(TransactionBranch, TxOutputs)
	PathToUnlockParams                  = lazybytes.Path(TransactionBranch, TxUnlockData)
	PathToInputIDs                      = lazybytes.Path(TransactionBranch, TxInputIDs)
	PathToSignature                     = lazybytes.Path(TransactionBranch, TxSignature)
	PathToSequencerAndStemOutputIndices = lazybytes.Path(TransactionBranch, TxSequencerAndStemOutputIndices)
	PathToInputCommitment               = lazybytes.Path(TransactionBranch, TxInputCommitment)
	PathToEndorsements                  = lazybytes.Path(TransactionBranch, TxEndorsements)
	PathToExplicitBaseline              = lazybytes.Path(TransactionBranch, TxExplicitBaseline)
	PathToLocalLibraries                = lazybytes.Path(TransactionBranch, TxLocalLibraries)
	PathToTimestamp                     = lazybytes.Path(TransactionBranch, TxTimestamp)
	PathToTotalProducedAmount           = lazybytes.Path(TransactionBranch, TxTotalProducedAmount)
)

// Mandatory output block indices
const (
	ConstraintIndexAmount = byte(iota)
	ConstraintIndexLock
	ConstraintIndexFirstOptionalConstraint
)

func (lib *Library) upgrade0(id *IdentityData) {
	lib.ID = id

	err := lib.UpgradeFromYAML([]byte(_upgradeEmbeddedYAML), _embeddedFunctions(lib))
	util.AssertNoError(err)

	// add main ledger constants
	err = lib.UpgradeFromYAML(_upgradeLedgerConstantsYAML(id.GenesisControllerPublicKey, uint64(id.GenesisTimeUnix)))
	util.AssertNoError(err)

	// add base helpers
	err = lib.UpgradeFromYAML([]byte(_upgradeBaseHelpers))
	util.AssertNoError(err)

	lib.appendInlineTests(func() {
		// inline tests
		libraryGlobal.MustEqual("timestampBytes(u32/255, 21)", NewLedgerTime(255, 21).Hex())
		libraryGlobal.MustEqual("ticksBefore(timestampBytes(u32/100, 5), timestampBytes(u32/101, 10))", "u64/133")
		libraryGlobal.MustError("mustValidTimeSlot(255)", "wrong slot data")
		libraryGlobal.MustEqual("mustValidTimeSlot(u32/255)", Slot(255).Hex())
		libraryGlobal.MustEqual("mustValidTimeTick(88)", "88")
		libraryGlobal.MustError("mustValidTimeTick(200)", "'wrong ticks value'")
		libraryGlobal.MustEqual("div(constInitialSupply, constSlotInflationBase)", "u64/30303030")
	})

	lib.upgrade0WithExtensions()

}

// DataContext is the data structure passed to the eval call. It contains:
// - tree: all validation context of the transaction, all data which is to be validated
// - path: a path in the validation context of the constraint being validated in the eval call
type DataContext struct {
	tree *lazybytes.Tree
	path lazybytes.TreePath
}

func NewDataContext(tree *lazybytes.Tree) *DataContext {
	return &DataContext{tree: tree}
}

func (c *DataContext) DataTree() *lazybytes.Tree {
	return c.tree
}

func (c *DataContext) Path() lazybytes.TreePath {
	return c.path
}

func (c *DataContext) SetPath(path lazybytes.TreePath) {
	c.path = common.Concat(path.Bytes())
}

func (lib *Library) upgrade0WithExtensions() *Library {
	// TODO
	lib.upgrade0WithGeneralFunctions()
	lib.upgrade0WithConstraints()

	return lib
}

var upgrade0WithFunctions = []*easyfl.ExtendedFunctionData{
	{"pathToTransaction", fmt.Sprintf("%d", TransactionBranch), ""},
	{"pathToConsumedOutputs", fmt.Sprintf("0x%s", PathToConsumedOutputs.Hex()), ""},
	{"pathToProducedOutputs", fmt.Sprintf("0x%s", PathToProducedOutputs.Hex()), ""},
	{"pathToUnlockParams", fmt.Sprintf("0x%s", PathToUnlockParams.Hex()), ""},
	{"pathToInputIDs", fmt.Sprintf("0x%s", PathToInputIDs.Hex()), ""},
	{"pathToSignature", fmt.Sprintf("0x%s", PathToSignature.Hex()), ""},
	{"pathToSeqAndStemOutputIndices", fmt.Sprintf("0x%s", PathToSequencerAndStemOutputIndices.Hex()), ""},
	{"pathToInputCommitment", fmt.Sprintf("0x%s", PathToInputCommitment.Hex()), ""},
	{"pathToEndorsements", fmt.Sprintf("0x%s", PathToEndorsements.Hex()), ""},
	{"pathToExplicitBaseline", fmt.Sprintf("0x%s", PathToExplicitBaseline.Hex()), ""},
	{"pathToLocalLibrary", fmt.Sprintf("0x%s", PathToLocalLibraries.Hex()), ""},
	{"pathToTimestamp", fmt.Sprintf("0x%s", PathToTimestamp.Hex()), ""},
	{"pathToTotalProducedAmount", fmt.Sprintf("0x%s", PathToTotalProducedAmount.Hex()), ""},
	// mandatory block indices in the output
	{"amountConstraintIndex", fmt.Sprintf("%d", ConstraintIndexAmount), ""},
	{"lockConstraintIndex", fmt.Sprintf("%d", ConstraintIndexLock), ""},
	// mandatory constraints and values
	// $0 is output binary as lazy array
	{"amountConstraint", "@Array8($0, amountConstraintIndex)", ""},
	{"lockConstraint", "@Array8($0, lockConstraintIndex)", ""},
	// recognize what kind of path is at $0
	{"isPathToConsumedOutput", "hasPrefix($0, pathToConsumedOutputs)", ""},
	{"isPathToProducedOutput", "hasPrefix($0, pathToProducedOutputs)", ""},
	// make branch path by index $0
	{"consumedOutputPathByIndex", "concat(pathToConsumedOutputs,$0)", ""},
	{"unlockParamsPathByIndex", "concat(pathToUnlockParams,$0)", ""},
	{"producedOutputPathByIndex", "concat(pathToProducedOutputs,$0)", ""},
	// takes 1-byte $0 as output index
	{"consumedOutputByIndex", "@Path(consumedOutputPathByIndex($0))", ""},
	{"unlockParamsByIndex", "@Path(unlockParamsPathByIndex($0))", ""},
	{"producedOutputByIndex", "@Path(producedOutputPathByIndex($0))", ""},
	// takes $0 'constraint index' as 2 bytes: 0 for output index, 1 for block index
	{"producedConstraintByIndex", "@Array8(producedOutputByIndex(byte($0,0)), byte($0,1))", ""},
	{"consumedConstraintByIndex", "@Array8(consumedOutputByIndex(byte($0,0)), byte($0,1))", ""},
	{"unlockParamsByConstraintIndex", "@Array8(unlockParamsByIndex(byte($0,0)), byte($0,1))", ""}, // 2-byte index (outIdx, constrIdx)

	{"consumedLockByInputIndex", "consumedConstraintByIndex(concat($0, lockConstraintIndex))", ""},
	{"inputIDByIndex", "@Path(concat(pathToInputIDs,$0))", ""},
	{"timestampOfInputByIndex", "timestampBytesFromPrefix(inputIDByIndex($0))", ""},
	{"timeSlotOfInputByIndex", "first4Bytes(inputIDByIndex($0))", ""},
	// special transaction related
	{"txBytes", "@Path(pathToTransaction)", ""},
	{"txSignature", "@Path(pathToSignature)", ""},
	{"txTimestampBytes", "@Path(pathToTimestamp)", ""},
	{"txExplicitBaseline", "@Path(pathToExplicitBaseline)", ""},
	{"txTotalProducedAmount", "uint8Bytes(@Path(pathToTotalProducedAmount))", ""},
	{"txTimeSlot", "first4Bytes(txTimestampBytes)", ""},
	{"txTimeTick", "timeTickFromTimestampBytes(txTimestampBytes)", ""},
	{"txSequencerOutputIndex", "byte(@Path(pathToSeqAndStemOutputIndices), 0)", ""},
	{"txStemOutputIndex", "byte(@Path(pathToSeqAndStemOutputIndices), 1)", ""},
	{"txEssenceBytes", "concat(" +
		"@Path(pathToInputIDs), " +
		"@Path(pathToProducedOutputs), " +
		"@Path(pathToTimestamp), " +
		"@Path(pathToSeqAndStemOutputIndices), " +
		"@Path(pathToInputCommitment), " +
		"@Path(pathToEndorsements))", ""},
	{"sequencerFlagON", "not(isZero(bitwiseAND(byte($0,4),0x01)))", ""},
	{"isSequencerTransaction", "not(equal(txSequencerOutputIndex, 0xff))", ""},
	{"isBranchTransaction", "and(isSequencerTransaction, not(equal(txStemOutputIndex, 0xff)))", ""},
	// endorsements
	{"numEndorsements", "ArrayLength8(@Path(pathToEndorsements))", ""},
	{"numInputs", "ArrayLength8(@Path(pathToInputIDs))", ""},
	// functions with prefix 'self' are invocation context specific, i.e. they use function '@' to calculate
	// local values which depend on the invoked constraint
	{"selfOutputPath", "slice(@,0,2)", ""},
	{"selfSiblingConstraint", "@Array8(@Path(selfOutputPath), $0)", ""},
	{"selfOutputBytes", "@Path(selfOutputPath)", ""},
	{"selfNumConstraints", "ArrayLength8(selfOutputBytes)", ""},
	// unlock param branch (0 - transaction, 0 unlock params)
	// invoked output block
	{"self", "@Path(@)", ""},
	// bytecode prefix of the invoked constraint. It is needed to avoid forward references in the EasyFL code
	{"selfBytecodePrefix", "parsePrefixBytecode(self)", ""},
	{"selfIsConsumedOutput", "isPathToConsumedOutput(@)", ""},
	{"selfIsProducedOutput", "isPathToProducedOutput(@)", ""},
	// output index of the invocation
	{"selfOutputIndex", "byte(@, 2)", ""},
	// block index of the invocation
	{"selfBlockIndex", "tail(@, 3)", ""},
	// branch (2 bytes) of the constraint invocation
	{"selfBranch", "slice(@,0,1)", ""},
	// output index || block index
	{"selfConstraintIndex", "slice(@, 2, 3)", ""},
	// data of a constraint
	{"constraintData", "tail($0,1)", ""},
	// invocation output data
	{"selfConstraintData", "constraintData(self)", ""},
	// unlock parameters of the invoked consumed constraint
	{"selfUnlockParameters", "@Path(concat(pathToUnlockParams, selfConstraintIndex))", ""},
	// path referenced by the reference unlock params
	{"selfReferencedPath", "concat(selfBranch, selfUnlockParameters, selfBlockIndex)", ""},
	// returns unlock block of the sibling
	{"selfSiblingUnlockBlock", "@Array8(@Path(concat(pathToUnlockParams, selfOutputIndex)), $0)", ""},
	// returns selfUnlockParameters if blake2b hash of it is equal to the given hash, otherwise nil
	{"selfHashUnlock", "if(equal($0, blake2b(selfUnlockParameters)),selfUnlockParameters,nil)", ""},
	// takes ED25519 signature from full signature, first 64 bytes
	{"signatureED25519", "slice($0, 0, 63)", ""},
	// takes ED25519 public key from full signature
	{"publicKeyED25519", "slice($0, 64, 95)", ""},
}

func (lib *Library) upgrade0WithGeneralFunctions() {
	lib.UpgradeWithExtensions(upgrade0WithFunctions...)
}

func (lib *Library) upgrade0WithConstraints() {
	addAmountConstraint(lib)
	addAddressED25519Constraint(lib)
	addConditionalLock(lib)
	addDeadlineLockConstraint(lib)
	addTimeLockConstraint(lib)
	addChainConstraint(lib)
	addStemLockConstraint(lib)
	addSequencerConstraint(lib)
	addInflationConstraint(lib)
	addSenderED25519Constraint(lib)
	addChainLockConstraint(lib)
	//addRoyaltiesED25519Constraint(lib)
	addImmutableConstraint(lib)
	addCommitToSiblingConstraint(lib)
	//addTotalAmountConstraint(lib)
	addDelegationLock(lib)
}
