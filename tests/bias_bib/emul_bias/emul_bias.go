package main

import (
	"fmt"

	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/unitrie/common"
	"golang.org/x/crypto/blake2b"
)

const (
	nRepeat  = 1000
	nSecrets = 5
	nBuckets
)

func main() {
	secrets := make([][32]byte, nSecrets)
	for i := 0; i < nSecrets; i++ {
		secrets[i] = blake2b.Sum256([]byte(fmt.Sprint("%d%d%d", i, i, i)))
	}

	var curHash [32]byte

	options := make([][32]byte, nSecrets)
	buckets := make([]int, nBuckets)

	for i := 0; i < nRepeat; i++ {
		for j := range options {
			options[j] = blake2b.Sum256(common.Concat(curHash[:], secrets[j][:]))
		}

		curHash = util.Maximum(options, func(el1, el2 [32]byte) bool {
			return base.RandomFromSeed(el1[:], 5_000_000) < base.RandomFromSeed(el2[:], 5_000_000)
		})
		rnd := base.RandomFromSeed(curHash[:], 5_000_000)
		//fmt.Printf("%4d      rnd = %s\n", i, util.Th(rnd))

		nBucket := (nBuckets * int(rnd)) / 5_000_000
		buckets[nBucket]++
	}
	fmt.Println()
	for i := range buckets {
		fmt.Printf("bucket #%d     %4d (%.1f%%)\n", i, buckets[i], float64(buckets[i])*100/float64(nRepeat))
	}
}
