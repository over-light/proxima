package ledger

import (
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/lazybytes"
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

	err := lib.UpgradeFromYAML([]byte(_definitionsEmbeddedYAML), EmbeddedFunctions(lib.Library))
	util.AssertNoError(err)

	// add main ledger constants
	err = lib.UpgradeFromYAML(_definitionsLedgerConstantsYAML(id.GenesisControllerPublicKey, uint64(id.GenesisTimeUnix)))
	util.AssertNoError(err)

	// add base helpers
	err = lib.UpgradeFromYAML([]byte(_definitionsBaseHelpersYAML))
	util.AssertNoError(err)

	// add general functions
	err = lib.UpgradeFromYAML([]byte(_definitionsGeneralYAML))
	util.AssertNoError(err)

	lib.upgrade0WithConstraints()

	lib.appendInlineTests(func() {
		// inline tests
		libraryGlobal.MustEqual("timestampBytes(u32/255, 21)", base.NewLedgerTime(255, 21).Hex())
		libraryGlobal.MustEqual("ticksBefore(timestampBytes(u32/100, 5), timestampBytes(u32/101, 10))", "u64/133")
		libraryGlobal.MustError("mustValidTimeSlot(255)", "wrong slot data")
		libraryGlobal.MustEqual("mustValidTimeSlot(u32/255)", base.Slot(255).Hex())
		libraryGlobal.MustEqual("mustValidTimeTick(88)", "88")
		libraryGlobal.MustError("mustValidTimeTick(200)", "'wrong ticks value'")
		libraryGlobal.MustEqual("div(constInitialSupply, constSlotInflationBase)", "u64/30303030")
	})
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
