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
	"fmt"
	"net"
	"os/exec"

	"github.com/jsimonetti/hodos/internal/config"
	"golang.org/x/sys/unix"
)

func (s *Server) nextHopFailLink(ifi *config.Interface) {
	s.nextHopFail(ifi, unix.AF_INET, true)
	s.nextHopFail(ifi, unix.AF_INET6, true)
}

func (s *Server) nextHopFail(ifi *config.Interface, family uint8, linkDown bool) {
	var belowMinimum bool
	if linkDown {
		ifi.LinkDown()
		s.l.Debugf("linkDown: interface %v", ifi)
	} else {
		belowMinimum = ifi.HostDown(family)
		s.l.Printf("hostDown: family %s, interface %s, up %d/%d, below: %t", fam(family), ifi.Name, ifi.Up(family), ifi.MinimumUp, belowMinimum)
	}
	if linkDown || belowMinimum {
		s.l.Printf("nextHopFail: family %s, interface %q", fam(family), ifi.Name)
		out, err := s.execScript("DOWN", family, ifi)
		if err != nil {
			s.l.Printf("nextHopFail: could not run down_action: %s", err)
		}
		if len(out) > 0 {
			s.l.Printf(">>> %q", string(out))
		}

		// delete all gateway routes from main for this interface
		s.deleteGatewaysFor(ifi, family)
	}
}

func (s *Server) nextHopAvailable(ifi *config.Interface, family uint8) {
	atMinimum := ifi.HostUp(family)
	s.l.Printf("hostUp: family %s, interface %s, up %d/%d, at: %t", fam(family), ifi.Name, ifi.Up(family), ifi.MinimumUp, atMinimum)
	if atMinimum {
		s.l.Printf("nextHopAvailable: family %s, interface %q", fam(family), ifi.Name)
		out, err := s.execScript("UP", family, ifi)
		if err != nil {
			s.l.Printf("nextHopAvailable: could not run up_action: %s", err)
		}
		if len(out) > 0 {
			s.l.Printf(">>> %q", string(out))
		}

		// copy all gateway routes from interface table to main and modify
		// route priority to set metric
		s.addGatewaysFor(ifi, family)
	}
}

func (s *Server) addGatewaysFor(ifi *config.Interface, family uint8) error {
	ifIndex, err := net.InterfaceByName(ifi.Name)
	if err != nil {
		return err
	}
	msgs, err := s.nlconn.Route.List()
	if err != nil {
		return err
	}

	for _, msg := range msgs {
		if msg.Attributes.Table == ifi.Table && msg.Family == family && msg.Attributes.OutIface == uint32(ifIndex.Index) && msg.Attributes.Gateway != nil {
			// Add this route to the main table
			msg.Table = unix.RT_TABLE_MAIN
			msg.Attributes.Table = unix.RT_TABLE_MAIN
			msg.Attributes.Priority = ifi.Metric
			if err := s.nlconn.Route.Replace(&msg); err != nil {
				s.l.Printf("error adding gateway route %+v: %s", msg, err)
			}
		}
	}
	return nil
}

func (s *Server) deleteGatewaysFor(ifi *config.Interface, family uint8) error {
	ifIndex, err := net.InterfaceByName(ifi.Name)
	if err != nil {
		return err
	}
	msgs, err := s.nlconn.Route.List()
	if err != nil {
		return err
	}

	for _, msg := range msgs {
		if msg.Attributes.Table == unix.RT_TABLE_MAIN && msg.Family == family && msg.Attributes.OutIface == uint32(ifIndex.Index) &&
			msg.Attributes.Gateway != nil {
			s.nlconn.Route.Delete(&msg)
		}
	}
	return nil
}

func (s *Server) execScript(event string, family uint8, ifi *config.Interface) ([]byte, error) {
	script := ifi.UpAction
	if event == "DOWN" {
		script = ifi.DownAction
	}
	if script == "" {
		return nil, nil
	}
	cmd := exec.CommandContext(s.ctx, "/run/current-system/sw/bin/env", "sh", "-c", "'"+script+"'")
	cmd.Env = []string{"EVENT=" + event, "FAMILY=" + fam(family)}
	cmd.Env = append(cmd.Env, ifiToEnv(ifi)...)

	return cmd.CombinedOutput()
}

func ifiToEnv(ifi *config.Interface) []string {
	return []string{
		"NAME=" + ifi.Name,
		"DESCRIPTION='" + ifi.Description + "'",
		"TABLE=" + fmt.Sprintf("%d", ifi.Table),
		"UP_HOSTS4=" + fmt.Sprintf("%d", ifi.Up4()),
		"UP_HOSTS6=" + fmt.Sprintf("%d", ifi.Up6()),
		"MINIMUM_UP=" + fmt.Sprintf("%d", ifi.MinimumUp),
	}
}

func fam(family uint8) string {
	switch family {
	case unix.AF_INET6:
		return "IPv6"
	case unix.AF_INET:
		return "IPv4"
	default:
		return "UNKNOWN"
	}
}
