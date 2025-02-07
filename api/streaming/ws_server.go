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
		ListenToTransactions(fun func(tx *transaction.Transaction))
	}

	ws_server struct {
		environment
	}
)

const TraceTag = "streaming"

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func Run(addr string, env environment) {

	srv := &ws_server{
		environment: env,
	}

	http.HandleFunc("/ws", srv.wsHandler)
	srv.Tracef(TraceTag, "WebSocket server starting on :8080")
	go func() {
		err := http.ListenAndServe(addr, nil)
		util.AssertNoError(err)
	}()
}

// Stream data to the WebSocket client
func streamData(conn *websocket.Conn, data []byte) error {

	// Send message to client
	return conn.WriteMessage(websocket.TextMessage, data)
}

// WebSocket handler
func (srv *ws_server) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		api.WriteErr(w, "failed to upgrade to websocket connection")
		return
	}

	srv.Log().Infof("web socket client connected. Remote = %s", r.RemoteAddr)

	srv.ListenToTransactions(func(tx *transaction.Transaction) {
		srv.Tracef(TraceTag, "TX ID: %s", tx.IDShortString())

		if tx != nil {
			vidDep := api.VertexWithDependenciesFromTransaction(tx)
			respBin, err := json.MarshalIndent(vidDep, "", "  ")
			if err != nil {
				srv.Tracef(TraceTag, "Error in MarshalIndent: %s", err.Error())
			}

			err = streamData(conn, respBin)
			if err != nil {
				srv.Log().Infof("web socket client disconnected. Remote = %s", r.RemoteAddr)
				return
			}
		} else {
			srv.Log().Infof("wsHandler error: tx is nil")
		}
	})
}
