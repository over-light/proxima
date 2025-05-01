package attacher

import (
	"fmt"
	"time"

	"github.com/lunfardo314/proxima/core/txmetadata"
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/util"
)

func (a *milestoneAttacher) wrapUpAttacher() {
	a.Tracef(TraceTagAttachMilestone, "wrapUpAttacher")

	a.slotInflation = a.pastCone.CalculateSlotInflation()

	a.finals.baseline = a.baselineBranchID.ID()
	a.finals.numVertices = a.pastCone.NumVertices()

	a.finals.ledgerCoverage, a.finals.coverageDelta = a.CoverageAndDelta()
	a.finals.slotInflation = a.slotInflation

	a.Tracef(TraceTagAttachMilestone, "set ledger coverage in %s to %s",
		a.vid.IDShortString, func() string { return util.Th(a.finals.ledgerCoverage) })

	if a.vid.IsBranchTransaction() {
		a.commitBranch()
		a.Tracef(TraceTagAttachMilestone, "finalized branch")
	} else {
		a.Tracef(TraceTagAttachMilestone, "finalized sequencer milestone")
	}

	a.checkConsistencyWithMetadata()

	calculatedMetadata := txmetadata.TransactionMetadata{
		CoverageDelta:  util.Ref(a.finals.coverageDelta),
		LedgerCoverage: util.Ref(a.finals.ledgerCoverage),
		SlotInflation:  util.Ref(a.finals.slotInflation),
	}
	if a.metadata != nil {
		calculatedMetadata.SourceTypeNonPersistent = a.metadata.SourceTypeNonPersistent
	}
	if a.vid.IsBranchTransaction() {
		calculatedMetadata.StateRoot = a.finals.root
		calculatedMetadata.Supply = util.Ref(a.baselineSupply + a.slotInflation)
	}
	a.Tracef(TraceTagAttachMilestone, "%s: calculated metadata: %s", a.name, calculatedMetadata.String)
}

func (a *milestoneAttacher) commitBranch() {
	a.Assertf(a.vid.IsBranchTransaction(), "a.vid.IsBranchTransaction()")

	muts, stats := a.pastCone.Mutations(a.vid.Slot())

	a.finals.numNewTransactions, a.finals.numDeletedOutputs, a.finals.numCreatedOutputs = uint32(stats.NumTransactions), stats.NumDeleted, stats.NumCreated

	seqID, stemOID := a.vid.MustSequencerIDAndStemID()
	upd := multistate.MustNewUpdatable(a.StateStore(), a.BaselineSugaredStateReader().Root())
	a.finals.supply = a.baselineSupply + a.finals.slotInflation
	ledgerCoverage, coverageDelta := a.CoverageAndDelta()

	util.Assertf(a.slotInflation == a.finals.slotInflation, "a.slotInflation == a.finals.slotInflation")
	supply := a.FinalSupply()

	err := upd.Update(muts, &multistate.RootRecordParams{
		StemOutputID:    stemOID,
		SeqID:           seqID,
		CoverageDelta:   coverageDelta,
		LedgerCoverage:  ledgerCoverage,
		SlotInflation:   a.slotInflation,
		Supply:          supply,
		NumTransactions: a.finals.numNewTransactions,
	})
	if err != nil {
		err = fmt.Errorf("%w:\n-------- past cone of %s --------\n%s", err, a.Name(), a.pastCone.Lines("     ").Join("\n"))
		a.pastCone.SaveGraph(util.Ref(a.vid.ID()).AsFileNameShort())
		a.SaveFullDAG("full_dag_failed_upd")
		time.Sleep(2 * time.Second)
	}
	a.AssertNoError(err)

	a.finals.root = upd.Root()

	a.EvidenceBranchSlot(a.vid.Slot(), global.IsHealthyCoverageDelta(coverageDelta, supply, global.FractionHealthyBranch))
}
