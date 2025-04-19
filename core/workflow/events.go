package workflow

import (
	"github.com/lunfardo314/proxima/core/vertex"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util/eventtype"
)

var (
	EventNewTx     = eventtype.RegisterNew[*vertex.WrappedTx]("new tx") // event may be posted more than once for the transaction
	EventTxDeleted = eventtype.RegisterNew[base.TransactionID]("del tx")
)

func (w *Workflow) PostEventNewTransaction(vid *vertex.WrappedTx) {
	w.events.PostEvent(EventNewTx, vid)
}

func (w *Workflow) PostEventTxDeleted(txid base.TransactionID) {
	w.events.PostEvent(EventTxDeleted, txid)
}
