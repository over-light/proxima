package streaming

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/lunfardo314/proxima/api"
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger/transaction"
	"github.com/lunfardo314/proxima/util"
)

type (
	environment interface {
		global.Logging
		OnTransaction(fun func(tx *transaction.Transaction) bool)
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
	http.HandleFunc(api.PathDAGVertexStream, srv.dagVertexStreamHandler)
}

// WebSocket handler
func (srv *wsServer) dagVertexStreamHandler(w http.ResponseWriter, r *http.Request) {
	u := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	conn, err := u.Upgrade(w, r, nil)
	if err != nil {
		api.WriteErr(w, "failed to upgrade to websocket connection")
		return
	}

	srv.Log().Infof("web socket client connected. Remote = %s", r.RemoteAddr)

	var respBin []byte
	srv.OnTransaction(func(tx *transaction.Transaction) bool {
		srv.Tracef(TraceTag, "TX ID: %s", tx.IDShortString())

		vertexWD := api.VertexWithDependenciesFromTransaction(tx)
		respBin, err = json.MarshalIndent(vertexWD, "", "  ")
		util.AssertNoError(err)

		if err = conn.WriteMessage(websocket.TextMessage, respBin); err != nil {
			srv.Log().Infof("web socket client disconnected. Remote = %s", r.RemoteAddr)
		}
		return err == nil // returns false to remove callback
	})
}
