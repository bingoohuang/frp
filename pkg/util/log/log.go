// Copyright 2016 fatedier, fatedier@gmail.com
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package log

import (
	"bytes"
	"log"
)

type Level int

const (
	TraceLevel Level = iota
	DebugLevel
	InfoLevel
	WarnLevel
	ErrorLevel
)

func InitLogger(logPath string, levelStr string, maxDays int, disableLogColor bool) {

}

func Errorf(format string, v ...interface{}) {
	log.Printf("E! "+format, v...)
}

func Warnf(format string, v ...interface{}) {
	log.Printf("W! "+format, v...)
}

func Infof(format string, v ...interface{}) {
	log.Printf(format, v...)
}

func Debugf(format string, v ...interface{}) {
	log.Printf("D! "+format, v...)
}

func Tracef(format string, v ...interface{}) {
	log.Printf("D! "+format, v...)
}

func Logf(level Level, offset int, format string, v ...interface{}) {
	switch level {
	case ErrorLevel:
		log.Printf("E! "+format, v...)
	case WarnLevel:
		log.Printf("W! "+format, v...)
	case InfoLevel:
		log.Printf(format, v...)
	default:
		log.Printf("D! "+format, v...)
	}
}

type WriteLogger struct {
	level  Level
	offset int
}

func NewWriteLogger(level Level, offset int) *WriteLogger {
	return &WriteLogger{
		level:  level,
		offset: offset,
	}
}

func (w *WriteLogger) Write(p []byte) (n int, err error) {
	Logf(w.level, w.offset, string(bytes.TrimRight(p, "\n")))
	return len(p), nil
}
