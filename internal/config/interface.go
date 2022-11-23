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
	"fmt"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

// An Interface provides configuration for an individual interface.
type Interface struct {
	Name        string
	Description string
	Debug       bool

	Table      uint32
	Metric     uint32
	UpAction   string
	DownAction string

	BurstInterval time.Duration
	BurstSize     int
	ICMPInterval  time.Duration
	ICMPTimeout   time.Duration

	MinimumUp    int
	upHostsv4    int32
	upHostsv6    int32
	totalHostsv4 int32
	totalHostsv6 int32

	Hosts []Host
}

func parseInterface(cfg cfgInterface, parent *Config) (*Interface, error) {
	var err error

	ifi := &Interface{
		Name:        cfg.Name,
		Description: cfg.Description,
		Debug:       cfg.Debug,

		Table:      0,
		UpAction:   parent.UpAction,
		DownAction: parent.DownAction,

		MinimumUp: DEF_MINIMUMUP,

		Hosts: make([]Host, 0, len(cfg.Hosts)),
	}

	if cfg.Table != nil {
		if *cfg.Table < 1 || *cfg.Table > TABLE_MAX {
			return nil, fmt.Errorf("table is incorrect: %d, should be between %d and %d", *cfg.Table, 1, TABLE_MAX)
		}
		if *cfg.Table == unix.RT_TABLE_LOCAL || *cfg.Table == unix.RT_TABLE_MAIN {
			return nil, fmt.Errorf("table is invalid: %d, reserved table", *cfg.Table)
		}
		ifi.Table = uint32(*cfg.Table)
	}

	if cfg.Metric != nil {
		if *cfg.Metric < 1 || *cfg.Metric > 32764 {
			return nil, fmt.Errorf("metric is incorrect: %d, should be between %d and %d", *cfg.Metric, 1, 32764)
		}
		if ifi.Table == 0 { // route sync must be enabled
			return nil, fmt.Errorf("table is incorrect: must be set to non-zero for metric to work")
		}
		ifi.Metric = uint32(*cfg.Metric)
	}

	if cfg.MinimumUp != nil {
		if *cfg.MinimumUp > len(cfg.Hosts) || *cfg.MinimumUp < 1 {
			return nil, fmt.Errorf("minimum_up is incorrect: %d, should be between %d and %d", *cfg.MinimumUp, 1, len(cfg.Hosts))
		}
		ifi.MinimumUp = *cfg.MinimumUp
	}

	ifi.BurstSize = parent.BurstSize
	if cfg.BurstSize != nil {
		if *cfg.BurstSize < BURSTSIZE_MIN || *cfg.BurstSize > BURSTSIZE_MAX {
			return nil, fmt.Errorf("burst_size is incorrect: %d, should be between %d and %d", *cfg.BurstSize, BURSTSIZE_MIN, BURSTSIZE_MAX)
		}
		ifi.BurstSize = *cfg.BurstSize
	}

	if ifi.BurstInterval, err = parseDuration(cfg.BurstInterval, parent.BurstInterval); err != nil {
		return nil, err
	}
	if ifi.ICMPInterval, err = parseDuration(cfg.ICMPInterval, parent.ICMPInterval); err != nil {
		return nil, err
	}
	if ifi.ICMPTimeout, err = parseDuration(cfg.ICMPTimeout, parent.ICMPTimeout); err != nil {
		return nil, err
	}

	if cfg.UpAction != nil {
		ifi.UpAction = *cfg.UpAction
	}
	if cfg.DownAction != nil {
		ifi.DownAction = *cfg.DownAction
	}

	seen := make(map[string]bool)
	for i, h := range cfg.Hosts {
		host, err := parseHost(h, ifi)
		if err != nil {
			return nil, fmt.Errorf("host %d: %v", i, err)
		}

		if _, ok := seen[host.Host.String()]; ok {
			return nil, fmt.Errorf("host %d: %q cannot appear multiple times for interface %q", i, host.Host.String(), cfg.Name)
		}
		seen[host.Host.String()] = true

		ifi.Hosts = append(ifi.Hosts, *host)
		if host.Family == unix.AF_INET {
			//ifi.upHostsv4++
			ifi.totalHostsv4++
		}
		if host.Family == unix.AF_INET6 {
			//ifi.upHostsv6++
			ifi.totalHostsv6++
		}
	}

	return ifi, nil
}

func (i *Interface) LinkDown() {
	atomic.StoreInt32(&i.upHostsv4, 0)
	atomic.StoreInt32(&i.upHostsv6, 0)
}

func (i *Interface) HostDown(family uint8) bool {
	if family == unix.AF_INET {
		return i.hostDownv4()
	}
	return i.hostDownv6()
}

func (i *Interface) HostUp(family uint8) bool {
	if family == unix.AF_INET {
		return i.hostUpv4()
	}
	return i.hostUpv6()
}

func (i *Interface) hostDownv4() bool {
	up := atomic.AddInt32(&i.upHostsv4, -1)
	if up < 0 {
		atomic.StoreInt32(&i.upHostsv6, 0)
	}
	// only trigger monitor down if we are one below mimimum
	return up == int32(i.MinimumUp)-1
}

func (i *Interface) hostUpv4() bool {
	up := atomic.AddInt32(&i.upHostsv4, 1)
	if up > i.totalHostsv4 {
		atomic.StoreInt32(&i.upHostsv4, i.totalHostsv4)
	}
	// only trigger monitor up if we are at mimimum, not above
	return up == int32(i.MinimumUp)
}

func (i *Interface) hostDownv6() bool {
	up := atomic.AddInt32(&i.upHostsv6, -1)
	if up < 0 {
		atomic.StoreInt32(&i.upHostsv6, 0)
	}
	// only trigger monitor down if we are one below mimimum
	return up == int32(i.MinimumUp)-1
}

func (i *Interface) hostUpv6() bool {
	up := atomic.AddInt32(&i.upHostsv6, 1)
	if up > i.totalHostsv6 {
		atomic.StoreInt32(&i.upHostsv6, i.totalHostsv6)
	}
	// only trigger monitor up if we are at mimimum, not above
	return up == int32(i.MinimumUp)
}

func (i *Interface) Up4() int32 {
	return atomic.LoadInt32(&i.upHostsv4)
}

func (i *Interface) Up6() int32 {
	return atomic.LoadInt32(&i.upHostsv6)
}

func (i *Interface) Up(family uint8) int32 {
	if family == unix.AF_INET {
		return i.Up4()
	}
	return i.Up6()
}
