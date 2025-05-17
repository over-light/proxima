package tests

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"
)

// Validating and making sense of inflation-related constants

func TestInflation(t *testing.T) {
	t.Logf("slotInflationBase: %s", util.Th(ledger.L().ID.SlotInflationBase))
	t.Logf("linearInflationSlots: %s", util.Th(ledger.L().ID.LinearInflationSlots))
	r, err := ledger.L().EvalFromSource(nil, "div(constInitialSupply, constSlotInflationBase)")
	require.NoError(t, err)
	minAmountOnSlot := func(n int) uint64 {
		return binary.BigEndian.Uint64(r) + uint64(n)
	}
	t.Logf("div(constInitialSupply, constSlotInflationBase): %s", util.Th(minAmountOnSlot(0)))

	t.Run("1", func(t *testing.T) {
		ledger.L().MustEqual("constGenesisTimeUnix", fmt.Sprintf("u64/%d", ledger.L().ID.GenesisTimeUnix))
	})
	t.Run("chain inflation", func(t *testing.T) {
		calc := func(tsIn, tsOut base.LedgerTime, amount uint64) uint64 {
			inflation := ledger.L().CalcChainInflationAmount(tsIn, tsOut, amount)
			t.Logf("chainInflation(%s, %s, %s) = %s", tsIn.String(), tsOut.String(), util.Th(amount), util.Th(inflation))
			return inflation
		}
		i := calc(base.T(0, 0), base.T(0, 1), ledger.DefaultInitialSupply)
		require.EqualValues(t, ledger.L().ID.SlotInflationBase, i)

		i = calc(base.T(0, 0), base.T(0, 50), ledger.DefaultInitialSupply)
		require.EqualValues(t, ledger.L().ID.SlotInflationBase, i)

		i = calc(base.T(0, 0), base.T(0, 127), ledger.DefaultInitialSupply)
		require.EqualValues(t, ledger.L().ID.SlotInflationBase, i)

		i = calc(base.T(0, 0), base.T(1, 0), ledger.DefaultInitialSupply)
		require.EqualValues(t, 0, i)

		i = calc(base.T(0, 1), base.T(0, 127), ledger.DefaultInitialSupply)
		require.EqualValues(t, 0, i)

		i = calc(base.T(0, 1), base.T(1, 0), ledger.DefaultInitialSupply)
		require.EqualValues(t, 0, i)

		for s := 1; s < 30; s++ {
			m := s
			if uint64(m) > ledger.L().ID.LinearInflationSlots {
				m = int(ledger.L().ID.LinearInflationSlots)
			}
			i = calc(base.T(0, 1), base.T(base.Slot(s), 1), ledger.DefaultInitialSupply)
			require.EqualValues(t, int(ledger.L().ID.SlotInflationBase)*m, int(i))

			i = calc(base.T(0, 1), base.T(base.Slot(s), 127), ledger.DefaultInitialSupply)
			require.EqualValues(t, int(ledger.L().ID.SlotInflationBase)*m, i)

			i = calc(base.T(0, 1), base.T(base.Slot(s), 1), ledger.DefaultInitialSupply/100_000)
			require.EqualValues(t, int(ledger.L().ID.SlotInflationBase)*m/100_000, int(i))

			i = calc(base.T(0, 1), base.T(base.Slot(s), 127), ledger.DefaultInitialSupply/100_000)
			require.EqualValues(t, int(ledger.L().ID.SlotInflationBase)*m/100_000, i)
		}
	})
}

func TestBranchInflationBonusDistrib(t *testing.T) {
	const numBins = 100
	bins := make([]int, numBins)
	for i := 0; i < 10000; i++ {
		rndData := blake2b.Sum256([]byte(fmt.Sprintf("test%dtest%d", i, i)))
		v := ledger.L().BranchInflationBonusFromRandomnessProof(rndData[:])
		bins[int(v)%numBins]++
	}
	sum := 0
	minVal := bins[0]
	maxVal := bins[0]
	for i, v := range bins {
		sum += v
		minVal = min(minVal, v)
		maxVal = max(maxVal, v)
		t.Logf("bin %3d: %d", i, v)
	}
	t.Logf("avg: %d, min: %d, max:%d", sum/numBins, minVal, maxVal)
}

func TestBranchInflationBonus(t *testing.T) {
	proofStr := []string{
		"02fe7b0149150a8e0a8ee9cf0101a85da7fb2f6cb0c496a9c0503289899eec823e24796b0e607abe2c76e2ee1b85fdb93f097ad203c46710b958bf656aaeca732ce412b4543970b39bf416f50c52f28339",
		"02b306479a5da05b02d6ec7a2abc71b3da3b2013e3c248f62ccc316b1214ff7d489349b05a8abccc831b25f0aa88b4ecf00ced8a06e541d8a8334f7d93839daec0c1aa1a00ac50dcfe1396ecbc75e1b95a",
		"027a335cf45148ebab7d32dec71ecb56e77b107fe6b2c7bd3cba8356e45d68b20c7bb2a8cc658bde80e6b39d004af7757a063388b3c10337f75c3d4694b4d98f4d07c903c64da84487b8ef2ad4e4c06c45",
		"02b306479a5da05b02d6ec7a2abc71b3da3b2013e3c248f62ccc316b1214ff7d489349b05a8abccc831b25f0aa88b4ecf00ced8a06e541d8a8334f7d93839daec0c1aa1a00ac50dcfe1396ecbc75e1b95a",
		"03f807332ed9891dbbdf13c133fd5c6a3cc19a07793085fd02347fde2515c8c9fbe50f3989b9d899f1fa2516e48b6537a50f903a89954038ce6f8c053a21141fa26ca1a67a8261b69393d090976381818f",
		"03ade7ed7813b7d8f2e50e355bf05bc137d9d6f24babd069335ca96c0e10548ae540095f584435c631d864499ce6aae9a10def4fb1ce1b1d835c67d284978421c561e7699f78d519515cb05ee74afc399a",
		"025c21dd3c95c04b9cba48581ba6aca2b60bfbe2fa7e4b25fda99c298fde6db03bcd2b8eaa2a5b6982a6d942901fa00b8a0dc9b54e2251e2567011eb26ea08f9f1c4436577bfc1250e851ad1003cc8f7e5",
	}
	for _, proof := range proofStr {
		proofBin, err := hex.DecodeString(proof)
		require.NoError(t, err)
		vDirect := ledger.L().BranchInflationBonusDirect(proofBin)
		vLedger := ledger.L().BranchInflationBonusFromRandomnessProof(proofBin)
		require.EqualValues(t, vDirect, vLedger)
		t.Logf("%s\n           vLedger =  %6d, vDirect: %6d", proof, vLedger, vDirect)
	}
}
