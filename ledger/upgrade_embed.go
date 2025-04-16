package ledger

import (
	"github.com/lunfardo314/easyfl"
	"github.com/lunfardo314/proxima/util"
)

func (lib *Library) mustUpgradeWithEmbedded() {
	err := lib.UpgradeFromYAML([]byte(_upgradeEmbeddedYAML), _embeddedFunctions(lib))
	util.AssertNoError(err)
}

func _embeddedFunctions(lib *Library) func() map[string]easyfl.EmbeddedFunction {
	return func() map[string]easyfl.EmbeddedFunction {
		return map[string]easyfl.EmbeddedFunction{
			"@":                evalPath,
			"@Path":            evalAtPath,
			"@Array8":          evalAtArray8,
			"ArrayLength8":     evalNumElementsOfArray,
			"ticksBefore":      evalTicksBefore64,
			"vrfVerify":        evalVRFVerify,
			"callLocalLibrary": lib.evalCallLocalLibrary,
		}
	}
}

const _upgradeEmbeddedYAML string = `
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
