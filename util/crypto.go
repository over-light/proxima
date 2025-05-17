package util

import (
	"math/big"
)

// ScaleBytesAsBigInt interprets 'data' as BigInteger (big-endian).
// It returns a random uin64 number in [0, scale] by scaling the big integer
// value to the interval [0, scale). The 'scale' value itself is not included
func ScaleBytesAsBigInt(data []byte, scale uint64) uint64 {
	ret := new(big.Int).SetBytes(data)
	ret.Mul(ret, new(big.Int).SetUint64(scale))
	ret.Rsh(ret, 256)
	return ret.Uint64()
}
