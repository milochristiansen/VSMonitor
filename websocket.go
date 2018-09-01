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
import "time"
import "strings"
import "net/http"

import "github.com/gorilla/websocket"

var GlobalSockets = &Sockets{
	clients:  map[*websocket.Conn]bool{},
	Messages: make(chan *SocketMessage),
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

	Messages chan *SocketMessage
}

func (s *Sockets) Broadcast(msg *LogMessage) {
	s.Lock()
	defer s.Unlock()

	for conn := range s.clients {
		err := conn.WriteJSON(msg)
		if err != nil {
			fmt.Println("Socket closed:", err)
			conn.Close()
			delete(s.clients, conn)
		}
	}
}

func (s *Sockets) SendTo(conn *websocket.Conn, msg *LogMessage) {
	s.Lock()
	defer s.Unlock()

	err := conn.WriteJSON(msg)
	if err != nil {
		fmt.Println("Socket closed:", err)
		conn.Close()
		delete(s.clients, conn)
	}
}

type SocketMessage struct {
	SID     int
	Token   string
	Command string
}

func (s *Sockets) Upgrade(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send the monitor console activation packet.
	s.SendTo(conn, &LogMessage{0, time.Now(), InitClass, "Monitor"})

	// Before sending anything else, make sure this is an authorized user.
	msg := new(SocketMessage)
	err = conn.ReadJSON(&msg)
	if err != nil {
		fmt.Println("Socket closed:", err)
		conn.Close()
		return
	}
	GlobalConfig.RLock()
	hastokens := len(GlobalConfig.Tokens) > 0
	usr, ok := GlobalConfig.Tokens[msg.Token]
	GlobalConfig.RUnlock()
	if !hastokens {
		usr = &MonitorUser{
			Name:    "root",
			IsAdmin: true,
			Servers: make(map[int]bool),
		}
		s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "WARNING: There are no user accounts created yet! Create an account with the :user command."})
	} else if !ok {
		s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, "Invalid token."})
		return
	}

	// Send activation packets for each authorized server.
	GlobalConfig.RLock()
	t := time.Now()
	for sid := range GlobalConfig.LaunchedHandlers {
		if !usr.IsAdmin && !usr.Servers[sid] {
			continue
		}
		sinfo := GlobalConfig.Servers[sid]
		sinfo.RLock()
		s.SendTo(conn, &LogMessage{sid, t, InitClass, sinfo.Name})
		sinfo.RUnlock()
	}
	GlobalConfig.RUnlock()

	s.Lock()
	s.clients[conn] = true
	s.Unlock()

	for {
		msg := new(SocketMessage)
		err := conn.ReadJSON(&msg)
		if err != nil {
			fmt.Println("Socket closed:", err)
			conn.Close()
			s.Lock()
			delete(s.clients, conn)
			s.Unlock()
			break
		}

		// Validate token.
		GlobalConfig.RLock()
		hastokens := len(GlobalConfig.Tokens) > 0
		usr, ok := GlobalConfig.Tokens[msg.Token]
		GlobalConfig.RUnlock()
		if !hastokens {
			usr = &MonitorUser{
				Name:    "root",
				IsAdmin: true,
				Servers: make(map[int]bool),
			}
			s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "WARNING: There are no user accounts created yet! Create an account with :user."})
		} else if !ok {
			s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, "Invalid token."})
			continue
		}

		if !usr.IsAdmin && !usr.Servers[msg.SID] {
			s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, "You are not authorized to send messages to that server."})
			continue
		}

		if strings.HasPrefix(msg.Command, ":") {
			// It is a monitor command.
			parts := parseCommand([]byte(msg.Command))
			if len(parts) == 0 {
				// Basically impossible, or at least it should be.
				continue
			}
			switch parts[0] {
			case ":recover":
				cmdRecover(conn, usr, parts, msg.SID)
			case ":server":
				cmdServer(conn, usr, parts, msg.SID)
			case ":kill":
				cmdKill(conn, usr, parts, msg.SID)
			case ":user":
				cmdUser(conn, usr, parts, msg.SID)
			default:
				t := time.Now()
				s.SendTo(conn, &LogMessage{msg.SID, t, MonitorClass, ":recover"})
				s.SendTo(conn, &LogMessage{msg.SID, t, MonitorClass, ":server create \"name\" [stable|unstable|<x.x.x.x>]"})
				s.SendTo(conn, &LogMessage{msg.SID, t, MonitorClass, ":server update [<x.x.x.x>]"})
				//s.SendTo(conn, &LogMessage{msg.SID, t, MonitorClass, ":server delete"})
				s.SendTo(conn, &LogMessage{msg.SID, t, MonitorClass, ":kill (monitor|server)"})
				s.SendTo(conn, &LogMessage{msg.SID, t, MonitorClass, ":user (create|delete) \"<name>\""})
				s.SendTo(conn, &LogMessage{msg.SID, t, MonitorClass, ":user [authorize|deauthorize] \"<name>\" [<sid>|admin]"})
			}
			continue
		}

		GlobalConfig.RLock()
		sc, ok := GlobalConfig.LaunchedHandlers[msg.SID]
		GlobalConfig.RUnlock()
		if !ok {
			s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, "Could not send command to server, invalid SID."})
			continue
		}
		if !sc.Command(msg.Command) {
			s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, "Could not send command to server, server not up."})
			continue
		}
	}
}
