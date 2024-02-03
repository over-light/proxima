package workflow

import (
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type (
	ConfigParams struct {
		logLevel         zapcore.Level
		consumerLogLevel map[string]zapcore.Level
		logOutput        []string
		logTimeLayout    string
		doNotStartPruner bool
	}

	ConfigOption func(c *ConfigParams)
)

func defaultConfigParams() ConfigParams {
	return ConfigParams{
		logLevel:         zap.InfoLevel,
		consumerLogLevel: make(map[string]zapcore.Level),
		logOutput:        []string{"stdout"},
		logTimeLayout:    global.TimeLayoutDefault,
	}
}

func WithLogLevel(lvl zapcore.Level) ConfigOption {
	return func(c *ConfigParams) {
		c.logLevel = lvl
	}
}

func WithConsumerLogLevel(name string, lvl zapcore.Level) ConfigOption {
	return func(c *ConfigParams) {
		c.consumerLogLevel[name] = lvl
	}
}

func WithLogOutput(out string) ConfigOption {
	return func(c *ConfigParams) {
		if out != "" && util.Find(c.logOutput, out) == -1 {
			c.logOutput = append(c.logOutput, out)
		}
	}
}

func WithLogTimeLayout(layout string) ConfigOption {
	return func(c *ConfigParams) {
		if layout != "" {
			c.logTimeLayout = layout
		}
	}
}

func OptionDoNotStartPruner(c *ConfigParams) {
	c.doNotStartPruner = true
}

var allComponentNames = make([]string, 0)

func WithGlobalConfigOptions(c *ConfigParams) {
	WithLogLevel(parseLevel("logger.level", zap.InfoLevel))(c)

	for _, n := range allComponentNames {
		WithConsumerLogLevel(n, parseLevel("workflow."+n+".loglevel", zap.InfoLevel))(c)
	}
	WithLogOutput(viper.GetString("logger.output"))(c)
	WithLogTimeLayout(viper.GetString("logger.timelayout"))
}

func parseLevel(key string, def zapcore.Level) zapcore.Level {
	lvl, err := zapcore.ParseLevel(viper.GetString(key))
	if err != nil {
		return def
	}
	return lvl
}
