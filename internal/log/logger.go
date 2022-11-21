// Copyright 2019-2022 Jeroen Simonetti
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package log

import (
	"log"
	"os"
)

type Logger interface {
	Fatalf(string, ...interface{})
	Fatal(...interface{})
	Printf(string, ...interface{})
	Print(...interface{})
	Errorf(string, ...interface{})
	Error(...interface{})
	Warnf(string, ...interface{})
	Warn(...interface{})
	Infof(string, ...interface{})
	Info(...interface{})
	Debug(...interface{})
	Debugf(string, ...interface{})
}

func Default() Logger {
	return New(log.New(os.Stderr, "", log.LstdFlags))
}

func DefaultDebug() Logger {
	return NewDebug(log.New(os.Stderr, "", log.LstdFlags))
}
