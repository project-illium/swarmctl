// Copyright (c) 2022 The illium developers
// Use of this source code is governed by an MIT
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	black color = iota + 30
	red
	green
	yellow
	blue
	magenta
	cyan
	white
)

var log = zap.S()

// color represents a text color.
type color uint8

// Add adds the coloring to the given string.
func (c color) Add(s string) string {
	return fmt.Sprintf("\x1b[%dm%s\x1b[0m", uint8(c), s)
}

var LogLevelMap = map[string]zapcore.Level{
	"debug":     zap.DebugLevel,
	"info":      zap.InfoLevel,
	"warning":   zap.WarnLevel,
	"error":     zap.ErrorLevel,
	"alert":     zap.DPanicLevel,
	"critical":  zap.PanicLevel,
	"emergency": zap.FatalLevel,
}

var logLevelSeverity = map[zapcore.Level]string{
	zapcore.DebugLevel:  "DEBUG",
	zapcore.InfoLevel:   "INFO",
	zapcore.WarnLevel:   "WARNING",
	zapcore.ErrorLevel:  "ERROR",
	zapcore.DPanicLevel: "CRITICAL",
	zapcore.PanicLevel:  "ALERT",
	zapcore.FatalLevel:  "EMERGENCY",
}

func setupLogging() (*zap.AtomicLevel, error) {
	cfg := zap.NewProductionConfig()

	logLevel, ok := LogLevelMap[strings.ToLower("debug")]
	if !ok {
		return nil, errors.New("invalid log level")
	}
	cfg.Encoding = "console"
	cfg.Level = zap.NewAtomicLevelAt(logLevel)

	levelToColor := map[zapcore.Level]color{
		zapcore.DebugLevel:  magenta,
		zapcore.InfoLevel:   blue,
		zapcore.WarnLevel:   yellow,
		zapcore.ErrorLevel:  red,
		zapcore.DPanicLevel: red,
		zapcore.PanicLevel:  red,
		zapcore.FatalLevel:  red,
	}
	customLevelEncoder := func(level zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString("[" + levelToColor[level].Add(logLevelSeverity[level]) + "]")
	}
	cfg.EncoderConfig.EncodeLevel = customLevelEncoder
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	cfg.DisableCaller = true
	cfg.DisableStacktrace = true
	cfg.EncoderConfig.ConsoleSeparator = "  "

	logger, err := cfg.Build()
	if err != nil {
		return nil, err
	}
	zap.ReplaceGlobals(logger)

	log = zap.S()
	return &cfg.Level, nil
}
