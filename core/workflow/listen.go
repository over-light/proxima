package workflow

import (
	"sync"

	"github.com/lunfardo314/proxima/core/vertex"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/transaction"
)

// ListenToAccount listens to all outputs which belongs to the account (except stem-locked outputs)
func (w *Workflow) ListenToAccount(account ledger.Accountable, fun func(wOut vertex.WrappedOutput)) {
	w.events.OnEvent(EventNewTx, func(vid *vertex.WrappedTx) {
		var _indices [256]byte
		indices := _indices[:0]
		vid.RUnwrap(vertex.UnwrapOptions{Vertex: func(v *vertex.Vertex) {
			v.Tx.ForEachProducedOutput(func(idx byte, o *ledger.Output, oid *ledger.OutputID) bool {
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

type txListener struct {
	mutex          sync.Mutex
	handlerCounter int
	handlers       map[int]func(tx *transaction.Transaction) bool
}

func (w *Workflow) startListeningTransactions() {
	w.txListener = &txListener{
		handlers: make(map[int]func(tx *transaction.Transaction) bool),
	}
	w.events.OnEvent(EventNewTx, func(vid *vertex.WrappedTx) {
		var tx *transaction.Transaction

		vid.RUnwrap(vertex.UnwrapOptions{Vertex: func(v *vertex.Vertex) {
			tx = v.Tx
		}})
		if tx != nil {
			// no need for goroutine because events are on queue
			w.txListener.runFor(tx)
		}
	})
}

func (tl *txListener) runFor(tx *transaction.Transaction) {
	tl.mutex.Lock()
	defer tl.mutex.Unlock()

	for id, fun := range tl.handlers {
		if !fun(tx) {
			delete(tl.handlers, id)
		}
	}
}

func (w *Workflow) OnTransaction(fun func(tx *transaction.Transaction) bool) {
	w.txListener.mutex.Lock()
	defer w.txListener.mutex.Unlock()

	w.txListener.handlers[w.txListener.handlerCounter] = fun
	w.txListener.handlerCounter++
}

func (w *Workflow) OnTxDeleted(fun func(txid ledger.TransactionID)) {
	w.events.OnEvent(EventTxDeleted, func(txid ledger.TransactionID) {
		fun(txid)
	})
}
