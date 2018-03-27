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
import "time"
import "bufio"
import "regexp"

func (sc *ServerControler) logTransmitter() {
	for {
		log, ok := <-sc.logs
		if !ok {
			return
		}
		GlobalSockets.Broadcast(log)
	}
}

func (sc *ServerControler) commandStuffer() {
	var wrt io.WriteCloser
	ok := false
	for {
		wrt = <-sc.i
		for {
			select {
			case wrt, ok = <-sc.i:
				if !ok {
					return
				}
				if wrt == nil {
					continue
				}
			case cmd := <-sc.cmds:
				if wrt != nil {
					_, _ = wrt.Write([]byte(cmd + "\n"))
				} else {
					sc.logs <- &LogMessage{sc.sid, time.Now(), ErrorClass, "Cannot send command, server is not running."}
				}
			}
		}
	}
}

const (
	MonitorClass = "Monitor"
	ErrorClass   = "Monitor Error"
	InitClass    = "Monitor Init"
)

type LogMessage struct {
	SID     int // Server ID
	At      time.Time
	Class   string
	Message string
}

//var loglineRe = regexp.MustCompile(`([0-9]+\.[0-9]+\.[0-9]+ [0-9]+:[0-9]+:[0-9]+) \[([a-zA-Z ]+)\] (.*)\n`)
var loglineRe = regexp.MustCompile(`([0-9]+:[0-9]+:[0-9]+) \[([a-zA-Z ]+)\] (.*)\n`)

func (sc *ServerControler) logParser() {
	var rdr io.ReadCloser
	ok := false
	last := time.Now()
	lastClass := MonitorClass
	for {
		rdr, ok = <-sc.o
		if !ok {
			return
		}
		if rdr == nil {
			continue
		}

		brdr := bufio.NewReader(rdr)

		for {
			line, err := brdr.ReadString('\n')
			if err == io.EOF {
				break
			} else if err != nil {
				sc.logs <- &LogMessage{sc.sid, time.Now(), ErrorClass, err.Error()}
			}

			// Try to parse the line. Failure is possible.
			// For now we will use the lazy way and split it up with a regex.
			matches := loglineRe.FindStringSubmatch(line)
			if matches == nil {
				sc.logs <- &LogMessage{sc.sid, last, lastClass, line}
				continue
			}

			// 1: time
			// 2: class
			// 3: message
			//t, err := time.Parse("2.1.2006 15:04:05", matches[1])
			t, err := time.Parse("15:04:05", matches[1])
			if err != nil {
				sc.logs <- &LogMessage{sc.sid, time.Now(), ErrorClass, err.Error()}
				t = time.Now()
			}
			last = t
			lastClass = matches[2]
			sc.logs <- &LogMessage{sc.sid, t, matches[2], matches[3]}
		}
	}
}
