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
package icmp

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/jsimonetti/hodos/internal/log"
	ping "github.com/prometheus-community/pro-bing"
)

type Monitor struct {
	src       string
	dst       *net.IPAddr
	interFace string
	ctx       context.Context
	ctxCancel context.CancelFunc

	downFunc          func()
	upFunc            func()
	l                 log.Logger
	interval, timeout time.Duration
	burstsize         int

	wg *sync.WaitGroup
}

func New(ctx context.Context, src string, dst net.IP, ifi string, opts ...Option) (*Monitor, error) {
	m := &Monitor{
		src:       src,
		dst:       &net.IPAddr{IP: dst, Zone: ifi},
		interFace: ifi,

		downFunc:  func() {},
		upFunc:    func() {},
		l:         log.Default(),
		interval:  500 * time.Millisecond,
		timeout:   200 * time.Millisecond,
		burstsize: 3,
		wg:        &sync.WaitGroup{},
	}
	m.ctx, m.ctxCancel = context.WithCancel(ctx)

	for _, option := range opts {
		if err := option(m); err != nil {
			return nil, err
		}
	}

	if m.dst.IP.To4() == nil {
		m.src = m.src + "%" + m.interFace
	}

	return m, nil
}

func (m *Monitor) Up(upFunc func()) {
	m.upFunc = upFunc
}
func (m *Monitor) Down(downFunc func()) {
	m.downFunc = downFunc
}

type Option func(m *Monitor) error

// Interval is a functional Option to set
// an Interval for requests.
// Defaults to 500 miliseconds.
func Interval(t time.Duration) Option {
	return func(m *Monitor) error {
		m.interval = t
		return nil
	}
}

// Timeout is a functional Option to set
// a timeout for replies.
// Defaults to 200 miliseconds.
func Timeout(t time.Duration) Option {
	return func(m *Monitor) error {
		m.timeout = t
		return nil
	}
}

// BurstSize is a functional Option to set
// the count of packets to send in this burst
// Defaults to 3.
func BurstSize(s int) Option {
	return func(m *Monitor) error {
		m.burstsize = s
		return nil
	}
}

// Logger is a functional Option to set
// a new logger for this monitor
func Logger(l log.Logger) Option {
	return func(m *Monitor) error {
		m.l = l
		return nil
	}
}

func (m *Monitor) run() error {
	m.wg.Add(1)
	defer m.wg.Done()

	m.l.Debugf("starting monitor on %q for %s", m.interFace, m.dst.String())

	pinger := ping.New("")
	defer pinger.Stop()

	pinger.SetIPAddr(m.dst)
	pinger.Source = m.src
	pinger.Count = m.burstsize
	pinger.Timeout = time.Duration(m.burstsize) * (m.timeout + m.interval)
	pinger.Interval = m.interval
	pinger.SetPrivileged(true)

	pinger.OnRecv = func(pkt *ping.Packet) {
		m.l.Debugf("(%s) %d bytes from %s: icmp_seq=%d time=%v\n",
			m.interFace, pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt)
	}
	pinger.OnDuplicateRecv = func(pkt *ping.Packet) {
		m.l.Debugf("(%s) %d bytes from %s: icmp_seq=%d time=%v ttl=%v (DUP!)\n",
			m.interFace, pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt, pkt.TTL)
	}

	end := make(chan bool)
	pinger.OnFinish = func(stats *ping.Statistics) {
		m.l.Debugf("(%s) %d packets transmitted, %d packets received, %v%% packet loss\n",
			m.interFace, stats.PacketsSent, stats.PacketsRecv, stats.PacketLoss)
		m.l.Debugf("(%s) round-trip min/avg/max/stddev = %v/%v/%v/%v\n",
			m.interFace, stats.MinRtt, stats.AvgRtt, stats.MaxRtt, stats.StdDevRtt)
		close(end)
	}

	go func() {
		select {
		case <-m.ctx.Done():
			pinger.Stop()
		case <-end:
			return
		}
	}()

	if err := pinger.Run(); err != nil {
		return err
	}

	open := true
	select {
	case _, open = <-m.ctx.Done():
	default:
	}
	if open {
		stats := pinger.Statistics()
		if stats.PacketLoss > 75 {
			m.downFunc()
		} else {
			m.upFunc()
		}
	}
	m.l.Debugf("stopped monitor on %q for %s", m.interFace, m.dst.String())
	return nil
}

func (m *Monitor) Stop() {
	m.l.Debugf("stopping monitor on %q for %s", m.interFace, m.dst.String())
	m.ctxCancel()
	m.wg.Wait()
}

func (m *Monitor) Start(burstInterval time.Duration) {
	timer := time.NewTicker(burstInterval)
	for {
		select {
		case <-m.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			m.run()
		}
	}
}
