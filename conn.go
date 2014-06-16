// Copyright 2014 Martin Schnabel. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hub

import (
	"encoding/json"
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
	id     Id
	send   chan Msg
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
	return &conn{Id(hash.Sum32()), make(chan Msg, 64), wconn, time.NewTicker(pingPeriod)}, nil
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
		var msg Msg
		err = json.Unmarshal(bytes, &msg)
		if err != nil {
			log.Println("error decoding message", err)
			return
		}
		h.Route <- &Envelope{c.id, Route, msg}
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
			enc := json.NewEncoder(w)
			err = enc.Encode(msg)
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
