package ledger

import (
	"encoding/binary"

	"github.com/lunfardo314/easyfl"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/lazybytes"
	"github.com/lunfardo314/unitrie/common"
	"github.com/yoseplee/vrf"
)

var _unboundedEmbedded = map[string]easyfl.EmbeddedFunction{
	"@":            evalPath,
	"@Path":        evalAtPath,
	"@Array8":      evalAtArray8,
	"ArrayLength8": evalNumElementsOfArray,
	"ticksBefore":  evalTicksBefore64,
	"vrfVerify":    evalVRFVerify,
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

func EmbeddedFunctions(lib *easyfl.Library) func(string) easyfl.EmbeddedFunction {
	return func(sym string) easyfl.EmbeddedFunction {
		if ef, found := _unboundedEmbedded[sym]; found {
			return ef
		}
		if sym == "callLocalLibrary" {
			return makeEvalCallLocalLibraryEmbeddedFunc(lib)
		}
		return nil
	}
}

const _definitionsEmbeddedYAML string = `
functions:
# short
   -
      sym: "@"
      description: "returns path in the transaction of the validity constraint being evaluated"
      numArgs: 0
      embedded: true
      short: true
   -
      sym: "@Path"
      description: "returns element of the transaction at path $0"
      numArgs: 1
      embedded: true
      short: true
# long
   -
      sym: "@Array8"
      description: "returns element of the serialized lazy array at index $0"
      numArgs: 2
      embedded: true
   -
      sym: ArrayLength8
      description: "returns number of elements of lazy array as 1-byte long value"
      numArgs: 1
      embedded: true
   -
      sym: ticksBefore
      description: "number of ticks between timestamps $0 and $1 as big-endian uint64 if $0 is before $1, or 0x otherwise"
      numArgs: 2
      embedded: true
   -
      sym: vrfVerify
      description: "Verifiable Random Function (VRF) verification, where $0 is public key, $1 is proof, $2 is message"
      numArgs: 3
      embedded: true
   -
      sym: callLocalLibrary
      description: TBD
      numArgs: -1
      embedded: true
`

// embedded functions

func evalPath(par *easyfl.CallParams) []byte {
	return par.AllocData([]byte(par.DataContext().(*DataContext).Path())...)
}

func evalAtPath(par *easyfl.CallParams) []byte {
	return par.AllocData(par.DataContext().(*DataContext).DataTree().BytesAtPath(par.Arg(0))...)
}

func evalAtArray8(par *easyfl.CallParams) []byte {
	arr := lazybytes.ArrayFromBytesReadOnly(par.Arg(0))
	idx := par.Arg(1)
	if len(idx) != 1 {
		panic("evalAtArray8: 1-byte value expected")
	}
	return arr.At(int(idx[0]))
}

func evalNumElementsOfArray(par *easyfl.CallParams) []byte {
	arr := lazybytes.ArrayFromBytesReadOnly(par.Arg(0))
	return par.AllocData(byte(arr.NumElements()))
}

// evalVRFVerify: embedded VRF verifier. Dependency on unverified external crypto library
// arg 0 - pubkey
// arg 1 - proof
// arg 2 - msg
func evalVRFVerify(par *easyfl.CallParams) []byte {
	var ok bool
	err := util.CatchPanicOrError(func() error {
		var err1 error
		ok, err1 = vrf.Verify(par.Arg(0), par.Arg(1), par.Arg(2))
		return err1
	})
	if err != nil {
		par.Trace("'vrfVerify embedded' failed with: %v", err)
	}
	if err == nil && ok {
		return par.AllocData(0xff)
	}
	return nil
}

// CompileLocalLibrary compiles local library and serializes it as lazy array
// TODO move to easyfl together with lazybytes
func CompileLocalLibrary(source string, lib *easyfl.Library) ([]byte, error) {
	libBin, err := lib.CompileLocalLibrary(source)
	if err != nil {
		return nil, err
	}
	ret := lazybytes.MakeArrayFromDataReadOnly(libBin...)
	return ret.Bytes(), nil
}

func makeEvalCallLocalLibraryEmbeddedFunc(lib *easyfl.Library) easyfl.EmbeddedFunction {
	return func(ctx *easyfl.CallParams) []byte {
		// arg 0 - local library binary (as lazy array)
		// arg 1 - 1-byte index of then function in the library
		// arg 2 ... arg 15 optional arguments
		arr := lazybytes.ArrayFromBytesReadOnly(ctx.Arg(0))
		libData := arr.Parsed()
		idx := ctx.Arg(1)
		if len(idx) != 1 || int(idx[0]) >= len(libData) {
			ctx.TracePanic("evalCallLocalLibrary: wrong function index")
		}
		ret := lib.CallLocalLibrary(ctx.Slice(2, ctx.Arity()), libData, int(idx[0]))
		ctx.Trace("evalCallLocalLibrary: lib#%d -> %s", idx[0], easyfl.Fmt(ret))
		return ret
	}
}

// arg 0 and arg 1 are timestamps (5 bytes each)
// returns:
// nil, if ts1 is before ts0
// number of ticks between ts0 and ts1 otherwise, as big-endian uint64
func evalTicksBefore64(par *easyfl.CallParams) []byte {
	ts0bin, ts1bin := par.Arg(0), par.Arg(1)
	ts0, err := base.TimeFromBytes(ts0bin)
	if err != nil {
		par.TracePanic("evalTicksBefore64: %v", err)
	}
	ts1, err := base.TimeFromBytes(ts1bin)
	if err != nil {
		par.TracePanic("evalTicksBefore64: %v", err)
	}
	diff := base.DiffTicks(ts1, ts0)
	if diff < 0 {
		// ts1 is before ts0
		return nil
	}
	ret := par.Alloc(8)
	binary.BigEndian.PutUint64(ret, uint64(diff))
	return ret
}
