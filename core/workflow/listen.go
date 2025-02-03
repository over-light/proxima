package workflow

import (
	"github.com/lunfardo314/proxima/core/vertex"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/transaction"
)

//const TraceTag = "listenAccount"

// ListenToAccount listens to all outputs which belongs to the account (except stem-locked outputs)
func (w *Workflow) ListenToAccount(account ledger.Accountable, fun func(wOut vertex.WrappedOutput)) {
	w.events.OnEvent(EventNewTx, func(vid *vertex.WrappedTx) {
		var _indices [256]byte
		indices := _indices[:0]
		vid.RUnwrap(vertex.UnwrapOptions{Vertex: func(v *vertex.Vertex) {
			v.Tx.ForEachProducedOutput(func(idx byte, o *ledger.Output, oid *ledger.OutputID) bool {
				//w.Tracef(TraceTag, "output %s belongs to account %s = %v\n%s",
				//	oid.StringShort(), account.String(), ledger.BelongsToAccount(o.Lock(), account), o.Lines("           ").String())

				if ledger.BelongsToAccount(o.Lock(), account) && o.Lock().Name() != ledger.StemLockName {
					indices = append(indices, idx)
				}
				return true
			})
		}})
		for _, idx := range indices {
			fun(vertex.WrappedOutput{
				VID:   vid,
				Index: idx,
			})
		}
	})
}

func (w *Workflow) ListenToTransactions(fun func(tx *transaction.Transaction)) {
	w.events.OnEvent(EventNewGoodTx, func(vid *vertex.WrappedTx) {
		vid.RUnwrap(vertex.UnwrapOptions{Vertex: func(v *vertex.Vertex) {
			fun(v.Tx)
		}})
	})
}
