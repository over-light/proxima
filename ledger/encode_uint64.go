package ledger

import (
	"encoding/binary"
	"fmt"

	"github.com/lunfardo314/proxima/util"
)

// Uint64FromBytes takes any 8 or less bytes, padds with 0 prefix up to 8 bytes size and makes uin64 big-endian
func Uint64FromBytes(data []byte) (uint64, error) {
	if len(data) > 8 {
		return 0, fmt.Errorf("Uint64FromBytes: can't be more than 8 bytes")
	}
	var paddedData [8]byte
	copy(paddedData[8-len(data):], data)

	return binary.BigEndian.Uint64(paddedData[:]), nil
}

func MustUint64FromBytes(data []byte) uint64 {
	ret, err := Uint64FromBytes(data)
	util.AssertNoError(err)
	return ret
}

func TrimPrefixZeroBytes(data []byte) []byte {
	for i := 0; i < len(data); i++ {
		if data[i] != 0 {
			return data[i:]
		}
	}
	return nil
}
