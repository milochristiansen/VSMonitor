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

import "os"
import "fmt"
import "sync"
import "time"
import "strings"
import "net/http"
import "crypto/rand"

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

	// Send activation packets for each defined server.
	GlobalConfig.RLock()
	t := time.Now()
	s.SendTo(conn, &LogMessage{0, t, InitClass, "Monitor"})
	for sid := range GlobalConfig.LaunchedHandlers {
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
			usr = MonitorUser{
				Name:    "root",
				IsAdmin: true,
			}
			s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "WARNING: There are no user accounts created yet! Create an account with :newuser."})
		} else if !ok {
			s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, "Invalid token."})
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
				GlobalConfig.RLock()
				sc, ok := GlobalConfig.LaunchedHandlers[msg.SID]
				GlobalConfig.RUnlock()
				if !ok {
					s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, "Could not recover server, invalid SID."})
					continue
				}
				ok = sc.Start()
				if !ok {
					s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, "Could not recover server, server not down."})
					continue
				}
			case ":server":
				if len(parts) < 2 {
					s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, ":server create \"name\" [stable|unstable|x.x.x.x]"})
					s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, ":server update  [x.x.x.x]"})
					//s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, ":server delete"})
					continue
				}
				switch parts[1] {
				case "create":
					if len(parts) < 3 {
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, ":server create \"name\" [stable|unstable|x.x.x.x]"})
						continue
					}
					version := "stable"
					if len(parts) >= 4 {
						version = parts[3]
					}
					nsid := 0
					var err error
					switch version {
					case "stable":
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "Installing server..."})
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "Please wait, the monitor may need to download files."})
						nsid, err = GlobalConfig.InstallNewServer(true, parts[2])
						if err != nil {
							s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, err.Error()})
							continue
						}
					case "unstable":
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "Installing server..."})
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "Please wait, the monitor may need to download files."})
						nsid, err = GlobalConfig.InstallNewServer(false, parts[2])
						if err != nil {
							s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, err.Error()})
							continue
						}
					default:
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "Installing server..."})
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "Please wait, the monitor may need to download files."})
						nsid, err = GlobalConfig.InstallNewServerVersion(GameVersion(version), parts[2])
						if err != nil {
							s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, err.Error()})
							continue
						}
					}
					s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "Server installed."})
					s.Broadcast(&LogMessage{nsid, time.Now(), InitClass, parts[2]})
					s.Broadcast(&LogMessage{nsid, time.Now(), MonitorClass, "Use :recover to start server."})
					GlobalConfig.Lock()
					GlobalConfig.LaunchedHandlers[nsid] = GlobalConfig.NewServerControler(nsid)
					GlobalConfig.Unlock()
					GlobalConfig.Dump()
					continue

				case "update":
					GlobalConfig.RLock()
					sc, ok := GlobalConfig.Servers[msg.SID]
					GlobalConfig.RUnlock()
					if !ok {
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, "Could not update server, invalid SID."})
						continue
					}
					version := ""
					if len(parts) >= 3 {
						version = parts[2]
					}
					if version == "" {
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "Updating server..."})
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "Please wait, the monitor may need to download files."})
						err := GlobalConfig.UpdateServer(msg.SID)
						if err != nil {
							s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, err.Error()})
							continue
						}
					} else {
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "Updating server..."})
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "Please wait, the monitor may need to download files."})
						err := GlobalConfig.UpdateServerTo(GameVersion(version), msg.SID)
						if err != nil {
							s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, err.Error()})
							continue
						}
					}
					sc.RLock()
					version = string(sc.Version)
					sc.RUnlock()
					s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "Server updated to " + version})
					GlobalConfig.Dump()
					continue

				//case "delete":
				// TODO: Cannot cleanly shutdown halted servers right now.
				default:
					s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, ":server create \"name\"  [stable|unstable|x.x.x.x]"})
					s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, ":server update  [x.x.x.x]"})
					//s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, ":server delete"})
				}
			case ":kill":
				if len(parts) < 2 {
					s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, ":kill (monitor|server)"})
					continue
				}
				switch parts[1] {
				case "monitor":
					if !usr.IsAdmin {
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "Exiting the monitor is a admin only action."})
						continue
					}
					// TODO: Require the user to run the command twice within a time limit to confirm.
					GlobalConfig.Dump() // Just in case.
					os.Exit(0)
				case "server":
					GlobalConfig.RLock()
					sc, ok := GlobalConfig.LaunchedHandlers[msg.SID]
					GlobalConfig.RUnlock()
					if !ok {
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, "Could not kill server, invalid SID."})
						continue
					}
					ok = sc.Kill()
					if !ok {
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, "Could not kill server, server not up."})
						continue
					}
				default:
					s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, ":kill (monitor|server)"})
				}
			case ":user":
				if !usr.IsAdmin {
					s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "Managing users is a admin only action."})
					continue
				}
				if len(parts) < 3 {
					s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, ":user (create|delete) \"name\""})
					continue
				}
				switch parts[1] {
				case "create":
					GlobalConfig.RLock()
					ok := true
					for _, u := range GlobalConfig.Tokens {
						if u.Name == parts[2] {
							ok = false
							break
						}
					}
					GlobalConfig.RUnlock()
					if !ok {
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, "A user with that name already exists."})
						continue
					}

					b := make([]byte, 16)
					_, err := rand.Read(b)
					if err != nil {
						s.SendTo(conn, &LogMessage{msg.SID, time.Now(), ErrorClass, "Error creating user token."})
						continue
					}
					token := fmt.Sprintf("%X", b)

					GlobalConfig.Lock()
					hastokens := len(GlobalConfig.Tokens) > 0
					GlobalConfig.Tokens[token] = MonitorUser{
						Name:    parts[2],
						IsAdmin: !hastokens,
					}
					GlobalConfig.Unlock()
					s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "New user created! Token: " + token})
					GlobalConfig.Dump()
				case "delete":
					GlobalConfig.RLock()
					for tkn, u := range GlobalConfig.Tokens {
						if u.Name == parts[2] {
							delete(GlobalConfig.Tokens, tkn)
							s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, "User deleted."})
							GlobalConfig.Dump()
							break
						}
					}
					GlobalConfig.RUnlock()
				default:
					s.SendTo(conn, &LogMessage{msg.SID, time.Now(), MonitorClass, ":user (create|delete) \"name\""})
				}
			default:
				t := time.Now()
				s.SendTo(conn, &LogMessage{msg.SID, t, MonitorClass, ":recover"})
				s.SendTo(conn, &LogMessage{msg.SID, t, MonitorClass, ":server create \"name\" [stable|unstable|x.x.x.x]"})
				s.SendTo(conn, &LogMessage{msg.SID, t, MonitorClass, ":server update [x.x.x.x]"})
				//s.SendTo(conn, &LogMessage{msg.SID, t, MonitorClass, ":server delete"})
				s.SendTo(conn, &LogMessage{msg.SID, t, MonitorClass, ":kill (monitor|server)"})
				s.SendTo(conn, &LogMessage{msg.SID, t, MonitorClass, ":user (create|delete) \"name\""})
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
