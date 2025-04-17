package ledger

import (
	"time"

	"github.com/lunfardo314/proxima/ledger/base"
)

func TickDuration() time.Duration {
	return L().ID.TickDuration
}

func SlotDuration() time.Duration {
	return L().ID.SlotDuration()
}

func TimeFromClockTime(nowis time.Time) base.LedgerTime {
	return L().ID.LedgerTimeFromClockTime(nowis)
}

func UnixNanoFromLedgerTime(t base.LedgerTime) int64 {
	return L().ID.GenesisTime().Add(time.Duration(t.TicksSinceGenesis()) * TickDuration()).UnixNano()
}

func TimeNow() base.LedgerTime {
	return TimeFromClockTime(time.Now())
}

// ValidTransactionPace return true if input and target non-sequencer tx timestamps make a valid pace
func ValidTransactionPace(t1, t2 base.LedgerTime) bool {
	return base.DiffTicks(t2, t1) >= int64(TransactionPace())
}

// ValidSequencerPace return true if input and target sequencer tx timestamps make a valid pace
func ValidSequencerPace(t1, t2 base.LedgerTime) bool {
	return base.DiffTicks(t2, t1) >= int64(TransactionPaceSequencer())
}

func ClockTime(t base.LedgerTime) time.Time {
	return time.Unix(0, UnixNanoFromLedgerTime(t))
}
