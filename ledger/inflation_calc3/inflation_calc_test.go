package inflation_calc3

import (
	"testing"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/lines"
)

func TestInflation3(t *testing.T) {
	t.Logf("----- tick duration: %v, slots per year: %s", ledger.L().ID.TickDuration, util.Th(ledger.L().ID.SlotsPerYear()))

	ln := lines.New("   ")
	const (
		initSupply            = 1_000_000_000_000_000
		inflationTargetAnnual = initSupply / 10
		inflationEpochSlots   = 100_000
		branchInflationBonus  = 5_000_000
	)
	ln.Add("initSupply: %s", util.Th(initSupply))
	ln.Add("inflationTargetAnnual: %s", util.Th(inflationTargetAnnual))
	ln.Add("inflationEpochSlots: %s", util.Th(inflationEpochSlots))
	ln.Add("branchInflationBonus: %s", util.Th(branchInflationBonus))
	epochsPerYear := float64(ledger.L().ID.SlotsPerYear()) / float64(inflationEpochSlots)
	ln.Add("epochsPerYear: %.03f", epochsPerYear)
	inflationTargetForEpoch := inflationTargetAnnual / epochsPerYear
	ln.Add("inflationTargetForEpoch: %s", util.Th(int(inflationTargetForEpoch)))
	inflationTargetForSlot := inflationTargetForEpoch / inflationEpochSlots
	ln.Add("inflationTargetForSlot: %s", util.Th(int(inflationTargetForSlot)))
	inflationTargetForSlotConst := uint64(32_000_000) // uint64(inflationTargetForEpoch / inflationEpochSlots)
	ln.Add("inflationTargetForSlotConst: %s", util.Th(inflationTargetForSlotConst))

	t.Logf("\n%s", ln.String())

	P := func(n int) uint64 {
		return initSupply/inflationTargetForSlotConst + uint64(n) - 1
	}

	const years = 100

	supply := uint64(initSupply)
	slotsPerYear := ledger.L().ID.SlotsPerYear()
	year := 1
	supplyYearStart := uint64(initSupply)

	for s := 0; s < slotsPerYear*years; s++ {
		p := P(s)
		if s > 0 && s%slotsPerYear == 0 {
			inflationAnnual := supply - supplyYearStart
			t.Logf("EoY %4d, supply %s, inflation: %s, rate YoY: %.02f%%",
				year, util.Th(supply), util.Th(inflationAnnual), 100*float64(inflationAnnual)/float64(supplyYearStart))
			supplyYearStart = supply
			year++
		}
		supply += supply/p + branchInflationBonus
	}
	t.Logf("MaxUint64: %s", util.Th(uint64(0xffffffffffffffff)))
}
