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
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/jsimonetti/hodos/internal/build"
	"github.com/jsimonetti/hodos/internal/cap"
	"github.com/jsimonetti/hodos/internal/config"
	logger "github.com/jsimonetti/hodos/internal/log"
	"github.com/jsimonetti/hodos/internal/server"

	"net/http"
	_ "net/http/pprof"
)

const thisApp = "hodosd"
const defaultCfgFile = "hodos.toml"

var (
	cfgFlag     = flag.String("c", defaultCfgFile, "path to configuration file")
	exampleFlag = flag.Bool("example", false, "print out an example configuration")
	debug       = flag.Bool("d", false, "enable debug logging")
)

func main() {
	flag.Usage = func() {
		// Indicate version in usage.
		fmt.Printf("%s\nflags:\n", build.Banner(thisApp))
		flag.PrintDefaults()
	}
	flag.Parse()

	//ll := log.New(os.Stderr, fmt.Sprintf("[%d] ", os.Getpid()), log.LstdFlags)
	ll := log.New(os.Stderr, "", 0)
	var l logger.Logger
	l = logger.New(ll)
	if *debug {
		l = logger.NewDebug(ll)
	}

	if *exampleFlag {
		fmt.Printf("%s\n", config.DefaulConfig())
		return
	}

	if !cap.HasCapabilities() {
		log.Print("you don't have the proper rights")
		log.Fatalf("either add CAP_NET_ADMIN (setcap 'cap_net_admin+p' %s) or run as root", os.Args[0])
	}

	l.Print(fmt.Sprintf("%s starting with configuration file %q", build.Banner(thisApp), *cfgFlag))

	// open the config file
	f, err := os.Open(*cfgFlag)
	if err != nil {
		l.Fatalf("failed to open configuration file: %v", err)
	}

	// parse config file
	cfg, err := config.Parse(f)
	if err != nil {
		l.Fatalf("failed to parse %q: %v", f.Name(), err)
	}
	_ = f.Close()

	go func() {
		l.Print(http.ListenAndServe(":6060", nil))
	}()

	ctx := context.Background()
	// run the server
	server, err := server.New(ctx, l, cfg)
	if err != nil {
		l.Fatalf("failed to start server: %s", err)
	}
	server.Start()

	l.Debugf("shut down with this many routines left: %d\n", runtime.NumGoroutine())
}
