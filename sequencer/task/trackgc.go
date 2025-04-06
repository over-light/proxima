package task

import (
	"fmt"
	"sync"
	"time"

	"github.com/lunfardo314/proxima/core/attacher"
	"github.com/lunfardo314/proxima/util/trackgc"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	trackTasks = trackgc.New[taskData](func(p *taskData) string {
		return "taskData-" + p.Name
	})

	trackProposers = trackgc.New[proposer](func(p *proposer) string {
		return "proposer-" + p.Name
	})

	trackIncAttachers = trackgc.New[attacher.IncrementalAttacher](func(p *attacher.IncrementalAttacher) string {
		return "incAtt-" + p.Name()
	})

	trackProposals = trackgc.New[proposal](func(p *proposal) string {
		return "proposal-" + p.hrString
	})

	trackSlotData = trackgc.New[SlotData](func(p *SlotData) string {
		return fmt.Sprintf("slot-data-%d", p.slot)
	})

	registerMetricsOnce sync.Once
)

var runTaskDurationGauge = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "proxima_seq_run_task_duration",
	Help: "duration of running task in milliseconds",
})

func registerGCMetricsOnce(env environment) {
	registerMetricsOnce.Do(func() {
		reg := env.MetricsRegistry()
		if reg == nil {
			return
		}

		trackTasksGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "proxima_trackgc_tasks",
			Help: "not GCed object counter",
		})
		trackProposersGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "proxima_trackgc_proposers",
			Help: "not GCed object counter",
		})
		trackIncAttachersGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "proxima_trackgc_inc_attachers",
			Help: "not GCed object counter",
		})
		trackProposalsGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "proxima_trackgc_proposals",
			Help: "not GCed object counter",
		})
		trackSlotDataGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "proxima_trackgc_slot_data",
			Help: "not GCed object counter",
		})

		trackTasks.StartTrackingWithMetrics(trackTasksGauge, 3*time.Second)
		trackProposers.StartTrackingWithMetrics(trackProposersGauge, 3*time.Second)
		trackIncAttachers.StartTrackingWithMetrics(trackIncAttachersGauge, 3*time.Second)
		trackProposals.StartTrackingWithMetrics(trackProposalsGauge, 3*time.Second)
		trackSlotData.StartTrackingWithMetrics(trackSlotDataGauge, 3*time.Second)

		reg.MustRegister(
			trackTasksGauge,
			trackProposersGauge,
			trackIncAttachersGauge,
			trackProposalsGauge,
			trackSlotDataGauge,
			runTaskDurationGauge,
		)
	})
}
