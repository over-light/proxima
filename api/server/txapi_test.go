package server

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lunfardo314/proxima/api"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/tests"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileScript(t *testing.T) {
	srv := &server{}

	// Prepare request
	req := httptest.NewRequest(http.MethodGet, "/txapi/v1/compile_script?source=slice(0x0102,0,0)", nil)
	w := httptest.NewRecorder()

	// Call handler
	srv.compileScript(w, req)

	// Validate response
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	data, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)

	var ret api.Bytecode
	err = json.Unmarshal(data, &ret)
	assert.NoError(t, err)
	assert.Equal(t, "1182010281008100", ret.Bytecode) // Hex for "compiledBytecode"

}

func TestDecompileBytecode(t *testing.T) {
	srv := &server{}

	// Prepare request
	req := httptest.NewRequest(http.MethodGet, "/txapi/v1/decompile_bytecode?bytecode=1182010281008100", nil)
	w := httptest.NewRecorder()

	// Call handler
	srv.decompileBytecode(w, req)

	// Validate response
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	data, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)

	var ret api.ScriptSource
	err = json.Unmarshal(data, &ret)
	assert.NoError(t, err)
	assert.Equal(t, "slice(0x0102,0,0)", ret.Source) // Hex for "compiledBytecode"
}

func TestParseOutputData(t *testing.T) {
	srv := &server{}

	const amount = uint64(31415926535)
	addr := ledger.AddressED25519FromPrivateKey(testutil.GetTestingPrivateKey(100))
	chanID := ledger.RandomChainID()
	cc := ledger.NewChainConstraint(chanID, 1, 2, 0)
	o := ledger.NewOutput(func(o *ledger.Output) {
		o.WithAmount(amount).
			WithLock(addr)
		o.PushConstraint(cc.Bytes())
	})
	oDataStr := hex.EncodeToString(o.Bytes())
	reqStr := fmt.Sprintf("/txapi/v1/parse_output_data?output_data=%s", oDataStr)

	// Prepare request
	req := httptest.NewRequest(http.MethodGet, reqStr, nil)
	w := httptest.NewRecorder()

	// Call handler
	srv.parseOutputData(w, req)

	// Validate response
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	data, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)

	var ret api.ParsedOutput
	err = json.Unmarshal(data, &ret)
	assert.NoError(t, err)

	assert.Equal(t, oDataStr, ret.Data)
	assert.Equal(t, amount, ret.Amount)
	assert.Equal(t, chanID.StringHex(), ret.ChainID)
	assert.Equal(t, 3, len(ret.Constraints))
	assert.Equal(t, "amount(u64/31415926535)", ret.Constraints[0])
	assert.Equal(t, addr.Source(), ret.Constraints[1])
	assert.Equal(t, cc.Source(), ret.Constraints[2])
}

func TestParseOutput(t *testing.T) {
	env, _, err := tests.StartTestEnv()
	require.NoError(t, err)

	mockServer := &server{
		environment: env,
	}

	genesisOut := ledger.GenesisStemOutput()
	oDataStr := hex.EncodeToString(genesisOut.Output.Bytes())

	// Prepare request
	request := fmt.Sprintf("/txapi/v1/parse_output?output_id=%s", genesisOut.ID.StringHex())
	req := httptest.NewRequest(http.MethodGet, request, nil)
	w := httptest.NewRecorder()

	// Call handler
	mockServer.parseOutput(w, req)

	// Validate response
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	data, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)

	var ret api.ParsedOutput
	err = json.Unmarshal(data, &ret)
	assert.NoError(t, err)

	assert.Equal(t, oDataStr, ret.Data)
	assert.Equal(t, len(ret.Constraints), 2)
}

func TestGetTXBytes(t *testing.T) {

	env, txid, err := tests.StartTestEnv()
	require.NoError(t, err)

	// Mock server
	mockServer := &server{
		environment: env,
	}

	// Prepare request
	request := fmt.Sprintf("/txapi/v1/get_txbytes?txid=%s", txid.StringHex())
	req := httptest.NewRequest(http.MethodGet, request, nil)
	w := httptest.NewRecorder()

	// Call handler
	mockServer.getTxBytes(w, req)

	// Validate response
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	data, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)

	var ret api.TxBytes
	err = json.Unmarshal(data, &ret)
	assert.NoError(t, err)
	//assert.Equal(ret.TxBytes, txBytes
	assert.NotEmpty(t, ret.TxBytes)
}

func TestGetParsedTransaction(t *testing.T) {
	//privKey := genesisPrivateKey
	env, txid, err := tests.StartTestEnv()
	require.NoError(t, err)

	// Mock server
	mockServer := &server{
		environment: env,
	}

	// Prepare request
	request := fmt.Sprintf("/txapi/v1/get_parsed_transaction?txid=%s", txid.StringHex())
	req := httptest.NewRequest(http.MethodGet, request, nil)
	w := httptest.NewRecorder()

	// Call handler
	mockServer.getParsedTransaction(w, req)

	// Validate response
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	data, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)

	var ret api.TransactionJSONAble
	err = json.Unmarshal(data, &ret)
	assert.NoError(t, err)
	assert.Equal(t, ret.TotalAmount, uint64(0x38d7ea4c68000))
	assert.Equal(t, ret.IsBranch, true)
	assert.Equal(t, len(ret.Inputs), 2)
	assert.Equal(t, len(ret.Outputs), 5)
}

func TestGetVertexDep(t *testing.T) {
	env, txid, err := tests.StartTestEnv()
	require.NoError(t, err)

	// Mock server
	mockServer := &server{
		environment: env,
	}

	// Prepare request
	request := fmt.Sprintf("/txapi/v1/get_vertex_dep?txid=%s", txid.StringHex())
	req := httptest.NewRequest(http.MethodGet, request, nil)
	w := httptest.NewRecorder()

	// Call handler
	mockServer.getVertexWithDependencies(w, req)

	// Validate response
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	data, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)

	t.Logf("JSON data:\n%s", string(data))
	var ret api.VertexWithDependencies
	err = json.Unmarshal(data, &ret)
	assert.NoError(t, err)
	txidBack, err := ledger.TransactionIDFromHexString(ret.ID)
	assert.NoError(t, err)
	assert.EqualValues(t, ret.SequencerID, ledger.BoostrapSequencerIDHex)
	assert.EqualValues(t, *txid, txidBack)
	assert.True(t, txid.IsSequencerMilestone())
	assert.True(t, txid.IsBranchTransaction())
	assert.EqualValues(t, 1_000_000_000_000_000, ret.TotalAmount)
	assert.EqualValues(t, 0, ret.TotalInflation)
	assert.True(t, ret.SequencerInputTxIndex != nil && *ret.SequencerInputTxIndex == 0)
	assert.True(t, ret.StemInputTxIndex != nil && *ret.StemInputTxIndex == 0)
	assert.EqualValues(t, 1, len(ret.Inputs))
	assert.EqualValues(t, 0, len(ret.Endorsements))
}

// use this function is avoid crash for err = nil
func (srv *server) AssertNoError(err error, prefix ...string) {
	util.AssertNoError(err, prefix...)
}
