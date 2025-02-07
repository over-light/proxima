package api

import (
	"encoding/json"
	"net/http"

	"github.com/lunfardo314/proxima/util"
)

func WriteErr(w http.ResponseWriter, errStr string) {
	respBytes, err := json.Marshal(&Error{Error: errStr})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = w.Write(respBytes)
	util.AssertNoError(err)
}

func WriteOk(w http.ResponseWriter) {
	respBytes, err := json.Marshal(&Error{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = w.Write(respBytes)
	util.AssertNoError(err)
}

func SetHeader(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
}
