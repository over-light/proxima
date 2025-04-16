package tests

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/util"
	"github.com/stretchr/testify/require"
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
		calc := func(tsIn, tsOut ledger.Time, amount uint64) uint64 {
			inflation := ledger.L().CalcChainInflationAmount(tsIn, tsOut, amount)
			t.Logf("chainInflation(%s, %s, %s) = %s", tsIn.String(), tsOut.String(), util.Th(amount), util.Th(inflation))
			return inflation
		}
		i := calc(ledger.T(0, 0), ledger.T(0, 1), ledger.DefaultInitialSupply)
		require.EqualValues(t, ledger.L().ID.SlotInflationBase, i)

		i = calc(ledger.T(0, 0), ledger.T(0, 50), ledger.DefaultInitialSupply)
		require.EqualValues(t, ledger.L().ID.SlotInflationBase, i)

		i = calc(ledger.T(0, 0), ledger.T(0, 127), ledger.DefaultInitialSupply)
		require.EqualValues(t, ledger.L().ID.SlotInflationBase, i)

		i = calc(ledger.T(0, 0), ledger.T(1, 0), ledger.DefaultInitialSupply)
		require.EqualValues(t, 0, i)

		i = calc(ledger.T(0, 1), ledger.T(0, 127), ledger.DefaultInitialSupply)
		require.EqualValues(t, 0, i)

		i = calc(ledger.T(0, 1), ledger.T(1, 0), ledger.DefaultInitialSupply)
		require.EqualValues(t, 0, i)

		for s := 1; s < 30; s++ {
			m := s
			if uint64(m) > ledger.L().ID.LinearInflationSlots {
				m = int(ledger.L().ID.LinearInflationSlots)
			}
			i = calc(ledger.T(0, 1), ledger.T(ledger.Slot(s), 1), ledger.DefaultInitialSupply)
			require.EqualValues(t, int(ledger.L().ID.SlotInflationBase)*m, int(i))

			i = calc(ledger.T(0, 1), ledger.T(ledger.Slot(s), 127), ledger.DefaultInitialSupply)
			require.EqualValues(t, int(ledger.L().ID.SlotInflationBase)*m, i)

			i = calc(ledger.T(0, 1), ledger.T(ledger.Slot(s), 1), ledger.DefaultInitialSupply/100_000)
			require.EqualValues(t, int(ledger.L().ID.SlotInflationBase)*m/100_000, int(i))

			i = calc(ledger.T(0, 1), ledger.T(ledger.Slot(s), 127), ledger.DefaultInitialSupply/100_000)
			require.EqualValues(t, int(ledger.L().ID.SlotInflationBase)*m/100_000, i)
		}
	})
}
