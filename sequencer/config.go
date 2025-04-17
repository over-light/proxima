package sequencer

import (
	"crypto/ed25519"
	"fmt"
	"math"
	"time"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/lines"
	"github.com/spf13/viper"
)

type (
	ConfigOptions struct {
		SequencerName             string
		Pace                      int // pace in ticks
		MaxTagAlongInputs         int
		MaxInputs                 int
		MaxTargetTs               base.LedgerTime
		MaxBranches               int
		EnsureSyncedBeforeStart   bool
		DelayStart                time.Duration
		BacklogTagAlongTTLSlots   int
		BacklogDelegationTTLSlots int
		MilestonesTTLSlots        int
		SingleSequencerEnforced   bool
		ForceRunInflator          bool
	}

	ConfigOption func(options *ConfigOptions)
)

const (
	defaultMaxInputs                 = 100
	defaultMaxTagAlongInputs         = 50
	minimumBacklogTagAlongTTLSlots   = 10
	minimumBacklogDelegationTTLSlots = 20
	minimumMilestonesTTLSlots        = 24 // 10
)

func defaultConfigOptions() *ConfigOptions {
	return &ConfigOptions{
		SequencerName:             "seq",
		Pace:                      ledger.TransactionPaceSequencer(),
		MaxTagAlongInputs:         defaultMaxTagAlongInputs,
		MaxInputs:                 defaultMaxInputs,
		MaxTargetTs:               base.NilLedgerTime,
		MaxBranches:               math.MaxInt,
		DelayStart:                ledger.SlotDuration(),
		BacklogTagAlongTTLSlots:   minimumBacklogTagAlongTTLSlots,
		BacklogDelegationTTLSlots: minimumBacklogDelegationTTLSlots,
		MilestonesTTLSlots:        minimumMilestonesTTLSlots,
	}
}

func configOptions(opts ...ConfigOption) *ConfigOptions {
	cfg := defaultConfigOptions()
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func paramsFromConfig() ([]ConfigOption, ledger.ChainID, ed25519.PrivateKey, error) {
	subViper := viper.Sub("sequencer")
	if subViper == nil {
		return nil, ledger.ChainID{}, nil, nil
	}
	name := subViper.GetString("name")
	if name == "" {
		return nil, ledger.ChainID{}, nil, fmt.Errorf("StartFromConfig: sequencer must have a name")
	}

	if !subViper.GetBool("enable") {
		// will skip
		return nil, ledger.ChainID{}, nil, nil
	}
	seqID, err := ledger.ChainIDFromHexString(subViper.GetString("chain_id"))
	if err != nil {
		return nil, ledger.ChainID{}, nil, fmt.Errorf("StartFromConfig: can't parse sequencer chain id: %v", err)
	}
	controllerKey, err := util.ED25519PrivateKeyFromHexString(subViper.GetString("controller_key"))
	if err != nil {
		return nil, ledger.ChainID{}, nil, fmt.Errorf("StartFromConfig: can't parse private key: %v", err)
	}

	backlogTagAlongTTLSlots := subViper.GetInt("backlog_tag_along_ttl_slots")
	if backlogTagAlongTTLSlots < minimumBacklogTagAlongTTLSlots {
		backlogTagAlongTTLSlots = minimumBacklogTagAlongTTLSlots
	}
	backlogDelegationTTLSlots := subViper.GetInt("backlog_delegation_ttl_slots")
	if backlogDelegationTTLSlots < minimumBacklogDelegationTTLSlots {
		backlogDelegationTTLSlots = minimumBacklogDelegationTTLSlots
	}
	milestonesTTLSlots := subViper.GetInt("milestones_ttl_slots")
	if milestonesTTLSlots < minimumMilestonesTTLSlots {
		milestonesTTLSlots = minimumMilestonesTTLSlots
	}

	cfg := []ConfigOption{
		WithName(name),
		WithPace(subViper.GetInt("pace")),
		WithMaxInputs(subViper.GetInt("max_inputs"), subViper.GetInt("max_tag_along_inputs")),
		WithMaxBranches(subViper.GetInt("max_branches")),
		WithBacklogTagAlongTTLSlots(backlogTagAlongTTLSlots),
		WithBacklogDelegationTTLSlots(backlogDelegationTTLSlots),
		WithMilestonesTTLSlots(milestonesTTLSlots),
		WithSingleSequencerEnforced,
	}
	if subViper.GetBool("ensure_synced_at_startup") {
		cfg = append(cfg, WithEnsureSyncedAtStartup)
	}
	return cfg, seqID, controllerKey, nil
}

func WithName(name string) ConfigOption {
	return func(o *ConfigOptions) {
		o.SequencerName = name
	}
}

func WithPace(pace int) ConfigOption {
	return func(o *ConfigOptions) {
		if pace < ledger.TransactionPaceSequencer() {
			pace = ledger.TransactionPaceSequencer()
		}
		o.Pace = pace
	}
}

func WithDelayStart(delay time.Duration) ConfigOption {
	return func(o *ConfigOptions) {
		o.DelayStart = delay
	}
}

func WithMaxInputs(maxInputs, maxTagAlongInputs int) ConfigOption {
	return func(o *ConfigOptions) {
		if maxInputs <= 0 || maxTagAlongInputs <= 0 || maxInputs > 254 || maxTagAlongInputs > maxInputs {
			o.MaxInputs = defaultMaxInputs
			o.MaxTagAlongInputs = defaultMaxTagAlongInputs
		} else {
			o.MaxInputs = maxInputs
			o.MaxTagAlongInputs = maxTagAlongInputs
		}
	}
}

func WithMaxBranches(maxBranches int) ConfigOption {
	return func(o *ConfigOptions) {
		if maxBranches >= 1 {
			o.MaxBranches = maxBranches
		}
	}
}

func WithBacklogTagAlongTTLSlots(slots int) ConfigOption {
	return func(o *ConfigOptions) {
		o.BacklogTagAlongTTLSlots = slots
	}
}

func WithBacklogDelegationTTLSlots(slots int) ConfigOption {
	return func(o *ConfigOptions) {
		o.BacklogDelegationTTLSlots = slots
	}
}

func WithMilestonesTTLSlots(slots int) ConfigOption {
	return func(o *ConfigOptions) {
		o.MilestonesTTLSlots = slots
	}
}

func WithEnsureSyncedAtStartup(o *ConfigOptions) {
	o.EnsureSyncedBeforeStart = true
}

func WithSingleSequencerEnforced(o *ConfigOptions) {
	o.SingleSequencerEnforced = true
}

func WithForceInflator() ConfigOption {
	return func(o *ConfigOptions) {
		o.ForceRunInflator = true
	}
}

func (cfg *ConfigOptions) lines(seqID ledger.ChainID, controller ledger.AddressED25519, prefix ...string) *lines.Lines {
	return lines.New(prefix...).
		Add("id: %s", seqID.String()).
		Add("Controller: %s", controller.String()).
		Add("Name: %s", cfg.SequencerName).
		Add("Pace: %d ticks", cfg.Pace).
		Add("MaxTagAlongInputs: %d", cfg.MaxTagAlongInputs).
		Add("MaxInputs: %d", cfg.MaxInputs).
		Add("MaxTargetTs: %s", cfg.MaxTargetTs.String()).
		Add("MaxSlots: %d", cfg.MaxBranches).
		Add("DelayStart: %v", cfg.DelayStart).
		Add("BacklogTagAlongTTLSlots: %d", cfg.BacklogTagAlongTTLSlots).
		Add("BacklogDelegationTTLSlots: %d", cfg.BacklogDelegationTTLSlots).
		Add("MilestoneTTLSlots: %d", cfg.MilestonesTTLSlots)
}
