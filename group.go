// Copyright 2014 Martin Schnabel. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hub

import "sync"

type Group struct {
	ID  uint64
	Hub *Hub
	sync.Mutex
	conns []*conn
}

func (g *Group) Subscribe(id uint64) {
	g.Lock()
	defer g.Unlock()
	for _, conn := range g.conns {
		if conn.id == id {
			return
		}
	}
	g.Hub.RLock()
	c := g.Hub.idmap[id]
	g.Hub.RUnlock()
	if c == nil {
		return
	}
	c.Lock()
	c.groups = append(c.groups, g.ID)
	c.Unlock()
	g.conns = append(g.conns, c)
}

func (g *Group) Unsubscribe(id uint64) {
	g.Lock()
	defer g.Unlock()
	for i, c := range g.conns {
		if c.id != id {
			continue
		}
		g.conns[i] = g.conns[len(g.conns)-1]
		g.conns = g.conns[:len(g.conns)-1]
		c.Lock()
		for i, gid := range c.groups {
			if gid != g.ID {
				continue
			}
			c.groups[i] = c.groups[len(c.groups)-1]
			c.groups = c.groups[:len(c.groups)-1]
			break
		}
		c.Unlock()
		break
	}
}

func (g *Group) Send(head string, v interface{}) error {
	if g == nil {
		return nil
	}
	msg, err := NewMsg(g.ID, g.ID, head, v)
	if err != nil {
		return err
	}
	g.SendMsg(msg, 0)
	return nil
}

func (g *Group) SendMsg(msg *Msg, except uint64) {
	g.Lock()
	defer g.Unlock()
	for _, conn := range g.conns {
		if conn.id == except {
			continue
		}
		conn.send <- msg
	}
}
