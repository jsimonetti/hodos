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
package server

import (
	"github.com/jsimonetti/hodos/internal/config"
	"github.com/jsimonetti/hodos/internal/icmp"
)

func (s *Server) addICMPMonitor(ifi *config.Interface, src string, host config.Host) error {
	s.l.Debugf("addICMPMonitor: add monitor on interface %q for host %+v", ifi.Name, host)
	m, err := icmp.New(s.ctx, src, *host.Host, ifi.Name, icmp.Logger(s.l),
		icmp.Interval(host.ICMPInterval),
		icmp.Timeout(host.ICMPTimeout),
		icmp.BurstSize(host.BurstSize))

	if err != nil {
		return err
	}

	isUp := false
	m.Down(func() {
		// debounce down
		if isUp {
			s.nextHopFail(ifi, host.Family, false)
			isUp = false
		}
	})
	m.Up(func() {
		// debounce up
		if !isUp {
			s.nextHopAvailable(ifi, host.Family)
			isUp = true
		}
	})
	s.icmpMonitors[ifi.Name][host.Name] = m

	go m.Start(host.BurstInterval)

	return nil
}
