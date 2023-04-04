package main

import (
	"io"
	"time"

	"github.com/lthibault/log"
	"github.com/sirupsen/logrus"
)

func logger(loglvl, logfmt string, trace, prettyprint bool, w io.Writer) log.Logger {
	return log.New(
		withLevel(trace, loglvl, logfmt),
		withFormat(logfmt, prettyprint),
		withErrWriter(w))
}

func withLevel(trace bool, loglvl, logfmt string) (opt log.Option) {
	var level = log.FatalLevel
	defer func() {
		opt = log.WithLevel(level)
	}()

	if trace {
		level = log.TraceLevel
		return
	}

	if logfmt == "none" {
		return
	}

	switch loglvl {
	case "trace", "t":
		level = log.TraceLevel
	case "debug", "d":
		level = log.DebugLevel
	case "info", "i":
		level = log.InfoLevel
	case "warn", "warning", "w":
		level = log.WarnLevel
	case "error", "err", "e":
		level = log.ErrorLevel
	case "fatal", "f":
		level = log.FatalLevel
	default:
		level = log.InfoLevel
	}

	return
}

func withFormat(logfmt string, prettyprint bool) log.Option {
	var fmt logrus.Formatter

	switch logfmt {
	case "none":
	case "json":
		fmt = &logrus.JSONFormatter{
			PrettyPrint:     prettyprint,
			TimestampFormat: time.RFC3339Nano,
		}
	default:
		fmt = new(logrus.TextFormatter)
	}
	return log.WithFormatter(fmt)
}

func withErrWriter(w io.Writer) log.Option {
	return log.WithWriter(w)
}
