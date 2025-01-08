package tests

import (
	"fmt"
	"testing"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/util"
)

// Validating and making sense of inflation-related constants

func TestInflation(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		ledger.L().MustEqual("constGenesisTimeUnix", fmt.Sprintf("u64/%d", ledger.L().ID.GenesisTimeUnix))
	})
	t.Run("chain inflation", func(t *testing.T) {
		run := func(tsIn, tsOut ledger.Time, amount, delayed uint64) uint64 {
			inflation := ledger.L().CalcChainInflationAmount(tsIn, tsOut, amount)
			t.Logf("chainInflation(%s, %s, %s, %s) = %s",
				tsIn.String(), tsOut.String(), util.Th(amount), util.Th(delayed), util.Th(inflation))
			return inflation
		}
		t.Logf("slot inflation base: %s", util.Th(ledger.L().ID.SlotInflationBase))
		tsIn := ledger.NewLedgerTime(0, 0)
		tsOut := tsIn.AddTicks(1)
		run(tsIn, tsOut, 1, 0)
		run(tsIn, tsOut, ledger.DefaultInitialSupply, 0)
		run(tsIn, tsOut, ledger.DefaultInitialSupply/2, 0)
		run(tsIn, tsOut, ledger.DefaultInitialSupply/3, 0)
		run(tsIn, tsOut, ledger.DefaultInitialSupply/10, 0)
		run(tsIn, tsOut, ledger.DefaultInitialSupply/11, 0)
		run(tsIn, tsOut, ledger.DefaultInitialSupply/20, 0)
		run(tsIn, tsOut, ledger.DefaultInitialSupply/1000, 0)
		run(tsIn, tsOut, ledger.DefaultInitialSupply/100000, 0)
		run(tsIn, tsOut, uint64(ledger.DefaultInitialSupply/(150000+8549)), 0)
		run(tsIn, tsOut, uint64(ledger.DefaultInitialSupply/(150000+8550)), 0)
		run(tsIn, tsOut, uint64(ledger.DefaultInitialSupply/(150000+8550)), 1336)

		tsIn = ledger.NewLedgerTime(0, 0)
		tsOut = tsIn.AddTicks(1199)
		run(tsIn, tsOut, 1, 0)
		run(tsIn, tsOut, ledger.DefaultInitialSupply, 0)
		run(tsIn, tsOut, ledger.DefaultInitialSupply/2, 0)
		run(tsIn, tsOut, ledger.DefaultInitialSupply/3, 0)
		run(tsIn, tsOut, ledger.DefaultInitialSupply/10, 0)
		run(tsIn, tsOut, ledger.DefaultInitialSupply/11, 0)
		run(tsIn, tsOut, ledger.DefaultInitialSupply/20, 0)
		run(tsIn, tsOut, ledger.DefaultInitialSupply/1000, 0)
		run(tsIn, tsOut, ledger.DefaultInitialSupply/100000, 0)
		run(tsIn, tsOut, uint64(ledger.DefaultInitialSupply/(150000+8549)), 0)
		run(tsIn, tsOut, uint64(ledger.DefaultInitialSupply/(150000+8550)), 0)

	})
}
