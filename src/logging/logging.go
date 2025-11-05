package logging

import (
	"io"
	"os"

	"github.com/oo-developer/tinymq/src/common"
	log "github.com/sirupsen/logrus"
)

func logger() *log.Entry {
	return log.WithFields(log.Fields{"service": "mmq"})
}

type logging struct {
	format  string
	output  string
	level   string
	warning bool
}

func NewLoggingService(format, output, level string) common.Service {
	f := format
	o := output
	l := level
	warning := false
	if format == "" {
		f = "text"
		warning = true
	}
	if output == "" {
		o = "stderr"
		warning = true
	}
	if l == "" {
		l = "info"
		warning = true
	}

	return &logging{
		format:  f,
		output:  o,
		level:   l,
		warning: warning,
	}
}

func (l logging) Start() {
	log.SetFormatter(getFormatter(l.format))
	log.SetOutput(getOutput(l.output))
	log.SetLevel(getLevel(l.level))
	if l.warning {
		log.Warnf("Logging is not completely configured, default configuration is used.")
	}
	log.Infof("Logging started: Level=%s, Format=%s, Output=%s", l.level, l.format, l.output)
}

func (l logging) Shutdown() {

}

func getLevel(level string) log.Level {
	switch level {
	case "debug":
		return log.DebugLevel
	case "info":
		return log.InfoLevel
	case "warn":
		return log.WarnLevel
	case "error":
		return log.ErrorLevel
	case "fatal":
		return log.FatalLevel
	case "panic":
		return log.PanicLevel
	default:
		return log.InfoLevel
	}
}

func getFormatter(formatter string) log.Formatter {
	switch formatter {
	case "json":
		return &log.JSONFormatter{}
	case "text":
		return &log.TextFormatter{}
	default:
		return &log.TextFormatter{}
	}
}

func getOutput(output string) io.Writer {
	switch output {
	case "stdout":
		return os.Stdout
	case "stderr":
		return os.Stderr
	default:
		file, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY, 0755)
		if err != nil {
			log.Fatal(err)
		}
		return file
	}
}

func Trace(args ...interface{}) {
	logger().Trace(args...)
}

func Debug(args ...interface{}) {
	logger().Debug(args...)
}

func Info(args ...interface{}) {
	logger().Info(args...)
}

func Warn(args ...interface{}) {
	logger().Warn(args...)
}

func Error(args ...interface{}) {
	logger().Error(args...)
}

func Fatal(args ...interface{}) {
	logger().Fatal(args...)
}

func Tracef(format string, args ...interface{}) {
	logger().Tracef(format, args...)
}

func Debugf(format string, args ...interface{}) {
	logger().Debugf(format, args...)
}

func Infof(format string, args ...interface{}) {
	logger().Infof(format, args...)
}

func Warnf(format string, args ...interface{}) {
	logger().Warnf(format, args...)
}

func Errorf(format string, args ...interface{}) {
	logger().Errorf(format, args...)
}

func Fatalf(format string, args ...interface{}) {
	logger().Fatalf(format, args...)
}

func LogError(err error, msg string) {
	if err == nil {
		return
	}
	log.Errorf("%s: %s", msg, err)
}

func LogWarning(err error, msg string) {
	if err == nil {
		return
	}
	log.Warnf("%s: %s", msg, err)
}
