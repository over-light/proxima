package glb

import (
	"sync"
	"time"

	"github.com/lunfardo314/proxima/api/client"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/spf13/viper"
)

var (
	UseAlternativeTagAlongSequencer bool
	TargetInclusionDepth            int
)

var displayEndpointOnce sync.Once

func GetClient(endpoint ...string) *client.APIClient {
	endp := ""
	if len(endpoint) > 0 {
		endp = endpoint[0]
	} else {
		endp = viper.GetString("api.endpoint")
	}
	Assertf(endp != "", "GetClient: node API endpoint not specified")
	var timeout []time.Duration
	if timeoutSec := viper.GetInt("api.timeout_sec"); timeoutSec > 0 {
		timeout = []time.Duration{time.Duration(timeoutSec) * time.Second}
	}
	displayEndpointOnce.Do(func() {
		if len(timeout) == 0 {
			Infof("using API endpoint: %s, default timeout", endpoint)
		} else {
			Infof("using API endpoint: %s, timeout: %v", endpoint, timeout[0])
		}
	})
	return client.NewWithGoogleDNS(endp, timeout...)
}

func InitLedgerFromNode() {
	ledgerID, err := GetClient().GetLedgerIdentityData()
	AssertNoError(err)
	ledger.MustInitSingleton(ledgerID)
	Infof("successfully connected to the node at %s", viper.GetString("api.endpoint"))
	Infof("verbose = %v", IsVerbose())
}
