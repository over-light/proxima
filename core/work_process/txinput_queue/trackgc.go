package txinput_queue

import (
	"fmt"
	"time"

	"github.com/lunfardo314/proxima/core/txmetadata"
	"github.com/lunfardo314/proxima/util/trackgc"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	trackTxBytes    = trackgc.NewByteArrayTracker()
	trackTxMetadata = trackgc.New[txmetadata.TransactionMetadata](func(p *txmetadata.TransactionMetadata) string {
		return fmt.Sprintf("%p", p)
	})
)

func StartTrackingTxBytes(env environment) {
	trackTxBytesAllocGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "proxima_trackgc_txbytes_nalloc",
		Help: "track GC of the object",
	})
	trackTxBytesTotalGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "proxima_trackgc_txbytes_total",
		Help: "track GC of the object",
	})
	trackTxMetadataGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "proxima_trackgc_txmetadata",
		Help: "track GC of the object",
	})

	trackTxBytes.StartTrackingWithMetrics(trackTxBytesAllocGauge, 3*time.Second, trackTxBytesTotalGauge)
	trackTxMetadata.StartTrackingWithMetrics(trackTxMetadataGauge, 3*time.Second)

	env.MetricsRegistry().MustRegister(trackTxBytesAllocGauge, trackTxMetadataGauge, trackTxBytesTotalGauge)
}
