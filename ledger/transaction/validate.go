package transaction

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"

	"sync/atomic"

	"github.com/lunfardo314/easyfl"
	"github.com/lunfardo314/easyfl/easyfl_util"
	"github.com/lunfardo314/easyfl/lazybytes"
	"github.com/lunfardo314/easyfl/slicepool"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/unitrie/common"
	"golang.org/x/crypto/blake2b"
)

func (ctx *TxContext) evalContext(path []byte) easyfl.GlobalData {
	ctx.dataContext.SetPath(path)
	switch ctx.traceOption {
	case TraceOptionNone:
		return easyfl.NewGlobalDataNoTrace(ctx.dataContext)
	case TraceOptionAll:
		return easyfl.NewGlobalDataTracePrint(ctx.dataContext)
	case TraceOptionFailedConstraints:
		return easyfl.NewGlobalDataLog(ctx.dataContext)
	default:
		panic("wrong trace option")
	}
}

// checkConstraint checks the constraint at path. In-line and unlock scripts are ignored
// for 'produces output' context
func (ctx *TxContext) checkConstraint(constraintData []byte, constraintPath lazybytes.TreePath, spool *slicepool.SlicePool) ([]byte, string, error) {
	var ret []byte
	var name string
	err := util.CatchPanicOrError(func() error {
		var err1 error
		ret, name, err1 = ctx.evalConstraint(constraintData, constraintPath, spool)
		return err1
	})
	if err != nil {
		return nil, name, err
	}
	return ret, name, nil
}

func (ctx *TxContext) Validate() error {
	if err := ctx._validate(); err != nil {
		return fmt.Errorf("%w\ntxid = %s (%s)", err, ctx.txid.StringShort(), ctx.txid.StringHex())
	}
	return nil
}

// _validate runs scripts on consumed and produced parts. Does not check the consistency of input commitment, because
// it already checked upon creation of the transaction context
func (ctx *TxContext) _validate() error {
	var inSum, outSum uint64
	var err error

	spool := slicepool.New()
	defer spool.Dispose()

	err = util.CatchPanicOrError(func() error {
		var err1 error
		if inSum, err1 = ctx.validateOutputsFailFast(true, spool); err1 != nil {
			return err1
		}
		if outSum, err1 = ctx.validateOutputsFailFast(false, spool); err1 != nil {
			return err1
		}
		return nil
	})
	if err != nil {
		return err
	}
	if inSum+ctx.inflationAmount != outSum {
		return fmt.Errorf("unbalanced amount between inputs and outputs: inputs %s, outputs %s, inflation: %s",
			util.Th(inSum), util.Th(outSum), util.Th(ctx.inflationAmount))
	}
	return nil
}

func (ctx *TxContext) writeStateMutationsTo(mut common.KVWriter) {
	// delete consumed outputs from the ledger
	ctx.ForEachInputID(func(idx byte, oid *base.OutputID) bool {
		mut.Set(oid[:], nil)
		return true
	})
	// add produced outputs to the ledger

	ctx.ForEachProducedOutputData(func(i byte, outputData []byte) bool {
		oid := base.MustNewOutputID(ctx.txid, i)
		mut.Set(oid[:], outputData)
		return true
	})
}

// ValidateWithReportOnConsumedOutputs validates the transaction and returns indices of failing consumed outputs, if any
// This for the convenience of automated VMs and sequencers
//func (ctx *TxContext) ValidateWithReportOnConsumedOutputs() ([]byte, error) {
//	var inSum, outSum uint64
//	var err error
//	var retFailedConsumed []byte
//
//	spool := slicepool.New()
//	defer spool.Dispose()
//
//	err = util.CatchPanicOrError(func() error {
//		var err1 error
//		inSum, retFailedConsumed, err1 = ctx._validateOutputs(true, false, spool)
//		return err1
//	})
//	if err != nil {
//		// return list of failed consumed outputs
//		return retFailedConsumed, err
//	}
//	err = util.CatchPanicOrError(func() error {
//		var err1 error
//		outSum, _, err1 = ctx._validateOutputs(false, true, spool)
//		return err1
//	})
//	if err != nil {
//		return nil, err
//	}
//	err = util.CatchPanicOrError(func() error {
//		return ctx.validateInputCommitmentSafe()
//	})
//	if err != nil {
//		return nil, err
//	}
//
//	if inSum+ctx.inflationAmount != outSum {
//		return nil, fmt.Errorf("unbalanced amount between inputs and outputs: inputs %s + inflation: %s != outputs %s",
//			util.Th(inSum), util.Th(ctx.inflationAmount), util.Th(outSum))
//	}
//	return nil, nil
//}

func (ctx *TxContext) validateOutputsFailFast(consumedBranch bool, spool *slicepool.SlicePool) (uint64, error) {
	totalAmount, _, err := ctx._validateOutputs(consumedBranch, true, spool)
	return totalAmount, err
}

// _validateOutputs validates consumed or produced outputs and, optionally, either fails fast,
// or return the list of indices of failed outputs
// If err != nil and failFast = false, returns list of failed consumed and produced output respectively
// if failFast = true, returns (totalAmount, nil, nil, error)
func (ctx *TxContext) _validateOutputs(consumedBranch bool, failFast bool, spool *slicepool.SlicePool) (uint64, []byte, error) {
	var branch lazybytes.TreePath
	if consumedBranch {
		branch = Path(ledger.ConsumedBranch, ledger.ConsumedOutputsBranch)
	} else {
		branch = Path(ledger.TransactionBranch, ledger.TxOutputs)
	}
	var lastErr error
	var sum uint64
	var extraDepositWeight uint32
	var failedOutputs bytes.Buffer

	path := common.Concat(branch, 0)
	ctx.tree.ForEach(func(i byte, data []byte) bool {
		var err error
		path[len(path)-1] = i
		o, err := ledger.OutputFromBytesReadOnly(data)
		if err != nil {
			if !failFast {
				failedOutputs.WriteByte(i)
			}
			lastErr = err
			return !failFast
		}

		if extraDepositWeight, err = ctx.runOutput(consumedBranch, o, path, spool); err != nil {
			if !failFast {
				failedOutputs.WriteByte(i)
			}
			lastErr = fmt.Errorf("%w :\n%s", err, o.ToString("   "))
			return !failFast
		}
		minDeposit := o.MinimumStorageDeposit(extraDepositWeight)
		amount := o.Amount()
		if amount < minDeposit {
			lastErr = fmt.Errorf("not enough storage deposit in output %s. Minimum %d, got %d",
				PathToString(path), minDeposit, amount)
			if !failFast {
				failedOutputs.WriteByte(i)
			}
			return !failFast
		}
		if amount > math.MaxUint64-sum {
			lastErr = fmt.Errorf("validateOutputsFailFast @ path %s: uint64 arithmetic overflow", PathToString(path))
			if !failFast {
				failedOutputs.WriteByte(i)
			}
			return !failFast
		}
		sum += amount
		return true
	}, branch)
	if lastErr != nil {
		util.Assertf(failFast || failedOutputs.Len() > 0, "failedOutputs.Len()>0")
		return 0, failedOutputs.Bytes(), lastErr
	}
	return sum, nil, nil
}

func (ctx *TxContext) UnlockParams(consumedOutputIdx, constraintIdx byte) []byte {
	return ctx.tree.MustBytesAtPath(Path(ledger.TransactionBranch, ledger.TxUnlockData, consumedOutputIdx, constraintIdx))
}

// runOutput checks constraints of the output one-by-one
func (ctx *TxContext) runOutput(consumedBranch bool, output *ledger.Output, path lazybytes.TreePath, spool *slicepool.SlicePool) (uint32, error) {
	blockPath := common.Concat(path, byte(0))
	var err error
	extraStorageDepositWeight := uint32(0)
	checkDuplicates := make(map[string]struct{})

	output.ForEachConstraint(func(idx byte, data []byte) bool {
		// checking for duplicated constraints in produced outputs
		if !consumedBranch {
			sd := string(data)
			if _, already := checkDuplicates[sd]; already {
				err = fmt.Errorf("duplicated constraints not allowed. Path %s", PathToString(blockPath))
				return false
			}
			checkDuplicates[sd] = struct{}{}
		}
		blockPath[len(blockPath)-1] = idx
		var res []byte
		var name string

		res, name, err = ctx.checkConstraint(data, blockPath, spool)
		if err != nil {
			err = fmt.Errorf("constraint '%s' failed with error '%v'. Path: %s", name, err, PathToString(blockPath))
			return false
		}
		if len(res) == 0 {
			var decomp string
			decomp, err = ledger.L().DecompileBytecode(data)
			if err != nil {
				decomp = fmt.Sprintf("(error while decompiling constraint: '%v')", err)
			}
			err = fmt.Errorf("constraint '%s' failed. Path: %s", decomp, PathToString(blockPath))
			return false
		}
		if len(res) == 4 {
			// 4 bytes long slice returned by the constraint is interpreted as 'true' and as uint32 extraStorageWeight
			extraStorageDepositWeight += binary.BigEndian.Uint32(res)
		}
		return true
	})
	if err != nil {
		return 0, err
	}
	return extraStorageDepositWeight, nil
}

func (ctx *TxContext) validateInputCommitmentSafe() error {
	return util.CatchPanicOrError(func() error {
		consumeOutputHash := ctx.ConsumedOutputHash()
		inputCommitment := ctx.InputCommitment()
		if !bytes.Equal(consumeOutputHash[:], inputCommitment) {
			return fmt.Errorf("hash of consumed outputs %v not equal to input commitment %v",
				easyfl_util.Fmt(consumeOutputHash[:]), easyfl_util.Fmt(inputCommitment))
		}
		return nil
	})
}

// ConsumedOutputHash is ias blake2b hash of the lazyarray composed of output data
func (ctx *TxContext) ConsumedOutputHash() [32]byte {
	consumedOutputBytes := ctx.tree.MustBytesAtPath(Path(ledger.ConsumedBranch, ledger.ConsumedOutputsBranch))
	return blake2b.Sum256(consumedOutputBytes)
}

func PathToString(path []byte) string {
	ret := "@"
	if len(path) == 0 {
		return ret + ".nil"
	}
	if len(path) >= 1 {
		switch path[0] {
		case ledger.TransactionBranch:
			ret += ".tx"
			if len(path) >= 2 {
				switch path[1] {
				case ledger.TxUnlockData:
					ret += ".unlock"
				case ledger.TxInputIDs:
					ret += ".inID"
				case ledger.TxOutputs:
					ret += ".out"
				case ledger.TxSignature:
					ret += ".sig"
				case ledger.TxTimestamp:
					ret += ".ts"
				case ledger.TxInputCommitment:
					ret += ".inhash"
				default:
					ret += "WRONG[1]"
				}
			}
			if len(path) >= 3 {
				ret += fmt.Sprintf("[%d]", path[2])
			}
			if len(path) >= 4 {
				ret += fmt.Sprintf(".block[%d]", path[3])
			}
			if len(path) >= 5 {
				ret += fmt.Sprintf("..%v", path[4:])
			}
		case ledger.ConsumedBranch:
			ret += ".consumed"
			if len(path) >= 2 {
				if path[1] != 0 {
					ret += ".WRONG[1]"
				} else {
					ret += ".[0]"
				}
			}
			if len(path) >= 3 {
				ret += fmt.Sprintf(".out[%d]", path[2])
			}
			if len(path) >= 4 {
				ret += fmt.Sprintf(".block[%d]", path[3])
			}
			if len(path) >= 5 {
				ret += fmt.Sprintf("..%v", path[4:])
			}
		default:
			ret += ".WRONG[0]"
		}
	}
	return ret
}

func constraintName(binCode []byte) string {
	if binCode[0] == 0 {
		return "array_constraint"
	}
	prefix, err := ledger.L().ParsePrefixBytecode(binCode)
	if err != nil {
		return fmt.Sprintf("unknown_constraint(%s)", easyfl_util.Fmt(binCode))
	}
	name, found := ledger.NameByPrefix(prefix)
	if found {
		return name
	}
	return fmt.Sprintf("constraint_call_prefix(%s)", easyfl_util.Fmt(prefix))
}

func (ctx *TxContext) evalConstraint(constr []byte, path lazybytes.TreePath, spool *slicepool.SlicePool) ([]byte, string, error) {
	if len(constr) == 0 {
		return nil, "", fmt.Errorf("constraint can't be empty")
	}
	var err error
	name := constraintName(constr)
	evalCtx := ctx.evalContext(path)
	if evalCtx.Trace() {
		evalCtx.PutTrace(fmt.Sprintf("--- check constraint '%s' at path %s", name, PathToString(path)))
	}

	var ret []byte
	if constr[0] == 0 {
		return nil, "", fmt.Errorf("binary code cannot begin with 0-byte")
	}
	ret, err = ledger.L().EvalFromBytecodeWithSlicePool(evalCtx, spool, constr)

	if evalCtx.Trace() {
		if err != nil {
			evalCtx.PutTrace(fmt.Sprintf("--- constraint '%s' at path %s: FAILED with '%v'", name, PathToString(path), err))
			printTraceIfEnabled(evalCtx)
		} else {
			if len(ret) == 0 {
				evalCtx.PutTrace(fmt.Sprintf("--- constraint '%s' at path %s: FAILED", name, PathToString(path)))
				printTraceIfEnabled(evalCtx)
			} else {
				evalCtx.PutTrace(fmt.Sprintf("--- constraint '%s' at path %s: OK", name, PathToString(path)))
			}
		}
	}

	return ret, name, err
}

// __printLogOnFail is global var for controlling printing failed validation trace or not
var __printLogOnFail atomic.Bool

func printTraceIfEnabled(evalCtx easyfl.GlobalData) {
	if __printLogOnFail.Load() {
		evalCtx.(*easyfl.GlobalDataLog).PrintLog()
	}
}
