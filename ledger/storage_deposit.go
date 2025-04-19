package ledger

import (
	"encoding/binary"
	"fmt"

	"github.com/lunfardo314/proxima/util"
)

// TODO proper calculation of the storage deposit

func StorageDeposit(nBytes int) uint64 {
	src := fmt.Sprintf("storageDeposit(u32/%d)", nBytes)
	res, err := L().EvalFromSource(nil, src)
	util.AssertNoError(err)
	return binary.BigEndian.Uint64(res)
}
