package log

import (
	"flag"
	"strings"

	"github.com/Sirupsen/logrus"
)

var Log = logrus.New()

func init() {
	var (
		formatter string
		level     string
	)

	flag.StringVar(&formatter, "log-formatter", "text", "Formatter name")
	flag.StringVar(&level, "log-level", "DEBUG", "Log level [DEBUG, INFO, WARN, ERROR]")

	flag.Parse()

	// Setup the formatter
	var logFormatter logrus.Formatter
	switch strings.ToLower(formatter) {
	case "text":
		logFormatter = new(logrus.TextFormatter)
	case "json":
		logFormatter = new(logrus.JSONFormatter)
	}
	Log.Formatter = logFormatter

	// Setup the Level
	var logLevel logrus.Level
	switch strings.ToLower(level) {
	case "error":
		logLevel = logrus.Error
	case "warn":
		logLevel = logrus.Warn
	case "info":
		logLevel = logrus.Info
	case "debug":
		logLevel = logrus.Debug
	}
	Log.Level = logLevel
}
