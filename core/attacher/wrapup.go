package attacher

import (
	"fmt"
	"time"

	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/util"
)

func (a *milestoneAttacher) wrapUpAttacher() {
	a.Tracef(TraceTagAttachMilestone, "wrapUpAttacher")

	a.slotInflation = a.pastCone.CalculateSlotInflation()

	a.finals.baseline = *a.BaselineBranch()
	a.finals.numVertices = a.pastCone.NumVertices()
	a.finals.TransactionMetadata.LedgerCoverage = util.Ref(a.FinalLedgerCoverage(a.vid.Timestamp()))
	a.finals.TransactionMetadata.CoverageDelta = util.Ref(a.CoverageDelta())
	a.finals.TransactionMetadata.SlotInflation = util.Ref(a.slotInflation)
	if a.providedMetadata != nil {
		a.finals.TransactionMetadata.SourceTypeNonPersistent = a.providedMetadata.SourceTypeNonPersistent
	}
	if a.vid.IsBranchTransaction() {
		a.commitBranch()
	}
	a.checkConsistencyWithMetadata()
}

func (a *milestoneAttacher) commitBranch() {
	a.Assertf(a.vid.IsBranchTransaction(), "a.vid.IsBranchTransaction()")

	muts, stats := a.pastCone.Mutations(a.vid.Slot())

	a.finals.MutationStats = stats

	seqID, stemOID := a.vid.MustSequencerIDAndStemID()
	upd := multistate.MustNewUpdatable(a.StateStore(), a.BaselineSugaredStateReader().Root())
	a.finals.TransactionMetadata.Supply = util.Ref(a.baselineSupply() + a.slotInflation)
	coverageDelta := a.CoverageDelta()

	util.Assertf(a.slotInflation == *a.finals.TransactionMetadata.SlotInflation, "a.slotInflation == *a.finals.TransactionMetadata.SlotInflation")
	supply := a.FinalSupply()

	err := upd.Update(muts, &multistate.RootRecordParams{
		StemOutputID:    stemOID,
		SeqID:           seqID,
		CoverageDelta:   coverageDelta,
		SlotInflation:   a.slotInflation,
		Supply:          supply,
		NumTransactions: uint32(a.finals.MutationStats.NumTransactions),
	})
	if err != nil {
		err = fmt.Errorf("%w:\n-------- past cone of %s --------\n%s", err, a.Name(), a.pastCone.Lines("     ").Join("\n"))
		a.pastCone.SaveGraph(util.Ref(a.vid.ID()).AsFileNameShort())
		a.SaveFullDAG("full_dag_failed_upd")
		time.Sleep(2 * time.Second)
	}
	a.AssertNoError(err)

	a.finals.TransactionMetadata.StateRoot = upd.Root()

	a.EvidenceBranchSlot(a.vid.Slot(), global.IsHealthyCoverageDelta(coverageDelta, supply, global.FractionHealthyBranch))
}
