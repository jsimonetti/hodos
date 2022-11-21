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
package config

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/pelletier/go-toml"
)

const (
	DEF_MINIMUMUP = 1
	BURSTSIZE_MIN = 1
	BURSTSIZE_MAX = 5
	TABLE_MAX     = 4294967295

	DEF_BURSTSIZE     int           = 3
	DEF_BURSTINTERVAL time.Duration = 15 * time.Second
	DEF_ICMPINTERVAL                = 2 * time.Second
	DEF_ICMPTIMEOUT                 = 250 * time.Millisecond
)

// cfgFile is the top-level of the configuration
type cfgFile struct {
	Debug bool `toml:"debug"` // wether to do tracing or not

	BurstInterval *string `toml:"burst_interval,omit_empty"` // global default ping interval (default 5s)
	BurstSize     *int    `toml:"burst_size,omit_empty"`     // number of pings to send (default 1)
	ICMPInterval  *string `toml:"icmp_interval,omit_empty"`  // global default ping interval (default 1s)
	ICMPTimeout   *string `toml:"icmp_timeout,omit_empty"`   // global default ping timeout (default 200ms)

	UpAction   string `toml:"up_action"`   // command to run when an interface goes up (also run at startup)
	DownAction string `toml:"down_action"` // command to run when an interface goes down

	Interfaces []cfgInterface `toml:"interfaces"`
}

type cfgInterface struct {
	Name        string `toml:"name"`        // Interface name (as seen in `ip link ls`)
	Description string `toml:"description"` // Description for this interface
	Debug       bool   `toml:"debug"`       // enable tracing for this interface

	Table  *int `toml:"table,omit_empty"`  // route table number for this interface
	Metric *int `toml:"metric,omit_empty"` // route table number for this interface

	UpAction   *string `toml:"up_action,omit_empty"`   // command to run when interface goes up (also run at startup)
	DownAction *string `toml:"down_action,omit_empty"` // command to run when interface goes down

	BurstInterval *string `toml:"burst_interval"` // global default ping interval (default 5s)
	BurstSize     *int    `toml:"burst_size"`     // number of pings to send (default 1)
	ICMPInterval  *string `toml:"icmp_interval"`  // global default ping interval (default 1s)
	ICMPTimeout   *string `toml:"icmp_timeout"`   // global default ping timeout (default 200ms)
	MinimumUp     *int    `toml:"minimum_up"`     // minimum amount of hosts to be up for this interface to be considered up (default: 1)

	Hosts []cfgHost `toml:"hosts,omitempty"`
}

type cfgHost struct {
	Name  string `toml:"name"`
	Host  string `toml:"host"`  // ip to use for pinging
	Debug bool   `toml:"debug"` // enable tracing for this host

	BurstInterval *string `toml:"burst_interval,omit_empty"` // global default ping interval (default 5s)
	BurstSize     *int    `toml:"burst_size,omit_empty"`     // number of pings to send (default 1)
	ICMPInterval  *string `toml:"icmp_interval,omit_empty"`  // global default ping interval (default 1s)
	ICMPTimeout   *string `toml:"icmp_timeout,omit_empty"`   // global default ping timeout (default 200ms)
}

func Parse(r io.Reader) (*Config, error) {
	var cfg cfgFile
	var err error
	if err = toml.NewDecoder(r).Strict(true).Decode(&cfg); err != nil {
		return nil, err
	}

	// Must configure at least one interface.
	if len(cfg.Interfaces) == 0 {
		return nil, errors.New("no interfaces configured")
	}

	c := &Config{
		Interfaces: make([]Interface, 0, len(cfg.Interfaces)),
		Debug:      cfg.Debug,
		UpAction:   cfg.UpAction,
		DownAction: cfg.DownAction,
	}

	c.BurstSize = DEF_BURSTSIZE
	if cfg.BurstSize != nil {
		if *cfg.BurstSize < BURSTSIZE_MIN || *cfg.BurstSize > BURSTSIZE_MAX {
			return nil, fmt.Errorf("burst_size is incorrect: %d, should be between %d and %d", *cfg.BurstSize, BURSTSIZE_MIN, BURSTSIZE_MAX)
		}
		c.BurstSize = *cfg.BurstSize
	}

	if c.BurstInterval, err = parseDuration(cfg.BurstInterval, DEF_BURSTINTERVAL); err != nil {
		return nil, err
	}
	if c.ICMPInterval, err = parseDuration(cfg.ICMPInterval, DEF_ICMPINTERVAL); err != nil {
		return nil, err
	}
	if c.ICMPTimeout, err = parseDuration(cfg.ICMPTimeout, DEF_ICMPTIMEOUT); err != nil {
		return nil, err
	}

	// Check that each interface is unique.
	// TODO(jsi): add check for unique tables
	seen := make(map[string]bool)
	for i, iface := range cfg.Interfaces {
		ifi, err := parseInterface(iface, c)
		if err != nil {
			// Narrow down the location of a configuration error.
			return nil, fmt.Errorf("interface %d: %v", i, err)
		}

		if _, ok := seen[ifi.Name]; ok {
			return nil, fmt.Errorf("interface %d: %q cannot appear multiple times in configuration", i, ifi.Name)
		}
		seen[ifi.Name] = true

		c.Interfaces = append(c.Interfaces, *ifi)
	}

	return c, nil
}

type Config struct {
	Debug bool

	BurstInterval time.Duration
	BurstSize     int
	ICMPInterval  time.Duration
	ICMPTimeout   time.Duration

	UpAction   string
	DownAction string

	Interfaces []Interface
}

// parseDuration parses a duration while also recognizing special values such
// as auto and infinite. If the key is unset or auto, def is used.
func parseDuration(s *string, def time.Duration) (time.Duration, error) {
	if s == nil {
		// Nil implies the key is not set at all, so use the default.
		return def, nil
	}
	// Use the user's value, but validate it per the RFC.
	return time.ParseDuration(*s)
}
