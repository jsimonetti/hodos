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
package linkstate

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/jsimonetti/hodos/internal/config"
	"github.com/jsimonetti/hodos/internal/log"
	"github.com/jsimonetti/rtnetlink"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

type Monitor struct {
	interFace config.Interface
	ctx       context.Context
	ctxCancel context.CancelFunc

	downFunc func()
	upFunc   func()
	isUp     bool
	l        log.Logger

	wg *sync.WaitGroup
}

func New(ctx context.Context, ifi config.Interface, opts ...Option) (*Monitor, error) {
	m := &Monitor{
		interFace: ifi,

		downFunc: func() {},
		upFunc:   func() {},
		l:        log.Default(),
		wg:       &sync.WaitGroup{},
	}
	m.ctx, m.ctxCancel = context.WithCancel(ctx)

	for _, option := range opts {
		if err := option(m); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// Up is used to add a callback that is run
// when this link becomes available
// It is generally used to start route and
// icmp monitors.
func (m *Monitor) Up(upFunc func()) {
	// debounce the up messages
	m.upFunc = func() {
		if !m.isUp {
			m.isUp = true
			upFunc()
		}
	}
}

// Down is used to add a callback that is run
// when this link becomes unavailable
// It is generally used to stop running route and
// icmp monitors.
func (m *Monitor) Down(downFunc func()) {
	// debounce the down messages
	m.downFunc = func() {
		if m.isUp {
			m.isUp = false
			downFunc()
		}
	}
}

// Option is a functional argument to *Monitor
type Option func(m *Monitor) error

// Logger is a functional Option to set
// a new logger for this monitor
func Logger(l log.Logger) Option {
	return func(m *Monitor) error {
		m.l = l
		return nil
	}
}

func (m *Monitor) Run() error {
	m.wg.Add(1)
	defer m.wg.Done()

	m.l.Debugf("interfaceMonitor: starting monitor on %q", m.interFace.Name)
	nl, err := rtnetlink.Dial(&netlink.Config{Groups: unix.RTNLGRP_LINK})
	if err != nil {
		m.l.Printf("interfaceMonitor: could not dial rtnetlink: %s", err)
		return err
	}
	defer nl.Close()
	defer m.l.Debugf("interfaceMonitor: ended for %q", m.interFace.Name)

	// bootstrap our state by getting all interfaces
	lreq := &rtnetlink.LinkMessage{}
	nl.Send(lreq, unix.RTM_GETLINK, netlink.Request|netlink.Dump)

	// endlessly loop
	for {
		nl.SetReadDeadline(time.Now().Add(1 * time.Second))
		select {
		case <-m.ctx.Done():
			// our caller has closed the context
			// so we stop monitoring
			return nil
		default:
			// receive all messages on the rtnetlink connection
			msgs, omsgs, err := nl.Receive()
			if err != nil {
				if e, ok := err.(net.Error); ok && e.Timeout() {
					continue
				}
				m.l.Printf("interfaceMonitor: receive error: %s", err)
			}

			// go over the messages
			for i, msg := range msgs {
				// check whether we got a LinkMessage
				if msg, ok := msg.(*rtnetlink.LinkMessage); ok {
					m.l.Debugf("interfaceMonitor: handling '%#v'", msg.Attributes)
					// if this message is not for the link we are
					// supposed to be monitoring, ignore it
					// TODO(jsi): add rtnetlink NLA_F_NESTED|IFLA_PROP_LIST to allow matching ALT_IF_NAME
					if msg.Attributes.Name != m.interFace.Name {
						continue
					}
					// for new links we need to decide the operational state
					// (links going down still result in a RTM_NEWLINK message)
					if omsgs[i].Header.Type == unix.RTM_NEWLINK {
						if msg.Attributes.OperationalState == rtnetlink.OperStateUp {
							m.upFunc()
						} else {
							m.downFunc()
						}
						m.l.Debugf("interfaceMonitor: netlink reports interface: %q (%d) (%q)\n", msg.Attributes.Name, msg.Index, msg.Attributes.OperationalState)
					}
					// for deleted links we only have to call the downFunc
					if omsgs[i].Header.Type == unix.RTM_DELLINK {
						m.downFunc()
						m.l.Debugf("interfaceMonitor: netlink reports deleted interface: %q (%d)\n", msg.Attributes.Name, msg.Index)
					}
				}
			}
		}
	}
}

func (m *Monitor) Stop() {
	m.l.Debugf("stopping monitor on %q", m.interFace)
	m.ctxCancel()
	m.wg.Wait()
}
