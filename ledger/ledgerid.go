package ledger

import (
	"encoding/hex"
	"time"

	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/lines"
)

const (
	GenesisOutputIndex     = byte(0)
	GenesisStemOutputIndex = byte(1)
)

func (id *IdentityParameters) GenesisTime() time.Time {
	return time.Unix(int64(id.GenesisTimeUnix), 0)
}

func (id *IdentityParameters) GenesisTimeUnixNano() int64 {
	return time.Unix(int64(id.GenesisTimeUnix), 0).UnixNano()
}

func (id *IdentityParameters) GenesisControlledAddress() AddressED25519 {
	return AddressED25519FromPublicKey(id.GenesisControllerPublicKey)
}

// TimeToTicksSinceGenesis converts time value into ticks since genesis
func (id *IdentityParameters) TimeToTicksSinceGenesis(nowis time.Time) int64 {
	timeSinceGenesis := nowis.Sub(id.GenesisTime())
	return int64(timeSinceGenesis / id.TickDuration)
}

func (id *IdentityParameters) LedgerTimeFromClockTime(nowis time.Time) base.LedgerTime {
	ret, err := base.TimeFromTicksSinceGenesis(id.TimeToTicksSinceGenesis(nowis))
	util.AssertNoError(err)
	return ret
}

func (id *IdentityParameters) SlotDuration() time.Duration {
	return id.TickDuration * time.Duration(base.TicksPerSlot)
}

func (id *IdentityParameters) SlotsPerDay() int {
	return int(24 * time.Hour / id.SlotDuration())
}

func (id *IdentityParameters) SlotsPerYear() int {
	return 365 * id.SlotsPerDay()
}

func (id *IdentityParameters) TicksPerYear() int {
	return id.SlotsPerYear() * base.TicksPerSlot
}

func (id *IdentityParameters) OriginChainID() ChainID {
	oid := GenesisOutputID()
	return MakeOriginChainID(oid)
}

func (id *IdentityParameters) IsPreBranchConsolidationTimestamp(ts base.LedgerTime) bool {
	return uint8(ts.Tick) > base.MaxTickValue-id.PreBranchConsolidationTicks
}

func (id *IdentityParameters) IsPostBranchConsolidationTimestamp(ts base.LedgerTime) bool {
	return uint8(ts.Tick) >= id.PostBranchConsolidationTicks
}

func (id *IdentityParameters) EnsurePostBranchConsolidationConstraintTimestamp(ts base.LedgerTime) base.LedgerTime {
	if id.IsPostBranchConsolidationTimestamp(ts) {
		return ts
	}
	return base.NewLedgerTime(ts.Slot, base.Tick(id.PostBranchConsolidationTicks))
}

func (id *IdentityParameters) String() string {
	return id.Lines().String()
}

func (id *IdentityParameters) Lines(prefix ...string) *lines.Lines {
	originChainID := id.OriginChainID()
	return lines.New(prefix...).
		Add("Description: '%s'", id.Description).
		Add("Initial supply: %s", util.Th(id.InitialSupply)).
		Add("Genesis controller public key: %s", hex.EncodeToString(id.GenesisControllerPublicKey)).
		Add("Genesis controller address (calculated): %s", id.GenesisControlledAddress().String()).
		Add("Genesis Unix time: %d (%s)", id.GenesisTimeUnix, id.GenesisTime().Format(time.RFC3339)).
		Add("ClockTime tick duration: %v", id.TickDuration).
		Add("Slot inflation base (constant C): %s", util.Th(id.SlotInflationBase)).
		Add("Linear inflation slots (constant lambda): %s", util.Th(id.LinearInflationSlots)).
		Add("Constant initial supply/slot inflation base: %s", util.Th(id.InitialSupply/id.SlotInflationBase)).
		Add("Branch inflation bonus base: %s", util.Th(id.BranchInflationBonusBase)).
		Add("Pre-branch consolidation ticks: %v", id.PreBranchConsolidationTicks).
		Add("Post-branch consolidation ticks: %v", id.PostBranchConsolidationTicks).
		Add("Minimum amount on sequencer: %s", util.Th(id.MinimumAmountOnSequencer)).
		Add("Transaction pace: %d", id.TransactionPace).
		Add("Sequencer pace: %d", id.TransactionPaceSequencer).
		Add("VB cost: %d", id.VBCost).
		Add("Max number of endorsements: %d", id.MaxNumberOfEndorsements).
		Add("Origin chain id (calculated): %s", originChainID.String())
}

func (id *IdentityParameters) TimeConstantsToString() string {
	nowis := time.Now()
	timestampNowis := id.LedgerTimeFromClockTime(nowis)

	// TODO sometimes fails
	//util.Assertf(util.Abs(nowis.UnixNano()-timestampNowis.UnixNano()) < int64(TickDuration()),
	//	"nowis.UnixNano()(%d)-timestampNowis.UnixNano()(%d) = %d < int64(TickDuration())(%d)",
	//	nowis.UnixNano(), timestampNowis.UnixNano(), nowis.UnixNano()-timestampNowis.UnixNano(), int64(TickDuration()))

	maxYears := base.MaxSlot / (id.SlotsPerDay() * 365)
	return lines.New().
		Add("TickDuration = %v", id.TickDuration).
		Add("SlotDuration = %v", id.SlotDuration()).
		Add("SlotsPerDay = %d", id.SlotsPerDay()).
		Add("MaxYears = %d", maxYears).
		Add("seconds per year = %d", 60*60*24*365).
		Add("GenesisTime = %v", id.GenesisTime().Format(time.StampNano)).
		Add("nowis %v", nowis.Format(time.StampNano)).
		Add("nowis nano %d", nowis.UnixNano()).
		Add("GenesisTimeUnix = %d", id.GenesisTimeUnix).
		Add("GenesisTimeUnixNano = %d", id.GenesisTimeUnixNano()).
		Add("ticks since genesis: %d", id.TimeToTicksSinceGenesis(nowis)).
		Add("timestampNowis = %s ", timestampNowis.String()).
		Add("timestampNowis.ClockTime() = %v ", ClockTime(timestampNowis)).
		Add("timestampNowis.ClockTime().UnixNano() = %v ", ClockTime(timestampNowis).UnixNano()).
		Add("timestampNowis.UnixNano() = %v ", UnixNanoFromLedgerTime(timestampNowis)).
		Add("rounding: nowis.UnixNano() - timestampNowis.UnixNano() = %d", nowis.UnixNano()-UnixNanoFromLedgerTime(timestampNowis)).
		Add("tick duration nano = %d", int64(TickDuration())).
		String()
}

func GenesisTransactionIDShort() (ret TransactionIDShort) {
	ret[0] = 1
	return
}

// GenesisTransactionID independent on any ledger constants
func GenesisTransactionID() TransactionID {
	return NewTransactionID(base.LedgerTime{}, GenesisTransactionIDShort(), true)
}

// GenesisOutputID independent on ledger constants, except GenesisOutputIndex which is byte(0)
func GenesisOutputID() (ret OutputID) {
	// we are placing sequencer flag = true into the genesis tx id to please sequencer constraint
	// of the origin branch transaction. It is the only exception
	ret = MustNewOutputID(GenesisTransactionID(), GenesisOutputIndex)
	return
}

// GenesisStemOutputID independent on ledger constants, except GenesisStemOutputIndex which is byte(1)
func GenesisStemOutputID() (ret OutputID) {
	ret = MustNewOutputID(GenesisTransactionID(), GenesisStemOutputIndex)
	return
}
