package sequencer

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lunfardo314/proxima/core/attacher"
	"github.com/lunfardo314/proxima/core/txmetadata"
	"github.com/lunfardo314/proxima/core/vertex"
	"github.com/lunfardo314/proxima/core/workflow"
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/ledger/transaction"
	"github.com/lunfardo314/proxima/sequencer/backlog"
	"github.com/lunfardo314/proxima/sequencer/task"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/checkpoints"
	"github.com/lunfardo314/proxima/util/set"
	"go.uber.org/zap"
)

type (
	Environment interface {
		global.NodeGlobal
		attacher.Environment
		IsSynced() bool
		TxBytesStore() global.TxBytesStore
		GetLatestMilestone(seqID base.ChainID) *vertex.WrappedTx
		LatestMilestonesDescending(filter ...func(seqID base.ChainID, vid *vertex.WrappedTx) bool) []*vertex.WrappedTx
		LatestMilestonesShuffled(filter ...func(seqID base.ChainID, vid *vertex.WrappedTx) bool) []*vertex.WrappedTx
		NumSequencerTips() int
		ListenToAccount(account ledger.Accountable, fun func(wOut vertex.WrappedOutput))
		MustEnsureBranch(txid base.TransactionID) *vertex.WrappedTx
		OwnSequencerMilestoneIn(txBytes []byte, meta *txmetadata.TransactionMetadata)
		LatestReliableState() (multistate.SugaredStateReader, error)
		SubmitTxBytesFromInflator(txBytes []byte)
	}

	Sequencer struct {
		Environment
		ctx                context.Context    // local context
		stopFun            context.CancelFunc // local stop function
		sequencerID        base.ChainID
		controllerKey      ed25519.PrivateKey
		backlog            *backlog.TagAlongBacklog
		config             *ConfigOptions
		logName            string
		log                *zap.SugaredLogger
		ownMilestonesMutex sync.RWMutex
		ownMilestones      map[*vertex.WrappedTx]outputsWithTime // map ms -> consumed outputs in the past

		// keeping counters for each referenced vid. When counter reaches 0, vid is deleted from the map
		mutexReferenceCounters sync.Mutex

		milestoneCount  int
		branchCount     int
		lastSubmittedTs base.LedgerTime
		infoMutex       sync.RWMutex
		info            Info
		//
		onCallbackMutex      sync.RWMutex
		onMilestoneSubmitted func(seq *Sequencer, vid *vertex.WrappedTx)
		onExit               func()

		slotData *task.SlotData

		metrics *sequencerMetrics
	}

	outputsWithTime struct {
		consumed set.Set[base.OutputID]
		since    time.Time
	}

	Info struct {
		In                     int
		Out                    int
		InflationAmount        uint64
		NumConsumedFeeOutputs  int
		NumFeeOutputsInTippool int
		NumOtherMsInTippool    int
		LedgerCoverage         uint64
		PrevLedgerCoverage     uint64
	}
)

const TraceTag = "sequencer"

func New(env Environment, seqID base.ChainID, controllerKey ed25519.PrivateKey, opts ...ConfigOption) (*Sequencer, error) {
	cfg := configOptions(opts...)
	logName := fmt.Sprintf("[%s-%s]", cfg.SequencerName, seqID.StringVeryShort())
	ret := &Sequencer{
		Environment:   env,
		sequencerID:   seqID,
		controllerKey: controllerKey,
		ownMilestones: make(map[*vertex.WrappedTx]outputsWithTime),
		config:        cfg,
		logName:       logName,
		log:           env.Log().Named(logName),
	}
	if cfg.SingleSequencerEnforced {
		ret.metrics = &sequencerMetrics{}
		ret.registerMetrics()
	}

	ret.ctx, ret.stopFun = context.WithCancel(env.Ctx())
	var err error

	if ret.backlog, err = backlog.New(ret); err != nil {
		return nil, err
	}
	if err = ret.backlog.LoadSequencerStartTips(seqID); err != nil {
		return nil, err
	}
	ret.Log().Infof("sequencer is starting with config:\n%s", cfg.lines(seqID, ledger.AddressED25519FromPrivateKey(controllerKey), "     ").String())

	return ret, nil
}

func NewFromConfig(glb *workflow.Workflow) (*Sequencer, error) {
	cfg, seqID, controllerKey, err := paramsFromConfig()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	return New(glb, seqID, controllerKey, cfg...)
}

func (seq *Sequencer) Start() {
	runFun := func() {
		seq.MarkWorkProcessStarted(seq.config.SequencerName)
		defer seq.MarkWorkProcessStopped(seq.config.SequencerName)

		if !seq.ensurePreConditions() {
			return
		}

		seq.log.Infof("sequencer has been STARTED %s", util.Ref(seq.SequencerID()).String())

		ttl := time.Duration(seq.config.MilestonesTTLSlots) * ledger.L().ID.SlotDuration()

		seq.RepeatInBackground(seq.SequencerName()+"_own_milestone_cleanup", ownMilestoneCleanupPeriod, func() bool {
			if n, remain := seq.purgeOwnMilestones(ttl); n > 0 {
				seq.Log().Infof("purged %d own milestones, %d remain. TTL = %v", n, remain, ttl)
			}
			return true
		}, true)

		seq.RepeatInBackground(seq.SequencerName()+"_own_milestone_recreate_map", ownMilestoneMapRecreatePeriod, func() bool {
			seq.recreateMapOwnMilestones()
			return true
		})

		seq.sequencerLoop()

		seq.onCallbackMutex.RLock()
		defer seq.onCallbackMutex.RUnlock()

		if seq.onExit != nil {
			seq.onExit()
		}
	}

	const debuggerFriendly = false

	if debuggerFriendly {
		go runFun()
	} else {
		util.RunWrappedRoutine(seq.config.SequencerName+"[sequencerLoop]", runFun, func(err error) bool {
			seq.log.Fatal(err)
			return false
		})
	}
}

func (seq *Sequencer) ensureSyncedIfNecessary() bool {
	if !seq.config.EnsureSyncedBeforeStart {
		return true
	}
	seq.Log().Infof("ensureSyncedIfNecessary: ensure node is synced before starting sequencer...")
	seq.RepeatSync(2*time.Second, func() bool {
		seq.Log().Infof("ensureSyncedIfNecessary: waiting for node synced before starting sequencer...")
		return !seq.IsSynced()
	})
	return seq.IsSynced()
}

func (seq *Sequencer) ensureNotTooCloseToSnapshot() bool {
	snapshotSlot := seq.Branches().SnapshotSlot()
	if snapshotSlot == 0 {
		return true
	}
	slotNow := ledger.TimeNow().Slot
	if slotNow-snapshotSlot < 64 {
		seq.log.Warnf("ensureNotTooCloseToSnapshot: current slot (%d) must be at least 64 slots from the snapshot slot (%d). Can't start sequencer. EXIT..",
			slotNow, snapshotSlot)
		return false
	}
	return true
}

func (seq *Sequencer) ensurePreConditions() bool {
	if !seq.ensureSyncedIfNecessary() {
		seq.log.Warnf("ensurePreConditions: node is not synced. Can't start sequencer. EXIT..")
		return false
	}
	seq.log.Infof("ensurePreConditions: node is synced")

	if !seq.ensureNotTooCloseToSnapshot() {
		seq.log.Warnf("ensurePreConditions: Can't start sequencer. EXIT..")
		return false
	}
	snapshotID := seq.Branches().SnapshotBranchID()
	seq.log.Infof("ensurePreConditions: snapshot branch is %s", snapshotID.String())

	if !seq.ensureFirstMilestone() {
		seq.log.Warnf("ensurePreConditions: Can't start sequencer. EXIT..")
		return false
	}
	seq.log.Infof("ensurePreConditions: waiting for %v (1 slot) before starting sequencer", ledger.L().ID.SlotDuration())
	time.Sleep(ledger.L().ID.SlotDuration())
	return true
}

const ensureStartingMilestoneTimeout = 5 * time.Second

// ensureFirstMilestone waiting for the first sequencer milestone arrive
func (seq *Sequencer) ensureFirstMilestone() bool {
	var startOutput vertex.WrappedOutput

	deadline := time.Now().Add(ensureStartingMilestoneTimeout)
	succ := seq.RepeatSync(ledger.L().ID.TickDuration, func() bool {
		if time.Now().After(deadline) {
			return false
		}
		startOutput = seq.OwnLatestMilestoneOutput()
		return startOutput.VID == nil || !startOutput.IsAvailable()
	})

	if !succ {
		seq.log.Errorf("ensureFirstMilestone: interrupted")
		return false
	}
	if startOutput.VID == nil || !startOutput.IsAvailable() {
		seq.log.Errorf("failed to find chain output to start")
		return false
	}
	if !seq.checkSequencerStartOutput(startOutput) {
		return false
	}
	seq.AddOwnMilestone(startOutput.VID)

	if sleepDuration := time.Until(ledger.ClockTime(startOutput.Timestamp())); sleepDuration > 0 {
		seq.log.Warnf("will delay start for %v to sync ledger time with the clock", sleepDuration)
		seq.ClockCatchUpWithLedgerTime(startOutput.Timestamp())
	}
	return true
}

func (seq *Sequencer) checkSequencerStartOutput(wOut vertex.WrappedOutput) bool {
	util.Assertf(wOut.VID != nil, "wOut.VID != nil")
	if !wOut.VID.IsSequencerMilestone() {
		seq.log.Warnf("checkSequencerStartOutput: start output %s is not a sequencer output", wOut.IDStringShort())
	}
	oReal, err := wOut.VID.OutputAt(wOut.Index)
	if oReal == nil || err != nil {
		seq.log.Errorf("checkSequencerStartOutput: failed to load start output %s: %s", wOut.IDStringShort(), err)
		return false
	}
	lock := oReal.Lock()
	if !ledger.BelongsToAccount(lock, ledger.AddressED25519FromPrivateKey(seq.controllerKey)) {
		seq.log.Errorf("checkSequencerStartOutput: provided private key does match sequencer lock %s", lock.String())
		return false
	}
	seq.log.Infof("checkSequencerStartOutput: sequencer controller is %s", lock.String())

	amount := oReal.Amount()
	if amount < ledger.L().ID.MinimumAmountOnSequencer {
		seq.log.Errorf("checkSequencerStartOutput: amount %s on output is less than minimum %s required on sequencer",
			util.Th(amount), util.Th(ledger.L().ID.MinimumAmountOnSequencer))
		return false
	}
	seq.log.Infof("sequencer start output %s has amount %s (%s%% of the initial supply)",
		wOut.IDStringShort(), util.Th(amount), util.PercentString(int(amount), int(ledger.L().ID.InitialSupply)))
	return true
}

func (seq *Sequencer) Ctx() context.Context {
	return seq.ctx
}

func (seq *Sequencer) Stop() {
	seq.stopFun()
}

func (seq *Sequencer) Backlog() *backlog.TagAlongBacklog {
	return seq.backlog
}

func (seq *Sequencer) SequencerID() base.ChainID {
	return seq.sequencerID
}

func (seq *Sequencer) ControllerPrivateKey() ed25519.PrivateKey {
	return seq.controllerKey
}

func (seq *Sequencer) SequencerName() string {
	return seq.config.SequencerName
}

func (seq *Sequencer) Log() *zap.SugaredLogger {
	return seq.log
}

func (seq *Sequencer) sequencerLoop() {
	beginAt := time.Now().Add(seq.config.DelayStart)
	if seq.config.DelayStart > 0 {
		seq.log.Infof("wait for %v before starting the main loop", seq.config.DelayStart)
	}
	time.Sleep(time.Until(beginAt))

	seq.Log().Infof("STARTING sequencer loop")
	defer func() {
		seq.Log().Infof("sequencer loop STOPPING..")
	}()

	for {
		select {
		case <-seq.Ctx().Done():
			return
		default:
			start := time.Now()
			if !seq.doSequencerStep() {
				return
			}
			duration := time.Since(start)
			if duration > 3*time.Second {
				seq.Log().Warnf(">>>>>>>>>>>>> sequencer step took %v", duration)
			}
		}
	}
}

const TraceTagTarget = "target"

func (seq *Sequencer) doSequencerStep() bool {
	seq.Tracef(TraceTag, "doSequencerStep")
	if seq.config.MaxBranches != 0 && seq.branchCount >= seq.config.MaxBranches {
		seq.log.Infof("reached max limit of branch milestones %d -> stopping", seq.config.MaxBranches)
		return false
	}

	timerStart := time.Now()
	targetTs := seq.getNextTargetTime()
	seq.newTargetSet()

	if seq.slotData == nil {
		seq.slotData = task.NewSlotData(targetTs.Slot)
	}

	seq.Assertf(ledger.ValidSequencerPace(seq.lastSubmittedTs, targetTs), "target is closer than allowed pace (%d): %s -> %s",
		ledger.TransactionPaceSequencer(), seq.lastSubmittedTs.String, targetTs.String)

	seq.Assertf(targetTs.After(seq.lastSubmittedTs), "wrong target ts %s: should be after previous submitted %s",
		targetTs.String, seq.lastSubmittedTs.String)

	if seq.config.MaxTargetTs != base.NilLedgerTime && targetTs.After(seq.config.MaxTargetTs) {
		seq.log.Infof("next target ts %s is after maximum ts %s -> stopping", targetTs, seq.config.MaxTargetTs)
		return false
	}

	seq.Tracef(TraceTagTarget, "target ts: %s. Now is: %s", targetTs, ledger.TimeNow())

	msTx, meta, err := seq.generateMilestoneForTarget(targetTs)

	switch {
	case errors.Is(err, task.ErrNotGoodEnough):
		seq.slotData.NotGoodEnough()
		seq.Tracef(TraceTagTarget, "'not good enough' for the target logical time %s in %v",
			targetTs, time.Since(timerStart))
		return true
	case errors.Is(err, task.ErrNoProposals):
		seq.slotData.NoProposals()
		seq.Tracef(TraceTagTarget, "'no proposals' for the target logical time %s in %v",
			targetTs, time.Since(timerStart))
		return true
	case err != nil:
		seq.Log().Warnf("FAILED to generate transaction for target %s. Now is %s. Reason: %v",
			targetTs, ledger.TimeNow(), err)
		return true
	}
	util.Assertf(msTx != nil, "msTx != nil")

	seq.Tracef(TraceTag, "produced milestone %s for the target logical time %s in %v. Meta: %s",
		msTx.IDShortString, targetTs, time.Since(timerStart), meta.String)

	saveLastSubmittedTs := seq.lastSubmittedTs

	meta.TxBytesReceived = util.Ref(time.Now())
	msVID := seq.submitMilestone(msTx, meta)
	if msVID != nil {
		if saveLastSubmittedTs.IsSlotBoundary() && msVID.Timestamp().IsSlotBoundary() {
			seq.Log().Warnf("branch jumped over the slot: %s -> %s. Step started: %s, %d (%s), %v ago, nowis: %s",
				saveLastSubmittedTs.String(), targetTs.String(),
				timerStart.Format(time.StampNano), timerStart.UnixNano(), ledger.TimeFromClockTime(timerStart).String(), time.Since(timerStart),
				ledger.TimeNow().String())
		}

		seq.AddOwnMilestone(msVID)
		seq.milestoneCount++
		if msVID.IsBranchTransaction() {
			seq.branchCount++
			seq.slotData.BranchTxSubmitted(msVID.ID())
		} else {
			seq.slotData.SequencerTxSubmitted(msVID.ID())
		}
		seq.updateInfo(msVID)
		seq.runOnMilestoneSubmitted(msVID)
		seq.onMilestoneSubmittedMetrics(msVID)
	}

	if targetTs.IsSlotBoundary() {
		seq.Log().Infof("SLOT STATS: %s", seq.slotData.Lines().Join(", "))
		seq.slotData = nil
	}
	return true
}

func (seq *Sequencer) getNextTargetTime() base.LedgerTime {
	// wait to catch up with ledger time
	seq.ClockCatchUpWithLedgerTime(seq.lastSubmittedTs)

	nowis := ledger.TimeNow()

	if base.DiffTicks(nowis.NextSlotBoundary(), nowis) < int64(ledger.L().ID.PreBranchConsolidationTicks) {
		return nowis.NextSlotBoundary()
	}

	var targetAbsoluteMinimum base.LedgerTime

	if seq.lastSubmittedTs.IsSlotBoundary() {
		targetAbsoluteMinimum = seq.lastSubmittedTs.AddTicks(int(ledger.L().ID.PostBranchConsolidationTicks))
	} else {
		targetAbsoluteMinimum = base.MaximumTime(
			seq.lastSubmittedTs.AddTicks(seq.config.Pace),
			nowis.AddTicks(1),
		)
	}
	if uint8(targetAbsoluteMinimum.Tick) < ledger.L().ID.PostBranchConsolidationTicks {
		targetAbsoluteMinimum = base.NewLedgerTime(targetAbsoluteMinimum.Slot, base.Tick(ledger.L().ID.PostBranchConsolidationTicks))
	}
	nextSlotBoundary := nowis.NextSlotBoundary()

	if !targetAbsoluteMinimum.Before(nextSlotBoundary) {
		return targetAbsoluteMinimum
	}
	// absolute minimum is before the next slot boundary, take the time now as a baseline
	minimumTicksAheadFromNow := (seq.config.Pace * 2) / 3 // seq.config.Pace
	targetAbsoluteMinimum = base.MaximumTime(targetAbsoluteMinimum, nowis.AddTicks(minimumTicksAheadFromNow))
	if !targetAbsoluteMinimum.Before(nextSlotBoundary) {
		return targetAbsoluteMinimum
	}

	if targetAbsoluteMinimum.TicksToNextSlotBoundary() <= seq.config.Pace {
		return base.MaximumTime(nextSlotBoundary, targetAbsoluteMinimum)
	}

	return targetAbsoluteMinimum
}

const disconnectTolerance = 4 * time.Second

// decideSubmitMilestone branch transactions are issued only if healthy, or bootstrap mode enabled
func (seq *Sequencer) decideSubmitMilestone(tx *transaction.Transaction, meta *txmetadata.TransactionMetadata) bool {
	if seq.DurationSinceLastMessageFromPeer() >= disconnectTolerance {
		seq.Log().Infof("WON'T SUBMIT BRANCH %s: node is disconnected for %v", seq.DurationSinceLastMessageFromPeer())
		return false
	}
	if tx.IsBranchTransaction() {
		healthy := global.IsHealthyCoverageDelta(*meta.CoverageDelta, *meta.Supply, global.FractionHealthyBranch)
		//bootstrapMode := seq.IsBootstrapMode()
		//if healthy || bootstrapMode {
		if healthy {
			seq.Log().Infof("SUBMIT BRANCH %s. Now: %s, proposer: %s, coverage: %s, inflation: %s",
				tx.IDShortString(), ledger.TimeNow().String(), tx.SequencerTransactionData().SequencerOutputData.MilestoneData.Name,
				util.Th(*meta.LedgerCoverage), util.Th(tx.InflationAmount()))
			return true
		}
		seq.Log().Infof("WON'T SUBMIT BRANCH %s. Now: %s, p: %s, cov.delta: %s/%s, supply: %s, infl: %s, slot infl: %s",
			tx.IDShortString(), ledger.TimeNow().String(), tx.SequencerTransactionData().SequencerOutputData.MilestoneData.Name,
			util.Th(*meta.LedgerCoverage), util.Th(*meta.CoverageDelta), util.Th(*meta.Supply), util.Th(tx.InflationAmount()), util.Th(*meta.SlotInflation))
		return false
	}

	seq.Log().Infof("SUBMIT SEQ TX %s. Now: %s, proposer: %s, coverage: %s, inflation: %s",
		tx.IDShortString(), ledger.TimeNow().String(), tx.SequencerTransactionData().SequencerOutputData.MilestoneData.Name,
		util.Th(*meta.LedgerCoverage), util.Th(tx.InflationAmount()))
	return true
}

func (seq *Sequencer) submitMilestone(tx *transaction.Transaction, meta *txmetadata.TransactionMetadata) *vertex.WrappedTx {
	if !seq.decideSubmitMilestone(tx, meta) {
		return nil
	}

	//if tx.Timestamp() == base.NewLedgerTime(8, 12) {
	//	seq.StartTracingTags(
	//		attacher.TraceTagAttachMilestone,
	//		attacher.TraceTagAttachVertex,
	//		attacher.TraceTagValidateSequencer,
	//	)
	//}
	const submitTimeout = 2 * time.Second
	{
		nm := "submit_" + tx.IDShortString()
		check := checkpoints.New(func(name string) {
			seq.Log().Fatalf("STUCK: submitMilestone @ %s", name)
		})
		check.Check(nm, submitTimeout)
		defer check.CheckAndClose(nm)
	}

	seq.OwnSequencerMilestoneIn(tx.Bytes(), meta)

	seq.Tracef(TraceTag, "new milestone %s submitted successfully", tx.IDShortString)

	vid, err := seq.waitMilestoneInTippool(tx.ID(), time.Now().Add(submitTimeout))

	if err != nil {
		seq.Log().Error(err)
		return nil
	}
	seq.lastSubmittedTs = vid.Timestamp()
	return vid
}

func (seq *Sequencer) waitMilestoneInTippool(txid base.TransactionID, deadline time.Time) (*vertex.WrappedTx, error) {
	for {
		select {
		case <-seq.Ctx().Done():
			return nil, fmt.Errorf("waitMilestoneInTippool: %s has been cancelled", txid.StringShort())
		case <-time.After(10 * time.Millisecond):
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("waitMilestoneInTippool: deadline has been missed while waiting for %s in the tippool", txid.StringShort())
			}
		default:
			vid := seq.GetLatestMilestone(seq.sequencerID)
			if vid != nil && vid.ID() == txid {
				return vid, nil
			}
		}
	}
}

func (seq *Sequencer) OnMilestoneSubmitted(fun func(seq *Sequencer, ms *vertex.WrappedTx)) {
	seq.onCallbackMutex.Lock()
	defer seq.onCallbackMutex.Unlock()

	if seq.onMilestoneSubmitted == nil {
		seq.onMilestoneSubmitted = fun
	} else {
		prevFun := seq.onMilestoneSubmitted
		seq.onMilestoneSubmitted = func(seq *Sequencer, ms *vertex.WrappedTx) {
			prevFun(seq, ms)
			fun(seq, ms)
		}
	}
}

func (seq *Sequencer) OnExitOnce(fun func()) {
	seq.onCallbackMutex.Lock()
	defer seq.onCallbackMutex.Unlock()

	if seq.onExit == nil {
		seq.onExit = fun
	} else {
		prevFun := seq.onExit
		seq.onExit = func() {
			prevFun()
			fun()

			seq.onCallbackMutex.Lock()
			defer seq.onCallbackMutex.Unlock()
			seq.onExit = prevFun
		}
	}
}

func (seq *Sequencer) runOnMilestoneSubmitted(ms *vertex.WrappedTx) {
	seq.onCallbackMutex.RLock()
	defer seq.onCallbackMutex.RUnlock()

	if seq.onMilestoneSubmitted != nil {
		seq.onMilestoneSubmitted(seq, ms)
	}
}

func (seq *Sequencer) MaxInputs() (int, int) {
	return seq.config.MaxInputs, seq.config.MaxTagAlongInputs
}

func (seq *Sequencer) BacklogTTLSlots() (int, int) {
	return seq.config.BacklogTagAlongTTLSlots, seq.config.BacklogDelegationTTLSlots
}

// bootstrapOwnMilestoneOutput find own milestone output in one of the latest milestones, or, alternatively in the LRB
func (seq *Sequencer) bootstrapOwnMilestoneOutput() vertex.WrappedOutput {
	milestones := seq.LatestMilestonesDescending()
	for _, ms := range milestones {
		baselineBranchID, ok := ms.BaselineBranch()
		if !ok {
			continue
		}
		rdr := multistate.MakeSugared(seq.Branches().GetStateReaderForTheBranch(baselineBranchID))
		chainOut, _, err := rdr.GetChainTips(seq.sequencerID)
		if errors.Is(err, multistate.ErrNotFound) {
			continue
		}
		seq.AssertNoError(err)

		return attacher.AttachOutputWithID(*chainOut, seq, attacher.WithInvokedBy("tippool 1"))
	}
	// didn't find in latest milestones in the tippool, try LRB
	branchData := seq.Branches().FindLatestReliableBranch(global.FractionHealthyBranch)
	if branchData == nil {
		seq.Log().Warnf("bootstrapOwnMilestoneOutput: can't find LRB")
		return vertex.WrappedOutput{}
	}
	rdr := multistate.MakeSugared(seq.Branches().GetStateReaderForTheBranch(branchData.TxID()))
	chainOut, err := rdr.GetChainOutput(seq.SequencerID())
	if err != nil {
		seq.Log().Warnf("bootstrapOwnMilestoneOutput: can't load own milestone output from LRB")
		return vertex.WrappedOutput{}
	}
	return attacher.AttachOutputWithID(*chainOut, seq, attacher.WithInvokedBy("tippool 2"))
}

func (seq *Sequencer) generateMilestoneForTarget(targetTs base.LedgerTime) (*transaction.Transaction, *txmetadata.TransactionMetadata, error) {
	deadline := ledger.ClockTime(targetTs)
	nowis := time.Now()
	seq.Tracef(TraceTag, "generateMilestoneForTarget: target: %s, deadline: %s, nowis: %s",
		targetTs.String, deadline.Format("15:04:05.999"), nowis.Format("15:04:05.999"))

	if behind := deadline.Sub(nowis); behind < -2*ledger.L().ID.TickDuration {
		return nil, nil, fmt.Errorf("sequencer: target %s (%v) is before current clock by %v: too late to generate milestone",
			targetTs.String(), ledger.ClockTime(targetTs).Format("15:04:05.999"), behind)
	}
	return task.Run(seq, targetTs, seq.slotData)
}

func (seq *Sequencer) NumOutputsInBuffer() int {
	return seq.Backlog().NumOutputsInBuffer()
}

func (seq *Sequencer) NumMilestones() int {
	return seq.NumSequencerTips()
}
