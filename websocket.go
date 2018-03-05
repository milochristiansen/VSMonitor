/*
Copyright 2018 by Milo Christiansen

This software is provided 'as-is', without any express or implied warranty. In
no event will the authors be held liable for any damages arising from the use of
this software.

Permission is granted to anyone to use this software for any purpose, including
commercial applications, and to alter it and redistribute it freely, subject to
the following restrictions:

1. The origin of this software must not be misrepresented; you must not claim
that you wrote the original software. If you use this software in a product, an
acknowledgment in the product documentation would be appreciated but is not
required.

2. Altered source versions must be plainly marked as such, and must not be
misrepresented as being the original software.

3. This notice may not be removed or altered from any source distribution.
*/

package main

import "fmt"
import "sync"
import "net/http"

import "github.com/gorilla/websocket"

var GlobalSockets = &Sockets{
	clients:  map[*websocket.Conn]bool{},
	Messages: make(chan string),
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,

	CheckOrigin: func(r *http.Request) bool { return true },
}

type Sockets struct {
	sync.Mutex

	// Used for broadcast.
	clients map[*websocket.Conn]bool

	Messages chan string
}

func (s *Sockets) Broadcast(msg []byte) {
	s.Lock()
	defer s.Unlock()

	for conn := range s.clients {
		err := conn.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			fmt.Println("Socket closed:", err)
			conn.Close()
			delete(s.clients, conn)
		}
	}
}

func (s *Sockets) Upgrade(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.Lock()
	s.clients[conn] = true
	s.Unlock()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("Socket closed:", err)
			s.Lock()
			delete(s.clients, conn)
			s.Unlock()
			break
		}

		// TODO: Strip off auth token once I add that.
		s.Messages <- string(msg)
	}
}
