// Copyright 2014 Martin Schnabel. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hub

import (
	"encoding/json"
	"net/http"
	"sync"
)

const (
	RouteID  = 0
	GroupID  = 1 << 32
	ExceptID = 2 << 32
)

var (
	Signon  = "_signon"
	Signoff = "_signoff"
)

type Msg struct {
	From, To uint64
	Head     string
	Data     []byte
}

func NewMsg(from, to uint64, head string, v interface{}) (m *Msg, err error) {
	m = &Msg{from, to, head, nil}
	m.Data, err = json.Marshal(v)
	return
}

func (m *Msg) Reply(head string, v interface{}) (*Msg, error) {
	return NewMsg(m.To, m.From, head, v)
}

func (m *Msg) Unmarshal(v interface{}) error {
	return json.Unmarshal(m.Data, v)
}

type Group struct {
	ID uint64
	sync.Mutex
	Users []uint64
}

type Hub struct {
	sync.RWMutex
	conns  []*conn
	idmap  map[uint64]*conn
	groups map[uint64]*Group
	Route  chan *Msg
}

func New() *Hub {
	h := &Hub{
		idmap:  make(map[uint64]*conn),
		groups: make(map[uint64]*Group),
		Route:  make(chan *Msg, 64),
	}
	return h
}

func (h *Hub) signon(c *conn) {
	h.Lock()
	h.conns = append(h.conns, c)
	h.idmap[c.id] = c
	h.Unlock()
	h.Route <- &Msg{c.id, RouteID, Signon, nil}
}

func (h *Hub) signoff(c *conn) {
	h.Lock()
	for i, conn := range h.conns {
		if c != conn {
			continue
		}
		last := len(h.conns) - 1
		h.conns[i] = h.conns[last]
		h.conns = h.conns[:last]
	}
	delete(h.idmap, c.id)
	h.Unlock()
	c.close()
	close(c.send)
	h.Route <- &Msg{c.id, RouteID, Signoff, nil}
}

func (h *Hub) Group(id uint64) *Group {
	if id&GroupID == 0 || id^GroupID == 0 {
		return nil
	}
	h.Lock()
	g := h.groups[id]
	if g == nil {
		g = &Group{ID: id}
		h.groups[id] = g
	}
	h.Unlock()
	return g
}

func (h *Hub) Send(m *Msg) {
	var except uint64
	if m.To&ExceptID != 0 {
		m.To ^= ExceptID
		except = m.From
	}
	if m.To == RouteID {
		h.Route <- m
		return
	}
	h.RLock()
	switch {
	case m.To == GroupID:
		for _, c := range h.conns {
			if c.id == except {
				continue
			}
			c.send <- m
		}
	case m.To&GroupID != 0:
		g := h.groups[m.To]
		if g == nil {
			break
		}
		g.Lock()
		for _, id := range g.Users {
			if id == except {
				continue
			}
			if c, ok := h.idmap[id]; ok {
				c.send <- m
			}
		}
		g.Unlock()
	default:
		if c, ok := h.idmap[m.To]; ok {
			c.send <- m
		}
	}
	h.RUnlock()
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c, err := newconn(w, r)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	h.signon(c)
	go c.write()
	c.read(h)
	h.signoff(c)
}
