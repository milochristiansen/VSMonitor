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

// VS Monitor: Vintage Story server monitor and controller.
package main

import "os"
import "io"
import "fmt"
import "flag"
import "time"
import "mime"
import "os/exec"
import "strings"
import "net/http"
import "io/ioutil"
import "crypto/tls"
import "crypto/rand"
import "path/filepath"

import "github.com/milochristiansen/axis2"
import "github.com/milochristiansen/axis2/sources"

import "golang.org/x/crypto/acme/autocert"

/*

WARNING!

This program is an example of lazy code at its finest.

I did most everything the easy way, not the correct way. Please don't use this as a good example.

*/

const (
	countRestarts = 3
	loopThreshold = 15 * time.Second
)

var (
	serverBinary = []string{}
	host         = "localhost"
	port         = "2660"
	admin        = ""
	noAutoCert   = false
)

func main() {
	flag.StringVar(&host, "host", host, "Host name of the server running the monitor. If 'localhost' then HTTP will be used instead of HTTPS.")
	flag.StringVar(&port, "port", port, "Port to listen on when -nocert is set or when listening on localhost (default is 2660.")
	flag.BoolVar(&noAutoCert, "nocert", noAutoCert, "If set a certificate will *not* be automatically acquired from Let's Encrypt.")
	flag.StringVar(&admin, "admin", admin, "The administrator user. If unset or invalid anyone can carry out admin functions.")

	flag.Parse()

	serverBinary = flag.Args()

	if len(serverBinary) == 0 {
		serverBinary = []string{"VintagestoryServer.exe"}
	}

	ichan := make(chan io.WriteCloser)
	ochan := make(chan io.ReadCloser)
	logs := make(chan string)
	restart, kill, down := make(chan bool), make(chan bool), make(chan bool)
	tokens := loadTokens()

	go webUI(tokens)

	go logReader(ochan, logs)

	// Spin up the process manager
	go restartLoop(restart, kill, down, ichan, ochan)

	// Integration loop.
	isDown := false
	var commandWriter io.WriteCloser
	for {
		select {
		case i := <-ichan:
			commandWriter = i
		case <-down:
			isDown = true
		case msg := <-logs:
			log(msg)
		case command := <-GlobalSockets.Messages:
			// Validating the token on every message is way overkill, but this way I don't have to track any connection state.
			parts := strings.SplitN(command, "|", 2)
			if len(parts) != 2 {
				log("Monitor Error: Invalid command or missing token.\n")
				continue
			}

			user, ok := tokens[parts[0]]
			if !ok {
				log("Monitor Error: Invalid token.\n")
				continue
			}
			if parts[0] == "DEADBEEF" {
				log("WARNING: You are using the default token! Please issue yourself a proper token and revoke the token for 'root'.\n")
			}

			command = parts[1]

			commandBits := strings.Split(command, " ")
			for i, v := range commandBits {
				commandBits[i] = strings.TrimSpace(v)
			}

			switch commandBits[0] {
			case ":help":
				log("Monitor commands: :recover, :kill, :token\n")
			case ":kill":
				if len(commandBits) != 2 {
					log(":kill (monitor|server)\n")
					continue
				}

				switch commandBits[1] {
				case "monitor":
					os.Exit(0)
				case "server":
					if isDown {
						kill <- true
					} else {
						log("Monitor Error: Server is already down.\n")
					}
				default:
					log(":kill (monitor|server)\n")
				}
			case ":recover":
				if isDown {
					isDown = false
					restart <- true
				} else {
					log("Monitor Error: Server is not down.\n")
				}
			case ":token":
				if len(commandBits) != 3 {
					log(":token (issue|revoke) username\n")
					continue
				}
				if admin != "" && user != admin {
					log(":token is an administrator only command.\n")
					return
				}

				switch commandBits[1] {
				case "issue":
					b := make([]byte, 16)
					_, err := rand.Read(b)
					if err != nil {
						log("Monitor Error: Could not issue token: %v\n", err)
						continue
					}
					token := fmt.Sprintf("%X", b)
					tokens[token] = commandBits[2]
					err = dumpTokens(tokens)
					if err != nil {
						log("Monitor Error: Error saving tokens: %v\n", err)
						continue
					}
					log("Monitor: Issued token %v for %v\n", token, commandBits[2])
				case "revoke":
					for token, user := range tokens {
						if user == commandBits[2] {
							delete(tokens, token)
							break
						}
					}
					err := dumpTokens(tokens)
					if err != nil {
						log("Monitor Error: Error saving tokens: %v\n", err)
						continue
					}
					log("Monitor: Revoked token for: %v\n", commandBits[2])
				default:
					log(":token (issue|revoke) username\n")
					continue
				}
			default:
				// Send to the command writer.
				if commandWriter != nil {
					_, err := commandWriter.Write([]byte(command))
					if err != nil {
						log("Monitor Error: %v\n", err)
					}
				} else {
					log("Monitor Error: Cannot send messages to server right now. Wait a bit and try again.\n")
				}
			}
		}
	}
}

func webUI(tokens map[string]string) {
	FS := new(axis2.FileSystem)
	FS.Mount("", sources.NewOSDir(baseDir()+"/monitor/ui"), false)

	http.HandleFunc("/socket", GlobalSockets.Upgrade)

	// Basic UI server.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "/ui.html"
		}

		typ := mime.TypeByExtension(GetExt(path))
		content, err := FS.ReadAll(path[1:])
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if typ != "" {
			w.Header().Set("Content-Type", typ)
		}
		fmt.Fprintf(w, "%s", content)
	})

	if host == "localhost" {
		err := http.ListenAndServe("127.0.0.1:"+port, nil)
		if err != nil {
			panic(err)
		}
	} else if noAutoCert {
		err := http.ListenAndServeTLS(":"+port, baseDir()+"/monitor/cert.crt", baseDir()+"/monitor/cert.key", nil)
		if err != nil {
			panic(err)
		}
	} else {
		certManager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(host),
			Cache:      autocert.DirCache(baseDir() + "/monitor/certs"),
		}

		server := &http.Server{
			Addr: ":https",
			TLSConfig: &tls.Config{
				GetCertificate: certManager.GetCertificate,
			},
		}

		// Needed for autocert to work.
		go http.ListenAndServe(":http", certManager.HTTPHandler(nil))

		//Key and cert are coming from Let's Encrypt
		err := server.ListenAndServeTLS("", "")
		if err != nil {
			panic(err)
		}
	}
}

func logReader(ochan chan io.ReadCloser, logs chan string) {
	var rdr io.ReadCloser
	buf := make([]byte, 0, 1024)
	for {
		select {
		case o := <-ochan:
			rdr = o
		default:
		}

		if rdr != nil {
			n, _ := rdr.Read(buf[:cap(buf)])
			if n == 0 {
				continue // We ignore all errors here :(
			}
			// TODO: It would be a good idea to split this into lines and send each line.
			// (would require more buffering)
			logs <- string(buf[:n])
		}
	}
}

func restartLoop(restart chan bool, kill chan bool, down chan bool, ichan chan io.WriteCloser, ochan chan io.ReadCloser) {
	var restarts [countRestarts]time.Time
	autostart := false

	for {
		if !autostart {
			down <- true // Alert the main system that the server is down and needs user intervention.
			ichan <- nil // Stop IO.
			ochan <- nil
			autostart = true
			<-restart // Wait for the main system to reply with a :recover command.
		}

		if t := time.Since(restarts[0]); t < loopThreshold {
			log("Monitor %d automatic server restarts in %v.\n  Server is DOWN at %v, awaiting :recover command.\n",
				countRestarts, t, time.Now().UTC().Format("06/01/02 15:04:05"))
			autostart = false
			continue
		}

		copy(restarts[0:], restarts[1:])
		restarts[len(restarts)-1] = time.Now()

		log("Monitor (re)starting server...\n")
		bin := serverBinary[0]
		var args []string
		if len(serverBinary) > 1 {
			args = serverBinary[1:]
		}
		cmd := exec.Command(bin, args...)

		ipipe, err := cmd.StdinPipe()
		if err != nil {
			log("Monitor could not restart server (attaching input pipe): %v\n  Server is DOWN at %v, awaiting :recover command.\n",
				err, time.Now().UTC().Format("06/01/02 15:04:05"))
			autostart = false
			continue
		}

		opipe, err := cmd.StdoutPipe()
		if err != nil {
			log("Monitor could not restart server (attaching output pipe): %v\n  Server is DOWN at %v, awaiting :recover command.\n",
				err, time.Now().UTC().Format("06/01/02 15:04:05"))
			autostart = false
			continue
		}

		err = cmd.Start()
		if err != nil {
			log("Monitor could not restart server: %v\n  Server is DOWN at %v, awaiting :recover command.\n",
				err, time.Now().UTC().Format("06/01/02 15:04:05"))
			autostart = false
			continue
		}

		ichan <- ipipe
		ochan <- opipe

		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case <-kill:
			if err := cmd.Process.Kill(); err != nil {
				log("Monitor failed to kill server: %v\n  Server is ROGUE at %v, awaiting :recover command.\n",
					err, time.Now().UTC().Format("06/01/02 15:04:05")) // lol
				autostart = false
				continue
			}
			log("Monitored server killed.\n  Server is DOWN at %v, awaiting :recover command.\n",
				time.Now().UTC().Format("06/01/02 15:04:05"))
			autostart = false
			continue
		case err := <-done:
			if err == nil {
				log("Monitored server exited intentionally.\n  Server is DOWN at %v, awaiting :recover command.\n",
					time.Now().UTC().Format("06/01/02 15:04:05"))
				autostart = false
				continue
			}
			log("Monitored server died.\n  Reported error: %v\n  Server going for automatic restart at %v.\n",
				err, time.Now().UTC().Format("06/01/02 15:04:05"))
			autostart = true
		}
	}
}

func dumpTokens(tokens map[string]string) error {
	out, err := os.Create(baseDir() + "/monitor/tokens.psv")
	if err != nil {
		return err
	}
	defer out.Close()

	fmt.Fprintf(out, "# Editing this file while the monitor is running can cause your changes to be lost without warning!\n")
	for token, user := range tokens {
		fmt.Fprintf(out, "%v|%v\n", user, token)
	}
	return nil
}

func loadTokens() map[string]string {
	content, err := ioutil.ReadFile(baseDir() + "/monitor/tokens.psv")
	if err != nil {
		return map[string]string{}
	}

	out := map[string]string{}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		out[parts[1]] = parts[0]
	}
	return out
}

var baseDirV string

func baseDir() string {
	if baseDirV != "" {
		return baseDirV
	}

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	baseDirV = filepath.Dir(ex)
	return baseDirV
}

func log(f string, v ...interface{}) {
	msg := fmt.Sprintf(f, v...)
	GlobalSockets.Broadcast([]byte(msg))
	fmt.Print(msg)
}

func GetExt(name string) string {
	// Find the last part of the extension
	i := len(name) - 1
	for i >= 0 {
		if name[i] == '.' {
			return name[i:]
		}
		i--
	}
	return ""
}
