package util

import (
	"math/big"

	"golang.org/x/crypto/blake2b"
)

// RandomFromSeed returns a random uin64 number in [0, scale) by scaling the blake2b hash
// value as BigInt to the interval [0, scale). The 'scale' value itself is not included
func RandomFromSeed(data []byte, scale uint64) uint64 {
	h := blake2b.Sum256(data)
	ret := new(big.Int).SetBytes(h[:])
	ret.Mul(ret, new(big.Int).SetUint64(scale))
	ret.Rsh(ret, 256)
	return ret.Uint64()
}
