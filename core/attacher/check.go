package attacher

import (
	"fmt"

	"github.com/lunfardo314/proxima/core/memdag"
	"github.com/lunfardo314/proxima/core/txmetadata"
	"github.com/lunfardo314/proxima/core/vertex"
	"github.com/lunfardo314/proxima/util"
)

func (a *milestoneAttacher) checkConsistencyBeforeWrapUp() (err error) {
	if a.vid.GetTxStatus() == vertex.Bad {
		return fmt.Errorf("checkConsistencyBeforeWrapUp: vertex %s is BAD", a.vid.IDShortString())
	}
	if a.SnapshotBranchID().Timestamp().AfterOrEqual(a.vid.Timestamp()) {
		// attacher is before the snapshot -> no need to check inputs, it must be in the state anyway
		return nil
	}
	a.vid.Unwrap(vertex.UnwrapOptions{Vertex: func(v *vertex.Vertex) {
		if err = a._checkMonotonicityOfInputTransactions(v); err != nil {
			return
		}
		err = a._checkMonotonicityOfEndorsements(v)
	}})
	if err != nil {
		err = fmt.Errorf("checkConsistencyBeforeWrapUp in attacher %s: %v\n---- attacher lines ----\n%s", a.name, err, a.dumpLinesString("       "))
		memdag.SavePastConeFromTxStore(a.vid.ID(), a.TxBytesStore(), a.vid.Slot()-3, "inconsist_"+util.Ref(a.vid.ID()).AsFileNameShort()+".gv")
	}
	return err
}

func (a *milestoneAttacher) _checkMonotonicityOfEndorsements(v *vertex.Vertex) (err error) {
	v.ForEachEndorsement(func(i byte, vidEndorsed *vertex.WrappedTx) bool {
		if vidEndorsed.IsBranchTransaction() {
			return true
		}
		lc := vidEndorsed.GetLedgerCoverageP()
		if lc == nil {
			err = fmt.Errorf("ledger coverage not set in the endorsed %s", vidEndorsed.IDShortString())
			return false
		}
		lcCalc := a.LedgerCoverage()
		if lcCalc < *lc {
			/* FIXME sometimes happens
			----------------------------------------------------------------------------- loc1
			02-25 22:42:40.617      FATAL   error: checkConsistencyBeforeWrapUp in attacher [206230|73sq]007e4f63cc95..: ledger coverage should not decrease along endorsement.
			Got: delta(1_977_251_601_888_347) at 206230|73 <= delta(1_977_849_204_153_775) in [206230|69sq]00920881f1ab... diff: 597_602_265_428
			---- attacher lines ----
			       attacher [206230|73sq]007e4f63cc95..
			          baseline: [206230|0br]01e9605b38f7..
			          baselineSupply: 1_007_011_094_113_571
			          Past cone:
			       ------ past cone: '[206230|73sq]007e4f63cc95..'
			       ------ baseline: [206230|0br]01e9605b38f7.., coverage: 1_979_173_849_528_818
			       #0 S+ [206227|106sq]013ad38ec377.. consumers: {1: {[206230|25sq]019c5d575476..}} flags: 00001111 known: true, defined: true, inTheState: (true,true), endorsementsOk: false, inputsOk: false, poke: false
			       #1 S+ [206227|111sq]01ebc2af0fe0.. consumers: {1: {[206230|31sq]017e9519dee2..}} flags: 00001111 known: true, defined: true, inTheState: (true,true), endorsementsOk: false, inputsOk: false, poke: false
			       #2 S+ [206227|125sq]01886f60405e.. consumers: {1: {[206230|25sq]01317bf0c752..}} flags: 00001111 known: true, defined: true, inTheState: (true,true), endorsementsOk: false, inputsOk: false, poke: false
			       #3 S+ [206229|25sq]011b07d81828.. consumers: {0: {[206230|57sq]013df14d4d51..}, 1: {[206230|57sq]013df14d4d51..}} flags: 00111111 known: true, defined: true, inTheState: (true,true), endorsementsOk: true, inputsOk: true, poke: false
			       #4 S+ [206229|25sq]019c0ac682d6.. consumers: {0: {[206229|113sq]00260ff39785..}, 1: {[206230|57sq]01ef4290c742..}} flags: 00111111 known: true, defined: true, inTheState: (true,true), endorsementsOk: true, inputsOk: true, poke: false
			       #5 S+ [206229|40sq]0125b117536c.. consumers: {0: {[206229|65sq]0145d0937e72..}, 1: {[206230|43sq]013550c6afbd..}} flags: 00001111 known: true, defined: true, inTheState: (true,true), endorsementsOk: false, inputsOk: false, poke: false
			       #6 S+ [206229|63sq]018e2dee6ec0.. consumers: {0: {[206230|31sq]017e9519dee2..}} flags: 00111111 known: true, defined: true, inTheState: (true,true), endorsementsOk: true, inputsOk: true, poke: false
			       #7 S+ [206229|65sq]0145d0937e72.. consumers: {} flags: 00111111 known: true, defined: true, inTheState: (true,true), endorsementsOk: true, inputsOk: true, poke: false
			       #8 S+ [206229|81sq]020cf09c0d96.. consumers: {0: {[206230|25sq]01317bf0c752..}} flags: 00111111 known: true, defined: true, inTheState: (true,true), endorsementsOk: true, inputsOk: true, poke: false
			       #9 S+ [206229|88sq]008fceec5bb1.. consumers: {0: {[206230|25sq]019c5d575476..}} flags: 00111111 known: true, defined: true, inTheState: (true,true), endorsementsOk: true, inputsOk: true, poke: false
			       #10 S+ [206229|103sq]037a0c959f5d.. consumers: {0: {[206230|37sq]00ef6d54f185..}} flags: 00111111 known: true, defined: true, inTheState: (true,true), endorsementsOk: true, inputsOk: true, poke: false
			       #11 S- [206229|113sq]00260ff39785.. consumers: {0: {[206230|57sq]01ef4290c742..}} flags: 00110111 known: true, defined: true, inTheState: (true,false), endorsementsOk: true, inputsOk: true, poke: false
			       #12 S+ [206229|128sq]0147c88838fc.. consumers: {0: {[206230|49sq]0012b7c25fa9..}} flags: 00111111 known: true, defined: true, inTheState: (true,true), endorsementsOk: true, inputsOk: true, poke: false
			       #13 S- [206229|134sq]00c81b28d5ce.. consumers: {0: {[206230|25sq]00064c3e5e84..}} flags: 00110111 known: true, defined: true, inTheState: (true,false), endorsementsOk: true, inputsOk: true, poke: false
			       #14 S+ [206230|0br]01e9605b38f7.. consumers: {0: {[206230|25sq]005e8d308d0b..}} flags: 00001111 known: true, defined: true, inTheState: (true,true), endorsementsOk: false, inputsOk: false, poke: false
			       #15 S- [206230|25sq]019c5d575476.. consumers: {0: {[206230|69sq]00920881f1ab..}} flags: 00110111 known: true, defined: true, inTheState: (true,false), endorsementsOk: true, inputsOk: true, poke: false
			       #16 S- [206230|25sq]01317bf0c752.. consumers: {} flags: 00110111 known: true, defined: true, inTheState: (true,false), endorsementsOk: true, inputsOk: true, poke: false
			       #17 S- [206230|25sq]005e8d308d0b.. consumers: {0: {[206230|43sq]013550c6afbd..}} flags: 00110111 known: true, defined: true, inTheState: (true,false), endorsementsOk: true, inputsOk: true, poke: false
			       #18 S- [206230|25sq]00064c3e5e84.. consumers: {0: {[206230|61sq]0038f4594774..}} flags: 00110111 known: true, defined: true, inTheState: (true,false), endorsementsOk: true, inputsOk: true, poke: false
			       #19 S- [206230|31sq]017e9519dee2.. consumers: {} flags: 00110111 known: true, defined: true, inTheState: (true,false), endorsementsOk: true, inputsOk: true, poke: false
			       #20 S- [206230|37sq]00ef6d54f185.. consumers: {} flags: 00110111 known: true, defined: true, inTheState: (true,false), endorsementsOk: true, inputsOk: true, poke: false
			       #21 S- [206230|43sq]013550c6afbd.. consumers: {0: {[206230|73sq]007e4f63cc95..}} flags: 00110111 known: true, defined: true, inTheState: (true,false), endorsementsOk: true, inputsOk: true, poke: false
			       #22 S- [206230|49sq]0012b7c25fa9.. consumers: {} flags: 00110111 known: true, defined: true, inTheState: (true,false), endorsementsOk: true, inputsOk: true, poke: false
			       #23 S- [206230|57sq]01ef4290c742.. consumers: {} flags: 00110111 known: true, defined: true, inTheState: (true,false), endorsementsOk: true, inputsOk: true, poke: false
			       #24 S- [206230|57sq]013df14d4d51.. consumers: {} flags: 00110111 known: true, defined: true, inTheState: (true,false), endorsementsOk: true, inputsOk: true, poke: false
			       #25 S- [206230|61sq]0038f4594774.. consumers: {} flags: 00110111 known: true, defined: true, inTheState: (true,false), endorsementsOk: true, inputsOk: true, poke: false
			       #26 S- [206230|69sq]00920881f1ab.. consumers: {} flags: 00110111 known: true, defined: true, inTheState: (true,false), endorsementsOk: true, inputsOk: true, poke: false
			       #27 S- [206230|73sq]007e4f63cc95.. consumers: {} flags: 00110101 known: true, defined: false, inTheState: (true,false), endorsementsOk: true, inputsOk: true, poke: false
			       ledger coverage: 1_977_251_601_888_347, delta: 987_664_677_123_938
			github.com/lunfardo314/proxima/global.(*Global).AssertNoError
			        /home/lunfardo/go/src/github.com/proxima/global/global.go:260
			github.com/lunfardo314/proxima/core/attacher.(*milestoneAttacher).run
			        /home/lunfardo/go/src/github.com/proxima/core/attacher/attacher_milestone.go:132
			github.com/lunfardo314/proxima/core/attacher.runMilestoneAttacher
			        /home/lunfardo/go/src/github.com/proxima/core/attacher/attacher_milestone.go:47
			github.com/lunfardo314/proxima/core/attacher.AttachTransaction.func1.2
			        /home/lunfardo/go/src/github.com/proxima/core/attacher/attach.go:130
			*/
			diff := *lc - lcCalc
			err = fmt.Errorf("ledger coverage should not decrease along endorsement.\nGot: delta(%s) at %s <= delta(%s) in %s. diff: %s",
				util.Th(lcCalc), a.vid.Timestamp().String(), util.Th(*lc), vidEndorsed.IDShortString(), util.Th(diff))
			return false
		}
		return true
	})
	return
}

func (a *milestoneAttacher) _checkMonotonicityOfInputTransactions(v *vertex.Vertex) (err error) {
	setOfInputTransactions := v.SetOfInputTransactions()
	util.Assertf(len(setOfInputTransactions) > 0, "len(setOfInputTransactions)>0")

	setOfInputTransactions.ForEach(func(vidInp *vertex.WrappedTx) bool {
		if !vidInp.IsSequencerMilestone() || vidInp.IsBranchTransaction() || v.Tx.Slot() != vidInp.Slot() {
			// checking sequencer, non-branch inputs on the same slot
			return true
		}
		lc := vidInp.GetLedgerCoverageP()
		if lc == nil {
			err = fmt.Errorf("ledger coverage not set in the input tx %s", vidInp.IDShortString())
			return false
		}
		lcCalc := a.LedgerCoverage()
		if lcCalc < *lc {
			diff := *lc - lcCalc
			err = fmt.Errorf("ledger coverage should not decrease along consumed transactions on the same slot.\nGot: delta(%s) at %s <= delta(%s) in %s. diff: %s",
				util.Th(lcCalc), a.vid.Timestamp().String(), util.Th(*lc), vidInp.IDShortString(), util.Th(diff))
			return false
		}
		return true
	})
	return
}

func (a *milestoneAttacher) calculatedMetadata() *txmetadata.TransactionMetadata {
	return &txmetadata.TransactionMetadata{
		StateRoot:      a.finals.root,
		LedgerCoverage: util.Ref(a.LedgerCoverage()),
		SlotInflation:  util.Ref(a.slotInflation),
		Supply:         util.Ref(a.baselineSupply + a.slotInflation),
	}
}

// checkConsistencyWithMetadata check but not enforces
func (a *milestoneAttacher) checkConsistencyWithMetadata() {
	calcMeta := a.calculatedMetadata()
	if !a.metadata.IsConsistentWith(calcMeta) {
		a.Log().Errorf("inconsistency in metadata of %s (source seq: %s, '%s'):\n   calculated metadata: %s\n   provided metadata: %s",
			a.vid.IDShortString(), a.vid.SequencerID.Load().StringShort(), a.vid.SequencerName(), calcMeta.String(), a.metadata.String())
	}
}
