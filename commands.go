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
import "time"
import "strconv"
import "crypto/rand"

import "github.com/gorilla/websocket"

func helpRecover(conn *websocket.Conn, sid int) {
	GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, ":recover"})
}

func cmdRecover(conn *websocket.Conn, usr *MonitorUser, args []string, sid int) {
	GlobalConfig.RLock()
	sc, ok := GlobalConfig.LaunchedHandlers[sid]
	GlobalConfig.RUnlock()
	if !ok {
		GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), ErrorClass, "Could not recover server, invalid SID."})
		return
	}
	ok = sc.Start()
	if !ok {
		GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), ErrorClass, "Could not recover server, server not down."})
		return
	}
}

func helpServer(conn *websocket.Conn, sid int) {
	GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, ":server create \"<name>\" [stable|unstable|<x.x.x.x>]"})
	GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, ":server update  [<x.x.x.x>]"})
	GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, ":server rename \"<name>\""})
	//GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, ":server delete"})
}

func cmdServer(conn *websocket.Conn, usr *MonitorUser, args []string, sid int) {
	if len(args) < 2 {
		helpServer(conn, sid)
		return
	}
	switch args[1] {
	case "create":
		if len(args) < 3 {
			helpServer(conn, sid)
			return
		}
		version := "stable"
		if len(args) >= 4 {
			version = args[3]
		}
		nsid := 0
		var err error
		switch version {
		case "stable":
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Installing server..."})
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Please wait, the monitor may need to download files."})
			nsid, err = GlobalConfig.InstallNewServer(true, args[2])
			if err != nil {
				GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), ErrorClass, err.Error()})
				return
			}
		case "unstable":
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Installing server..."})
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Please wait, the monitor may need to download files."})
			nsid, err = GlobalConfig.InstallNewServer(false, args[2])
			if err != nil {
				GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), ErrorClass, err.Error()})
				return
			}
		default:
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Installing server..."})
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Please wait, the monitor may need to download files."})
			nsid, err = GlobalConfig.InstallNewServerVersion(version, args[2])
			if err != nil {
				GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), ErrorClass, err.Error()})
				return
			}
		}
		GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Server installed."})
		GlobalSockets.Broadcast(&LogMessage{nsid, time.Now(), InitClass, args[2]})
		GlobalSockets.Broadcast(&LogMessage{nsid, time.Now(), MonitorClass, "Use :recover to start server."})
		GlobalConfig.Lock()
		GlobalConfig.LaunchedHandlers[nsid] = GlobalConfig.NewServerController(nsid)
		GlobalConfig.Unlock()
		GlobalConfig.Dump()
	case "update":
		GlobalConfig.RLock()
		sc, ok := GlobalConfig.Servers[sid]
		GlobalConfig.RUnlock()
		if !ok {
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), ErrorClass, "Could not update server, invalid SID."})
			return
		}
		version := ""
		if len(args) >= 3 {
			version = args[2]
		}
		if version == "" {
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Updating server..."})
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Please wait, the monitor may need to download files."})
			err := GlobalConfig.UpdateServer(sid)
			if err != nil {
				GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), ErrorClass, err.Error()})
				return
			}
		} else {
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Updating server..."})
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Please wait, the monitor may need to download files."})
			err := GlobalConfig.UpdateServerTo(version, sid)
			if err != nil {
				GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), ErrorClass, err.Error()})
				return
			}
		}
		sc.RLock()
		version = sc.Version
		sc.RUnlock()
		GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Server updated to " + version})
		GlobalConfig.Dump()
	case "rename":
		if len(args) < 3 {
			helpServer(conn, sid)
			return
		}
		GlobalConfig.RLock()
		sc, ok := GlobalConfig.Servers[sid]
		if !ok {
			GlobalConfig.RUnlock()
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), ErrorClass, "Could not update server, invalid SID."})
			return
		}
		sc.Lock()
		oldname := sc.Name
		sc.Name = args[2]
		err := os.Rename(fmt.Sprintf("%v/%v %v", GlobalConfig.DataDir, oldname, sid), fmt.Sprintf("%v/%v %v", GlobalConfig.DataDir, sc.Name, sid))
		sc.Unlock()
		GlobalConfig.RUnlock()
		if err != nil {
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), ErrorClass, "Could not move server config directory: " + err.Error()})
			return
		}
	case "delete":
		// TODO: Cannot cleanly shutdown halted servers right now.
		fallthrough
	default:
		helpServer(conn, sid)
	}
}

func helpKill(conn *websocket.Conn, sid int) {
	GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, ":kill (monitor|server)"})
}

func cmdKill(conn *websocket.Conn, usr *MonitorUser, args []string, sid int) {
	if len(args) < 2 {
		helpKill(conn, sid)
		return
	}
	switch args[1] {
	case "monitor":
		if !usr.IsAdmin {
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Exiting the monitor is a admin only action."})
			return
		}
		// TODO: Require the user to run the command twice within a time limit to confirm.
		GlobalConfig.Dump() // Just in case.
		os.Exit(0)
	case "server":
		GlobalConfig.RLock()
		sc, ok := GlobalConfig.LaunchedHandlers[sid]
		GlobalConfig.RUnlock()
		if !ok {
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), ErrorClass, "Could not kill server, invalid SID."})
			return
		}
		ok = sc.Kill()
		if !ok {
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), ErrorClass, "Could not kill server, server not up."})
			return
		}
	default:
		helpKill(conn, sid)
	}
}

func helpUser(conn *websocket.Conn, sid int) {
	GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, ":user (create|delete) \"<name>\""})
	GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, ":user authorize \"<name>\" [<sid>|admin]"})
}

func cmdUser(conn *websocket.Conn, usr *MonitorUser, args []string, sid int) {
	if !usr.IsAdmin {
		GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Managing users is a admin only action."})
		return
	}
	if len(args) < 3 {
		helpUser(conn, sid)
		return
	}
	switch args[1] {
	case "create":
		GlobalConfig.RLock()
		ok := true
		for _, u := range GlobalConfig.Tokens {
			if u.Name == args[2] {
				ok = false
				break
			}
		}
		GlobalConfig.RUnlock()
		if !ok {
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), ErrorClass, "A user with that name already exists."})
			return
		}

		b := make([]byte, 16)
		_, err := rand.Read(b)
		if err != nil {
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), ErrorClass, "Error creating user token."})
			return
		}
		token := fmt.Sprintf("%X", b)

		GlobalConfig.Lock()
		hastokens := len(GlobalConfig.Tokens) > 0
		GlobalConfig.Tokens[token] = &MonitorUser{
			Name:    args[2],
			IsAdmin: !hastokens,
			Servers: make(map[int]bool),
		}
		GlobalConfig.Unlock()
		GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "New user created! Token: " + token})
		GlobalConfig.Dump()
	case "delete":
		GlobalConfig.RLock()
		for tkn, u := range GlobalConfig.Tokens {
			if u.Name == args[2] {
				delete(GlobalConfig.Tokens, tkn)
				break
			}
		}
		GlobalConfig.RUnlock()
		GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "User deleted."})
		GlobalConfig.Dump()
	case "authorize":
		GlobalConfig.RLock()
		var cusr *MonitorUser
		for _, u := range GlobalConfig.Tokens {
			if u.Name == args[2] {
				cusr = u
				break
			}
		}
		if cusr == nil {
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Could not authorize, user not found."})
			GlobalConfig.RUnlock()
			return
		}
		if len(args) >= 4 {
			if args[3] == "admin" {
				cusr.IsAdmin = true
			} else {
				s, err := strconv.Atoi(args[3])
				if err != nil {
					GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Could not authorize, invalid SID."})
					GlobalConfig.RUnlock()
					return
				}
				cusr.Servers[s] = true
			}
		} else {
			cusr.Servers[sid] = true
		}
		GlobalConfig.RUnlock()
		GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "User authorized."})
		GlobalConfig.Dump()
	case "deauthorize":
		GlobalConfig.RLock()
		var cusr *MonitorUser
		for _, u := range GlobalConfig.Tokens {
			if u.Name == args[2] {
				cusr = u
				break
			}
		}
		if cusr == nil {
			GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Could not deauthorize, user not found."})
			GlobalConfig.RUnlock()
			return
		}
		if len(args) >= 4 {
			if args[3] == "admin" {
				cusr.IsAdmin = false
			} else {
				s, err := strconv.Atoi(args[3])
				if err != nil {
					GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "Could not deauthorize, invalid SID."})
					GlobalConfig.RUnlock()
					return
				}
				delete(cusr.Servers, s)
			}
		} else {
			delete(cusr.Servers, sid)
		}
		GlobalConfig.RUnlock()
		GlobalSockets.SendTo(conn, &LogMessage{sid, time.Now(), MonitorClass, "User deauthorized."})
		GlobalConfig.Dump()
	default:
		helpUser(conn, sid)
	}
}
