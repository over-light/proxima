package streaming

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/lunfardo314/proxima/api"
	"github.com/lunfardo314/proxima/core/vertex"
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger/transaction"
	"github.com/lunfardo314/proxima/util"
)

type (
	environment interface {
		global.Logging
		ListenToVids(fun func(vid *vertex.WrappedTx))
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
		err := http.ListenAndServe(":8080", nil)
		util.AssertNoError(err)
	}()
}

// Stream data to the WebSocket client
func streamData(conn *websocket.Conn, data []byte) error {

	// Send message to client
	return conn.WriteMessage(websocket.TextMessage, []byte(data))
}

// WebSocket handler
func (srv *ws_server) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	util.AssertNoError(err)
	srv.Tracef(TraceTag, "Client connected")

	srv.ListenToVids(func(vid *vertex.WrappedTx) {
		srv.Tracef(TraceTag, "TX ID: %s", vid.IDShortString())

		var tx *transaction.Transaction
		vid.RUnwrap(vertex.UnwrapOptions{
			Vertex: func(v *vertex.Vertex) {
				tx = v.Tx
			},
		})

		if tx != nil {
			vidDep := api.VertexWithDependenciesFromTransaction(tx)
			respBin, err := json.MarshalIndent(vidDep, "", "  ")
			if err != nil {
				srv.Tracef(TraceTag, "Error in MarshalIndent: %s", err.Error())
			}

			//log.Printf("Sending JSON-encoded vid: %s", string(respBin))
			err = streamData(conn, respBin)
			if err != nil {
				srv.Tracef(TraceTag, "Client disconnected")
				return
			}
		} else {
			srv.Log().Infof("wsHandler error: tx is nil")
		}
	})
}
