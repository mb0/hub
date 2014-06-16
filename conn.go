// Copyright 2014 Martin Schnabel. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hub

import (
	"hash/fnv"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pingPeriod = 60 * time.Second
)

type conn struct {
	id     uint64
	send   chan *Msg
	wconn  *websocket.Conn
	ticker *time.Ticker
}

func newconn(w http.ResponseWriter, r *http.Request) (*conn, error) {
	wconn, err := websocket.Upgrade(w, r, nil, 1024, 1024)
	if err != nil {
		return nil, err
	}
	hash := fnv.New32()
	hash.Write([]byte(r.RemoteAddr))
	id := uint64(hash.Sum32())
	return &conn{id, make(chan *Msg, 64), wconn, time.NewTicker(pingPeriod)}, nil
}

func (c *conn) read(h *Hub) {
	for {
		op, r, err := c.wconn.NextReader()
		if err != nil {
			if err != io.EOF && err != io.ErrUnexpectedEOF {
				log.Printf("error receiving message %+v\n", err)
			}
			return
		}
		if op == websocket.BinaryMessage {
			return
		}
		if op != websocket.TextMessage {
			continue
		}
		bytes, err := ioutil.ReadAll(r)
		if err != nil {
			log.Println("error receiving message", err)
			return
		}
		colon := 0
		for i, b := range bytes {
			if b == ':' {
				colon = i
				break
			}
		}
		h.Route <- &Msg{c.id, RouteID, string(bytes[:colon]), bytes[colon+1:]}
	}
}

func (c *conn) write() {
	for {
		select {
		case msg, ok := <-c.send:
			c.wconn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.wconn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.wconn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write([]byte(msg.Head))
			w.Write([]byte{':'})
			_, err = w.Write(msg.Data)
			if err != nil {
				log.Println("error encoding message", err)
			}
			err = w.Close()
			if err != nil {
				log.Println("error sending message", err)
				return
			}
		case now := <-c.ticker.C:
			c.wconn.SetWriteDeadline(now.Add(writeWait))
			err := c.wconn.WriteMessage(websocket.PingMessage, []byte{})
			if err != nil {
				log.Println("error sending ping", err)
				return
			}
		}
	}
}

func (c *conn) close() {
	c.ticker.Stop()
	c.wconn.Close()
}
