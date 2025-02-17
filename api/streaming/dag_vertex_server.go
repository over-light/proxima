package streaming

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/lunfardo314/proxima/api"
	"github.com/lunfardo314/proxima/core/txmetadata"
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/transaction"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/set"
)

type (
	environment interface {
		global.Logging
		OnTransaction(fun func(tx *transaction.Transaction) bool)
		TxBytesStore() global.TxBytesStore
	}
	wsServer struct {
		environment
	}
)

const TraceTag = "streaming"

func Run(env environment) {
	srv := &wsServer{
		environment: env,
	}
	srv.Log().Infof("[%s] web socket steraming is running", TraceTag)
	http.HandleFunc(api.PathDAGVertexStream, srv.dagVertexStreamHandler)
}

func vertexDepsForTx(srv *wsServer, txidstr string) []byte {

	txid, err := ledger.TransactionIDFromHexString(txidstr)
	if err != nil {
		return nil
	}

	txBytesWithMetadata := srv.TxBytesStore().GetTxBytesWithMetadata(&txid)
	if len(txBytesWithMetadata) == 0 {
		return nil
	}

	_, txBytes, err := txmetadata.SplitTxBytesWithMetadata(txBytesWithMetadata)
	if err != nil {
		return nil
	}

	tx, err := transaction.FromBytes(txBytes, transaction.MainTxValidationOptions...)
	if err != nil {
		return nil
	}
	resp := api.VertexWithDependenciesFromTransaction(tx)
	respBin, err := json.Marshal(resp)
	if err != nil {
		return nil
	}
	return respBin
}

// WebSocket handler
func (srv *wsServer) dagVertexStreamHandler(w http.ResponseWriter, r *http.Request) {
	u := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	conn, err := u.Upgrade(w, r, nil)
	if err != nil {
		srv.Log().Warnf("[%s] web socket client connected, remote: %s", TraceTag, r.RemoteAddr)
		api.WriteErr(w, "failed to upgrade to websocket connection")
		return
	}

	srv.Log().Infof("[%s] web socket client connected, remote: %s", TraceTag, r.RemoteAddr)

	txids := set.New[string]()
	var respBin []byte
	srv.OnTransaction(func(tx *transaction.Transaction) bool {
		srv.Tracef(TraceTag, "TX ID: %s", tx.IDShortString())

		vertexWD := api.VertexWithDependenciesFromTransaction(tx)

		txids.Insert(vertexWD.ID)
		for _, i := range vertexWD.Inputs {
			if !txids.Contains(i) {
				respBin := vertexDepsForTx(srv, i)
				if respBin != nil {
					srv.Log().Infof("Send tx not seen yet %s", i)
					txids.Insert(i)
					if err = conn.WriteMessage(websocket.TextMessage, respBin); err != nil {
						srv.Log().Infof("[%s] web socket client disconnected, remote: %s, err = %v", TraceTag, r.RemoteAddr, err)
					}
				}
			}
		}

		respBin, err = json.MarshalIndent(vertexWD, "", "  ")
		util.AssertNoError(err)

		if err = conn.WriteMessage(websocket.TextMessage, respBin); err != nil {
			srv.Log().Infof("[%s] web socket client disconnected, remote: %s, err = %v", TraceTag, r.RemoteAddr, err)
		}
		return err == nil // returns false to remove callback
	})
}
