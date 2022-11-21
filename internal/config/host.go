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
	"net"
	"time"

	"golang.org/x/sys/unix"
)

type Host struct {
	Name   string
	Host   *net.IP
	Debug  bool
	Family uint8

	BurstInterval time.Duration
	BurstSize     int
	ICMPInterval  time.Duration
	ICMPTimeout   time.Duration
}

func parseHost(cfg cfgHost, parent *Interface) (*Host, error) {
	var err error
	ip := net.ParseIP(cfg.Host)
	if ip == nil {
		return nil, fmt.Errorf("host ip address could not be parsed: %q, %q", cfg.Name, cfg.Host)
	}

	host := &Host{
		Name:  ip.String(),
		Host:  &ip,
		Debug: cfg.Debug,
	}

	if cfg.Name != "" {
		host.Name = cfg.Name
	}

	host.Family = unix.AF_INET
	if ip.To4() == nil {
		host.Family = unix.AF_INET6
	}

	host.BurstSize = parent.BurstSize
	if cfg.BurstSize != nil {
		if *cfg.BurstSize < BURSTSIZE_MIN || *cfg.BurstSize > BURSTSIZE_MAX {
			return nil, fmt.Errorf("burst_size is incorrect: %d, should be between %d and %d", *cfg.BurstSize, BURSTSIZE_MIN, BURSTSIZE_MAX)
		}
		host.BurstSize = *cfg.BurstSize
	}

	if host.BurstInterval, err = parseDuration(cfg.BurstInterval, parent.BurstInterval); err != nil {
		return nil, err
	}
	if host.ICMPInterval, err = parseDuration(cfg.ICMPInterval, parent.ICMPInterval); err != nil {
		return nil, err
	}
	if host.ICMPTimeout, err = parseDuration(cfg.ICMPTimeout, parent.ICMPTimeout); err != nil {
		return nil, err
	}

	return host, nil
}
