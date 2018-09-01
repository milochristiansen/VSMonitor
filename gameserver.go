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

import "io"
import "fmt"
import "time"
import "runtime"
import "os/exec"
import "sync/atomic"

const (
	countRestarts = 3
	loopThreshold = 60 * time.Second
)

type ServerController struct {
	c   *MonitorConfig
	sid int

	logs    chan *LogMessage
	cmds    chan string
	restart chan bool
	kill    chan bool
	i       chan io.WriteCloser
	o       chan io.ReadCloser

	isup    *int32 // Is the server running?
	isalive *int32 // Is the monitor loop running?
}

// NewServerController creates a new server control instance.
func (c *MonitorConfig) NewServerController(sid int) *ServerController {
	sc := &ServerController{
		c:       c,
		sid:     sid,
		logs:    make(chan *LogMessage, 16),
		cmds:    make(chan string),
		restart: make(chan bool),
		kill:    make(chan bool),
		i:       make(chan io.WriteCloser),
		o:       make(chan io.ReadCloser),
		isup:    new(int32),
		isalive: new(int32),
	}

	go sc.restartLoop()
	go sc.logParser()
	go sc.commandStuffer()
	go func() {
		for {
			log, ok := <-sc.logs
			if !ok {
				return
			}
			GlobalSockets.Broadcast(log)
		}
	}()

	return sc
}

// Kill orders the server to shutdown when up. Returns false if the server is not up.
func (sc *ServerController) Kill() bool {
	if atomic.LoadInt32(sc.isup) != 0 && atomic.LoadInt32(sc.isalive) != 0 {
		sc.kill <- true // You can send false to exit the controller loop too.
		return true
	}
	return false
}

// Start orders the server to start when down. Returns false if the server is not down.
func (sc *ServerController) Start() bool {
	if atomic.LoadInt32(sc.isup) == 0 && atomic.LoadInt32(sc.isalive) != 0 {
		sc.restart <- true
		return true
	}
	return false
}

// IsUp returns true if the server is running.
func (sc *ServerController) IsUp() bool {
	return atomic.LoadInt32(sc.isup) != 0
}

// IsAlive returns true if the monitor loop is running.
func (sc *ServerController) IsAlive() bool {
	return atomic.LoadInt32(sc.isalive) != 0
}

// Command sends a command to the server if it is running and returns true if the command was sent.
// WARNING: Just because this returns true does not mean the command was sent to the server, it just
// means it *probably* was sent.
func (sc *ServerController) Command(cmd string) bool {
	if atomic.LoadInt32(sc.isup) != 0 && atomic.LoadInt32(sc.isalive) != 0 {
		sc.cmds <- cmd
		return true
	}
	return false
}

// controller
func (sc *ServerController) log(f string, v ...interface{}) {
	sc.logs <- &LogMessage{sc.sid, time.Now(), MonitorClass, fmt.Sprintf(f, v...)}
}

func (sc *ServerController) restartLoop() {
	atomic.StoreInt32(sc.isalive, -1)

	var restarts [countRestarts]time.Time
	autostart := false

	for {
		if !autostart {
			atomic.StoreInt32(sc.isup, 0) // Alert the main system that the server is down and needs user intervention.
			sc.i <- nil                   // Stop IO.
			sc.o <- nil
			autostart = true
			<-sc.restart // Wait for the main system to reply with a :recover command.
		}

		if t := time.Since(restarts[0]); t < loopThreshold {
			sc.log("%d automatic server restarts in %v.", countRestarts, t)
			sc.log("Server is DOWN, awaiting :recover command.")
			autostart = false
			continue
		}

		copy(restarts[0:], restarts[1:])
		restarts[len(restarts)-1] = time.Now()

		sc.log("(re)starting server...")

		// Grab the paths needed:
		sc.c.RLock()
		sd, ok := sc.c.Servers[sc.sid]
		bin := sc.c.ServerDir
		dat := sc.c.DataDir
		sc.c.RUnlock()
		if !ok {
			sc.log("Fatal error, invalid SID (should be impossible).")
			atomic.StoreInt32(sc.isalive, 0)
			close(sc.i)
			close(sc.o)
			close(sc.logs)
			return
		}
		sd.RLock()
		ver := sd.Version
		name := sd.Name
		sid := sd.SID
		sd.RUnlock()
		sc.c.RLock()
		verinfo := sc.c.Versions[ver]
		sc.c.RUnlock()
		if verinfo != BinaryOK {
			err := sc.c.FindOrDownload(ver)
			sc.log("Could not restart server (downloading binaries): %v", err)
			sc.log("Server is DOWN, awaiting :recover command.")
			autostart = false
			continue
		}

		bin += fmt.Sprintf("/%v/VintagestoryServer.exe", ver)
		dat += fmt.Sprintf("/%v %v", name, sid)

		// And run the server!
		// HACK! But I'm feeling lazy, have pity on me.
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command(bin, "--dataPath", dat)
		} else {
			cmd = exec.Command("mono", bin, "--dataPath", dat)
		}

		ipipe, err := cmd.StdinPipe()
		if err != nil {
			sc.log("Could not restart server (attaching input pipe): %v", err)
			sc.log("Server is DOWN, awaiting :recover command.")
			autostart = false
			continue
		}

		opipe, err := cmd.StdoutPipe()
		if err != nil {
			sc.log("Could not restart server (attaching output pipe): %v", err)
			sc.log("Server is DOWN, awaiting :recover command.")
			autostart = false
			continue
		}

		err = cmd.Start()
		if err != nil {
			sc.log("Could not restart server: %v", err)
			sc.log("Server is DOWN, awaiting :recover command.")
			autostart = false
			continue
		}
		atomic.StoreInt32(sc.isup, -1) // Alert the system that the server is up.

		sc.i <- ipipe
		sc.o <- opipe

		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case keepgoing := <-sc.kill:
			if err := cmd.Process.Kill(); err != nil {
				sc.log("Failed to kill server: %v", err)
				sc.log("Server is ROGUE, run for your lives!") // lol
				autostart = false
				continue
			}
			sc.log("Server killed.")
			if !keepgoing {
				sc.log("Server is DOWN, and controller is exiting.")
				atomic.StoreInt32(sc.isalive, 0)
				close(sc.i)
				close(sc.o)
				close(sc.logs)
				return
			}
			sc.log("Server is DOWN, awaiting :recover command.")
			autostart = false
			continue
		case err := <-done:
			if err == nil {
				sc.log("Server exited intentionally.")
				sc.log("Server is DOWN, awaiting :recover command.")
				autostart = false
				continue
			}
			sc.log("Server died: %v", err)
			sc.log("Server is DOWN, awaiting :recover command.")
			autostart = true
		}
	}
}
