package ledger

import (
	"fmt"
	"time"

	"github.com/lunfardo314/easyfl"
	"github.com/lunfardo314/easyfl/easyfl_util"
	"github.com/lunfardo314/easyfl/lazybytes"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
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

func LibraryFromIdentityParameters(idParams *IdentityParameters, verbose ...bool) *Library {
	ret := newBaseLibrary(idParams)
	if len(verbose) > 0 && verbose[0] {
		fmt.Printf("------ Base EasyFL library:\n")
		ret.PrintLibraryStats()
	}

	upgrade0(ret.Library, idParams)

	if len(verbose) > 0 && verbose[0] {
		fmt.Printf("------ Extended EasyFL library:\n")
		ret.PrintLibraryStats()
	}
	ret.ID = idParams
	return ret
}

func LibraryYAMLFromIdentityParameters(id *IdentityParameters, compiled bool) []byte {
	return LibraryFromIdentityParameters(id).ToYAML(compiled, "# Proxima ledger definitions")
}

func ParseLedgerIdYAML(yamlData []byte, getResolver ...func(lib *easyfl.Library) func(sym string) easyfl.EmbeddedFunction) (*easyfl.Library, *IdentityParameters, error) {
	lib, err := easyfl.NewLibraryFromYAML(yamlData, getResolver...)
	if err != nil {
		return nil, nil, err
	}
	idParams, err := idParametersFromLibrary(lib)
	if err != nil {
		return nil, nil, err
	}
	return lib, idParams, nil
}

func _uint64FromConst(lib *easyfl.Library, constName string) (uint64, error) {
	res, err := lib.EvalFromSource(nil, constName)
	if err != nil {
		return 0, err
	}
	return easyfl_util.Uint64FromBytes(res)
}

func idParametersFromLibrary(lib *easyfl.Library) (*IdentityParameters, error) {
	ret := &IdentityParameters{}
	var err error
	var res []byte
	if ret.InitialSupply, err = _uint64FromConst(lib, "constInitialSupply"); err != nil {
		return nil, err
	}
	if res, err = lib.EvalFromSource(nil, "constGenesisControllerPublicKey"); err != nil {
		return nil, err
	}
	ret.GenesisControllerPublicKey = res
	if gt, err := _uint64FromConst(lib, "constGenesisTimeUnix"); err != nil {
		return nil, err
	} else {
		ret.GenesisTimeUnix = uint32(gt)
	}
	if td, err := _uint64FromConst(lib, "constTickDuration"); err != nil {
		return nil, err
	} else {
		ret.TickDuration = time.Duration(td)
	}
	if ret.SlotInflationBase, err = _uint64FromConst(lib, "constSlotInflationBase"); err != nil {
		return nil, err
	}
	if ret.LinearInflationSlots, err = _uint64FromConst(lib, "constLinearInflationSlots"); err != nil {
		return nil, err
	}
	if ret.BranchInflationBonusBase, err = _uint64FromConst(lib, "constBranchInflationBonusBase"); err != nil {
		return nil, err
	}
	if ret.MinimumAmountOnSequencer, err = _uint64FromConst(lib, "constMinimumAmountOnSequencer"); err != nil {
		return nil, err
	}
	if ret.MaxNumberOfEndorsements, err = _uint64FromConst(lib, "constMaxNumberOfEndorsements"); err != nil {
		return nil, err
	}
	if pb, err := _uint64FromConst(lib, "constPreBranchConsolidationTicks"); err != nil {
		return nil, err
	} else {
		if pb > 255 {
			return nil, fmt.Errorf("invalid pre branch consolidation ticks")
		}
		ret.PreBranchConsolidationTicks = byte(pb)
	}
	if pb, err := _uint64FromConst(lib, "constPostBranchConsolidationTicks"); err != nil {
		return nil, err
	} else {
		if pb > 255 {
			return nil, fmt.Errorf("invalid post branch consolidation ticks")
		}
		ret.PostBranchConsolidationTicks = byte(pb)
	}
	if tp, err := _uint64FromConst(lib, "constTransactionPace"); err != nil {
		return nil, err
	} else {
		if tp > 255 {
			return nil, fmt.Errorf("invalid transaction pace")
		}
		ret.TransactionPace = byte(tp)
	}
	if tp, err := _uint64FromConst(lib, "constTransactionPaceSequencer"); err != nil {
		return nil, err
	} else {
		if tp > 255 {
			return nil, fmt.Errorf("invalid sequencer transaction pace")
		}
		ret.TransactionPaceSequencer = byte(tp)
	}
	if ret.VBCost, err = _uint64FromConst(lib, "constVBCost16"); err != nil {
		return nil, err
	}
	if res, err = lib.EvalFromSource(nil, "constDescription"); err != nil {
		return nil, err
	}
	ret.Description = string(res)
	return ret, nil
}

func upgrade0(lib *easyfl.Library, id *IdentityParameters) {
	err := base.EmbedHardcoded(lib)
	util.AssertNoError(err)

	// add main ledger constants
	err = lib.UpgradeFromYAML(ConstantsYAMLFromIdentity(id))
	util.AssertNoError(err)

	// add base helpers
	err = lib.UpgradeFromYAML([]byte(_definitionsHelpersYAML))
	util.AssertNoError(err)

	// add general functions
	err = lib.UpgradeFromYAML([]byte(_definitionsGeneralYAML))
	util.AssertNoError(err)

	lib.MustExtendMany(inflationFunctionsSource)

	lib.MustExtendMany(amountSource)
	lib.MustExtendMany(addressED25519ConstraintSource)
	lib.MustExtendMany(conditionalLockSource)
	lib.MustExtendMany(deadlineLockSource)
	lib.MustExtendMany(timelockSource)
	lib.MustExtendMany(chainConstraintSource)
	lib.MustExtendMany(stemLockSource)
	lib.MustExtendMany(sequencerConstraintSource)
	lib.MustExtendMany(inflationConstraintSource)
	lib.MustExtendMany(senderED25519Source)
	lib.MustExtendMany(chainLockConstraintSource)
	lib.MustExtendMany(immutableDataConstraintSource)
	lib.MustExtendMany(commitToSiblingSource)
	lib.MustExtendMany(delegationLockSource)
}

// registerConstraints mass-registers all wrappers of constraints
func (lib *Library) registerConstraints() {
	registerAmountConstraint(lib)
	registerAddressED25519Constraint(lib)
	registerConditionalLock(lib)
	registerDeadlineLockConstraint(lib)
	registerTimeLockConstraint(lib)
	registerChainConstraint(lib)
	registerStemLockConstraint(lib)
	registerSequencerConstraint(lib)
	registerInflationConstraint(lib)
	registerSenderED25519Constraint(lib)
	registerChainLockConstraint(lib)
	registerImmutableConstraint(lib)
	registerCommitToSiblingConstraint(lib)
	registerDelegationLock(lib)

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
