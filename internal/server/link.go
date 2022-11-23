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
	"time"

	"github.com/jsimonetti/hodos/internal/config"
	"github.com/jsimonetti/hodos/internal/icmp"
	"github.com/jsimonetti/hodos/internal/linkstate"
	"github.com/jsimonetti/rtnetlink"
	"golang.org/x/sys/unix"
)

func (s *Server) linkDown(ifi *config.Interface) {
	s.l.Debugf("linkDown event: %q (%p)", ifi.Name, ifi)
	for _, m := range s.icmpMonitors[ifi.Name] {
		m.Stop()
	}

	if ifi.Table != 0 {
		// remove route rules
		nl, err := rtnetlink.Dial(nil)
		if err != nil {
			s.l.Printf("linkDown: could not dial rtnetlink: %s", err)
		}
		defer nl.Close()

		// first get all rules for this table
		// benefit of this, it works for all address families
		rumsgs, _ := nl.Rule.List()
		for _, msg := range rumsgs {
			if *msg.Attributes.Table == ifi.Table {
				if err = nl.Rule.Delete(&msg); err != nil {
					s.l.Printf("linkDown: error deleting route rules for table %d: %s", ifi.Table, err)
				}
			}
		}
	}

	s.nextHopFailLink(ifi)
}

func (s *Server) linkUp(ifi *config.Interface, shutdown chan bool) {
	s.l.Debugf("linkUp event: %q (%p)", ifi.Name, ifi)

	hasipv6 := false
	hasipv4 := false

	for _, host := range ifi.Hosts {
		if !hasipv4 && host.Family == unix.AF_INET {
			s.l.Debugf("linkUp: enabling IPv4 support for interface %q", ifi.Name)
			hasipv4 = true
			continue
		}
		if !hasipv6 && host.Family == unix.AF_INET6 {
			s.l.Debugf("linkUp: enabling IPv6 support for interface %q", ifi.Name)
			hasipv6 = true
		}
	}

	// we need to wait untill we have a valid ip address
	// on the interface before we can start an icmp monitor
	go func() {
		backoff4 := time.Duration(1)
		backoff6 := time.Duration(1)
		// 1 second seems like a decent enough time to wait
		timer4 := time.NewTicker(backoff4 * time.Second)
		timer6 := time.NewTicker(backoff6 * time.Second)
		if !hasipv4 {
			timer4.Stop()
		}
		if !hasipv6 {
			timer6.Stop()
		}

		for {
			select {
			case <-timer4.C:
				// not found, backoff?
				if backoff4.Seconds() < 32 {
					backoff4 = 2 * backoff4
				}
				timer4.Reset(backoff4 * time.Second)
				// try to find an ip address and start the monitor
				s.l.Debugf("linkUp: trying to find an ipv4 address on interface %q", ifi.Name)
				if src := findLocalAddressv4(ifi.Name); src != "" {
					s.l.Printf("linkUp: using IPv4 source %q for interface %q", src, ifi.Name)
					timer4.Stop()
					if hasipv4 {
						for _, host := range ifi.Hosts {
							if host.Family == unix.AF_INET {
								if ifi.Table != 0 {
									_, from, _ := net.ParseCIDR(fmt.Sprintf("%s/%d", src, 32))
									_, to, _ := net.ParseCIDR(fmt.Sprintf("%s/%d", host.Host, 32))
									if err := s.ruleAdd(from, to, ifi.Table, 1, unix.AF_INET); err != nil {
										s.l.Printf("linkUp: could not add route rule %q: %q-> (%q)", ifi.Name, from, to, err)
									}
								}
								if err := s.addICMPMonitor(ifi, src, host); err != nil {
									s.l.Printf("linkUp: could not start icmp monitor %q: %q -> %d (%q)", ifi.Name, src, host.Name, err)
								}
							}
						}
						// we start with everything down
						s.failGatewaysFor(ifi, unix.AF_INET)
					}
				}
			case <-timer6.C:
				// not found, backoff?
				if backoff6.Seconds() < 30 {
					backoff6 = 2 * backoff6
				}
				timer6.Reset(backoff6 * time.Second)
				// try to find an ip address and start the monitor
				s.l.Debugf("linkUp: trying to find an ipv6 address on interface %q", ifi.Name)
				if src := findLocalAddressv6(ifi.Name); src != "" {
					s.l.Printf("linkUp: using IPv6 source %q for interface %q", src, ifi.Name)
					timer6.Stop()
					if hasipv4 {
						for _, host := range ifi.Hosts {
							if host.Family == unix.AF_INET6 {
								if ifi.Table != 0 {
									_, from, _ := net.ParseCIDR(fmt.Sprintf("%s/%d", src, 128))
									_, to, _ := net.ParseCIDR(fmt.Sprintf("%s/%d", host.Host, 128))
									if err := s.ruleAdd(from, to, ifi.Table, 1, unix.AF_INET6); err != nil {
										s.l.Printf("linkUp: could not add route rule %q: %q-> (%q)", ifi.Name, from, to, err)
									}
								}
								if err := s.addICMPMonitor(ifi, src, host); err != nil {
									s.l.Printf("linkUp: could not start icmp monitor %q: %q -> %d (%q)", ifi.Name, src, host.Name, err)
								}
							}
						}
						// we start with everything down
						s.failGatewaysFor(ifi, unix.AF_INET6)
					}
				}
			case <-shutdown:
				// called as a safe measure to prevent stale monitors to startup
				return
			}
		}
	}()
}

func (s *Server) addLinkMonitor(ifi config.Interface) error {
	s.l.Debugf("addLinkMonitor: add monitor for interface %q", ifi.Name)
	m, err := linkstate.New(s.ctx, ifi, linkstate.Logger(s.l))
	if err != nil {
		return err
	}
	shutdown := make(chan bool)
	m.Down(func() {
		s.linkDown(&ifi)
		close(shutdown)
	})
	m.Up(func() {
		shutdown = make(chan bool)
		s.linkUp(&ifi, shutdown)
	})
	s.linkMonitors[ifi.Name] = m
	s.icmpMonitors[ifi.Name] = make(map[string]*icmp.Monitor)
	return nil
}

func findLocalAddressv4(interfaceName string) string {
	if ifi, err := net.InterfaceByName(interfaceName); err == nil { // get interface
		if addrs, err := ifi.Addrs(); err == nil { // get addresses
			for _, addr := range addrs { // get ipv4 address
				if ipv4Addr := addr.(*net.IPNet).IP.To4(); ipv4Addr != nil {
					return ipv4Addr.String()
				}
			}
		}
	}
	return ""
}

func findLocalAddressv6(interfaceName string) string {
	if ifi, err := net.InterfaceByName(interfaceName); err == nil { // get interface
		if addrs, err := ifi.Addrs(); err == nil { // get addresses
			for _, addr := range addrs { // get ipv4 address
				if ipv6Addr := addr.(*net.IPNet).IP.To4(); ipv6Addr == nil {
					if ipv6Addr := addr.(*net.IPNet).IP; ipv6Addr.IsGlobalUnicast() {
						return ipv6Addr.String()
					}
				}
			}
		}
	}
	return ""
}
