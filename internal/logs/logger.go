package logs

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

// Logger — глобальный логгер приложения (инициализируется через Init).
var Logger *logrus.Logger

// Options — параметры инициализации логгера.
type Options struct {
	Level  string // trace|debug|info|warning|error|fatal
	Format string // text|json
	File   string // путь/префикс лог-файла; если пусто — только stdout
}

// Init настраивает глобальный логгер по переданным опциям.
func Init(opts Options) {
	l := logrus.New()

	// уровень
	switch opts.Level {
	case "trace":
		l.SetLevel(logrus.TraceLevel)
	case "debug":
		l.SetLevel(logrus.DebugLevel)
	case "warning", "warn":
		l.SetLevel(logrus.WarnLevel)
	case "error":
		l.SetLevel(logrus.ErrorLevel)
	case "fatal":
		l.SetLevel(logrus.FatalLevel)
	default:
		l.SetLevel(logrus.InfoLevel)
	}

	// формат
	if opts.Format == "json" {
		l.SetFormatter(&logrus.JSONFormatter{})
	}

	// вывод
	if opts.File != "" {
		currentTime := time.Now().Format("2006-01-02_15-04-05")
		logFileName := fmt.Sprintf("%s_%s.log", opts.File, currentTime)
		file, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
		if err != nil {
			l.Fatalf("failed to open log file %s: %v", logFileName, err)
		}
		l.SetOutput(io.MultiWriter(file, os.Stdout))
	} else {
		l.SetOutput(os.Stdout)
	}

	Logger = l
}
