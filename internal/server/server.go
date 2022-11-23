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
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/jsimonetti/hodos/internal/config"
	"github.com/jsimonetti/hodos/internal/icmp"
	"github.com/jsimonetti/hodos/internal/linkstate"
	"github.com/jsimonetti/hodos/internal/log"
	"github.com/jsimonetti/hodos/internal/routesync"
	"github.com/jsimonetti/rtnetlink"
	"github.com/mdlayher/netlink"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
)

type Server struct {
	config *config.Config

	l         log.Logger
	ctx       context.Context
	ctxCancel context.CancelFunc

	linkMonitors map[string]*linkstate.Monitor
	routeSync    map[string]*routesync.Sync
	icmpMonitors map[string]map[string]*icmp.Monitor

	pid    uint32
	nlconn *rtnetlink.Conn // We need to open the first netlink conn to force our PID
}

func New(ctx context.Context, l log.Logger, config *config.Config) (*Server, error) {
	var err error
	s := &Server{
		config:       config,
		l:            l,
		linkMonitors: make(map[string]*linkstate.Monitor),
		routeSync:    make(map[string]*routesync.Sync),
		icmpMonitors: make(map[string]map[string]*icmp.Monitor),

		pid: uint32(os.Getpid()),
	}
	s.ctx, s.ctxCancel = context.WithCancel(ctx)

	// we force the kernel to assign our pid
	// we need this to be able to distinguish external netlink
	// from our own (we want to ignore our own)
	s.nlconn, err = rtnetlink.Dial(&netlink.Config{PID: s.pid})
	if err != nil {
		return nil, err
	}

	// set up a monitoring
	for _, ifi := range s.config.Interfaces {
		if err := s.addLinkMonitor(ifi); err != nil {
			return nil, err
		}

		if ifi.Table != 0 { // only do table sync if we use a table
			if err := s.addRouteSync(ifi); err != nil {
				return nil, err
			}
			routesync.WithMetric(ifi.Metric)(s.routeSync[ifi.Name])
		}
	}
	return s, nil
}

func (s *Server) Start() error {
	// Wait for signals to shut down the server.
	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		s.run()
	}()

	// loop until we should stop or get a signal
	for {
		select {
		case <-s.ctx.Done():
			return s.Stop()
		case sig := <-sigC:
			s.l.Printf("Server: terminating due to signal %s, cleaning up...\n", sig)
			return s.Stop()
		}
	}
}

func (s *Server) Stop() error {
	// remove routes
	//	s.l.Debugf("Server: removing temporary routes")
	//	for _, ifi := range s.config.Interfaces {
	//		s.failGatewaysFor(&ifi, unix.AF_INET)
	//		s.failGatewaysFor(&ifi, unix.AF_INET6)
	//	}

	s.l.Debugf("Server: tearing down icmp monitors")
	// tear down monitoring
	for ifi := range s.icmpMonitors {
		for _, m := range s.icmpMonitors[ifi] {
			m.Stop()
		}
	}
	s.l.Debugf("Server: tearing down link monitors")
	for _, m := range s.linkMonitors {
		m.Stop()
	}
	// if no interface has a non-zero table configured,
	// route table sync is not running
	if len(s.routeSync) > 0 {
		s.l.Debugf("Server: tearing down route table sync")
		for _, m := range s.routeSync {
			m.Stop()
		}
	}
	defer s.ctxCancel()
	return nil
}

func (s *Server) run() error {
	errGroup, _ := errgroup.WithContext(s.ctx)

	// set up a monitoring
	s.l.Debugf("Server: starting link monitors")
	for _, m := range s.linkMonitors {
		errGroup.Go(m.Run)
	}

	// if no interface has a non-zero table configured,
	// route table sync is not running
	if len(s.routeSync) > 0 {
		s.l.Debugf("Server: starting route table sync")
		for _, m := range s.routeSync {
			errGroup.Go(m.Run)
		}
	}

	return errGroup.Wait()
}

func (s *Server) ruleAdd(from *net.IPNet, to *net.IPNet, table uint32, priority uint32, family uint8) error {
	if from == nil {
		return fmt.Errorf("ipRuleDo: from ip is nil")
	}

	srcSize, _ := from.Mask.Size()
	dstSize, _ := to.Mask.Size()

	msg := &rtnetlink.RuleMessage{
		Family:    family,
		SrcLength: uint8(srcSize),
		DstLength: uint8(dstSize),
		Action:    unix.FR_ACT_TO_TBL,
		Attributes: &rtnetlink.RuleAttributes{
			Src:      netIPPtr(from.IP),
			Dst:      netIPPtr(to.IP),
			Table:    &table,
			Priority: &priority,
		},
	}
	if from.IP.To4() == nil {
		msg.Family = unix.AF_INET6
	}

	return s.nlconn.Rule.Add(msg)
}

func netIPPtr(v net.IP) *net.IP {
	if ip4 := v.To4(); ip4 != nil {
		// By default net.IP returns the 16 byte representation.
		// But netlink requires us to provide only four bytes
		// for legacy IPs.
		return &ip4
	}
	return &v
}
