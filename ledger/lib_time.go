package ledger

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/lunfardo314/proxima/util"
)

const (
	SlotByteLength = 4
	TickByteIndex
	SequencerBitMaskInTick = 0x01
	TimeByteLength         = SlotByteLength + 1 // bytes
	MaxSlot                = 0xffffffff         // 1 most significant bit must be 0
	MaxTickValue           = 0x7f               // 127
	MaxTime                = MaxSlot * TicksPerSlot
	TicksPerSlot           = MaxTickValue + 1
)

func TickDuration() time.Duration {
	return L().ID.TickDuration
}

func SlotDuration() time.Duration {
	return L().ID.SlotDuration()
}

// serialized timestamp is 5 bytes:
// - bytes 0-3 is big-endian slot
// - byte 4 is ticks << 1, i.e. last bit of timestamp is always 0

type (
	// Slot represents a particular time slot.
	// Starting slot 0 at genesis
	Slot uint32
	Tick uint8
	Time struct {
		Slot
		Tick
	}
)

var (
	NilLedgerTime      Time
	errWrongDataLength = fmt.Errorf("wrong data length")
	errWrongTickValue  = fmt.Errorf("wrong tick value")
)

// SlotFromBytes enforces 2 most significant bits of the first byte are 0
func SlotFromBytes(data []byte) (ret Slot, err error) {
	if len(data) != 4 {
		err = errWrongDataLength
		return
	}
	return Slot(binary.BigEndian.Uint32(data)), nil
}

func NewLedgerTime(slot Slot, t Tick) (ret Time) {
	util.Assertf(t <= MaxTickValue, "NewLedgerTime: invalid tick value %d", t)
	ret = Time{Slot: slot, Tick: t}
	return
}

func T(slot Slot, t Tick) Time {
	return NewLedgerTime(slot, t)
}

func TimeFromClockTime(nowis time.Time) Time {
	return L().ID.LedgerTimeFromClockTime(nowis)
}

func TimeNow() Time {
	return TimeFromClockTime(time.Now())
}

func ValidTime(ts Time) bool {
	return ts.Tick <= MaxTickValue
}

func TimeFromBytes(data []byte) (ret Time, err error) {
	if len(data) != TimeByteLength {
		err = errWrongDataLength
		return
	}
	if data[TickByteIndex]&SequencerBitMaskInTick != 0 {
		err = errWrongTickValue
		return
	}
	ret = Time{
		Slot: Slot(binary.BigEndian.Uint32(data[:SlotByteLength])),
		Tick: Tick(data[TickByteIndex] >> 1),
	}
	return
}

// TimeFromTicksSinceGenesis converts absolute value of ticks since genesis into the time value
func TimeFromTicksSinceGenesis(ticks int64) (ret Time, err error) {
	if ticks < 0 || ticks > MaxTime {
		err = fmt.Errorf("TimeFromTicksSinceGenesis: wrong int64")
		return
	}
	ret = Time{
		Slot: Slot(ticks / TicksPerSlot),
		Tick: Tick(ticks % TicksPerSlot),
	}
	return
}

func (s Slot) PutBytes(b []byte) {
	binary.BigEndian.PutUint32(b, uint32(s))
}

func (s Slot) Bytes() []byte {
	ret := make([]byte, SlotByteLength)
	s.PutBytes(ret)
	return ret
}

func (s Slot) Hex() string {
	return fmt.Sprintf("0x%s", hex.EncodeToString(s.Bytes()))
}

func (t Time) IsSlotBoundary() bool {
	return t.Tick == 0 && t != NilLedgerTime
}

func (t Time) UnixNano() int64 {
	return L().ID.GenesisTime().Add(time.Duration(t.TicksSinceGenesis()) * TickDuration()).UnixNano()
}

func (t Time) Time() time.Time {
	return time.Unix(0, t.UnixNano())
}

func (t Time) NextSlotBoundary() Time {
	if t.IsSlotBoundary() {
		return t
	}
	util.Assertf(t.Slot < MaxSlot, "t.Slot < MaxSlot")
	return NewLedgerTime(t.Slot+1, 0)
}

func (t Time) TicksToNextSlotBoundary() int {
	if t.IsSlotBoundary() {
		return 0
	}
	return TicksPerSlot - int(t.Tick)
}

func (t Time) Bytes() []byte {
	ret := make([]byte, TimeByteLength)
	binary.BigEndian.PutUint32(ret[:SlotByteLength], uint32(t.Slot))
	ret[TickByteIndex] = uint8(t.Tick) << 1
	return ret[:]
}

func (t Time) String() string {
	return fmt.Sprintf("%d|%d", t.Slot, t.Tick)
}

func (t Time) Source() string {
	return fmt.Sprintf("0x%s", hex.EncodeToString(t.Bytes()))
}

func (t Time) AsFileName() string {
	return fmt.Sprintf("%d_%d", t.Slot, t.Tick)
}

func (t Time) Short() string {
	e := t.Slot % 1000
	return fmt.Sprintf(".%d|%d", e, t.Tick)
}

func (t Time) After(t1 Time) bool {
	return t.TicksSinceGenesis() > t1.TicksSinceGenesis()
}

func (t Time) AfterOrEqual(t1 Time) bool {
	return !t.Before(t1)
}

func (t Time) Before(t1 Time) bool {
	return t.TicksSinceGenesis() < t1.TicksSinceGenesis()
}

func (t Time) BeforeOrEqual(t1 Time) bool {
	return !t.After(t1)
}

func (t Time) Hex() string {
	return fmt.Sprintf("0x%s", hex.EncodeToString(t.Bytes()))
}

func (t Time) TicksSinceGenesis() int64 {
	return int64(t.Slot)*TicksPerSlot + int64(t.Tick)
}

// DiffTicks returns difference in ticks between two timestamps:
// < 0 is t1 is before t2
// > 0 if t2 is before t1
// (i.e. t1 - t2)
func DiffTicks(t1, t2 Time) int64 {
	return t1.TicksSinceGenesis() - t2.TicksSinceGenesis()
}

// ValidTransactionPace return true is subsequent input and target non-sequencer tx timestamps make a valid pace
func ValidTransactionPace(t1, t2 Time) bool {
	return DiffTicks(t2, t1) >= int64(TransactionPace())
}

// ValidSequencerPace return true is subsequent input and target sequencer tx timestamps make a valid pace
func ValidSequencerPace(t1, t2 Time) bool {
	return DiffTicks(t2, t1) >= int64(TransactionPaceSequencer())
}

// AddTicks adds ticks to timestamp. Ticks can be negative
func (t Time) AddTicks(ticks int) Time {
	ret, err := TimeFromTicksSinceGenesis(t.TicksSinceGenesis() + int64(ticks))
	util.AssertNoError(err)
	return ret
}

// AddSlots adds slots to timestamp
func (t Time) AddSlots(slot Slot) Time {
	return t.AddTicks(int(slot << 8))
}

func MaximumTime(ts ...Time) Time {
	return util.Maximum(ts, func(ts1, ts2 Time) bool {
		return ts1.Before(ts2)
	})
}
