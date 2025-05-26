package attacher

import (
	"fmt"

	"github.com/lunfardo314/proxima/core/txmetadata"
	"github.com/lunfardo314/proxima/core/vertex"
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/unitrie/common"
)

func (a *milestoneAttacher) wrapUpAttacher() {
	a.Tracef(TraceTagAttachMilestone, "wrapUpAttacher")

	a.finals.baseline = *a.pastCone.GetBaseline()
	a.finals.numVertices = a.pastCone.NumVertices()

	delta := a.CoverageDelta()
	slotInflation := a.SlotInflation()
	a.finals.TransactionMetadata = txmetadata.TransactionMetadata{
		CoverageDelta:  util.Ref(delta),
		LedgerCoverage: util.Ref(a.FinalLedgerCoverage(a.vid.Timestamp(), delta)),
		SlotInflation:  util.Ref(slotInflation),
		Supply:         util.Ref(a.BaselineSupply() + slotInflation),
	}
	if a.vid.IsBranchTransaction() {
		root, stats := a.commitBranch()
		a.finals.StateRoot = root
		a.finals.MutationStats = stats
	}
	a.checkConsistencyWithMetadata()
}

func (a *milestoneAttacher) commitBranch() (common.VCommitment, vertex.MutationStats) {
	a.Assertf(a.vid.IsBranchTransaction(), "a.vid.IsBranchTransaction()")

	muts, stats := a.pastCone.Mutations(a.vid.Slot())
	seqID, stemOID := a.vid.MustSequencerIDAndStemID()
	upd := multistate.MustNewUpdatable(a.StateStore(), a.BaselineSugaredStateReader().Root())

	err := upd.Update(muts, &multistate.RootRecordParams{
		StemOutputID:    stemOID,
		SeqID:           seqID,
		CoverageDelta:   *a.finals.CoverageDelta,
		SlotInflation:   *a.finals.SlotInflation,
		Supply:          *a.finals.Supply,
		NumTransactions: uint32(a.finals.MutationStats.NumTransactions),
	})
	if err != nil {
		err = fmt.Errorf("attacher wrapup (%s) -> %w:\n------ tx\n%s\n-------- past cone --------\n%s",
			a.Name(), err, a.vid.TxLines("    ").String(), a.pastCone.Lines("     ").Join("\n"))
	}
	a.AssertNoError(err)
	a.EvidenceBranchSlot(a.vid.Slot(), global.IsHealthyCoverageDelta(*a.finals.CoverageDelta, *a.finals.Supply, global.FractionHealthyBranch))
	return upd.Root(), stats
}
