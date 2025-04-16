package tests

import (
	"testing"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	id, _ := ledger.GetTestingIdentityData()
	lib := ledger.InitLocally(id, true)
	t.Logf("------------------\n%s", lib.ID.String())
	t.Logf("------------------\n%s", string(lib.ID.YAML()))
	t.Logf("------------------\n%s", lib.ID.TimeConstantsToString())
}

func TestLedgerIDYAML(t *testing.T) {
	id := ledger.L().ID
	yamlableStr := id.YAMLAble().YAML()
	t.Logf("\n%s", string(yamlableStr))

	idBack, err := ledger.StateIdentityDataFromYAML(yamlableStr)
	require.NoError(t, err)
	require.EqualValues(t, id.Bytes(), idBack.Bytes())
}

func TestLedgerToYAML(t *testing.T) {
	t.Run("compiled", func(t *testing.T) {
		yamlData := ledger.L().ToYAML(true, "# ------------------- Proxima ledger definitions COMPILED -------------------------")
		t.Logf("\n%s", string(yamlData))
	})
	t.Run("not compiled", func(t *testing.T) {
		yamlData := ledger.L().ToYAML(false, "# ------------------- Proxima ledger definitions NOT COMPILED -------------------------")
		t.Logf("\n%s", string(yamlData))
	})
}

//func TestLedgerToYAMLFile(t *testing.T) {
//	yamlData := ledger.L().ToYAML(true, "# ------------------- Proxima ledger definitions COMPILED -------------------------")
//	_ = os.WriteFile("ledger.yaml", yamlData, 0644)
//
//}
