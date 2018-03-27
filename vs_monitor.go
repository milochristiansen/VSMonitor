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
import "fmt"
import "mime"
import "net/http"
import "os/signal"
import "crypto/tls"
import "encoding/json"
import "path/filepath"

import "github.com/milochristiansen/axis2"
import "github.com/milochristiansen/axis2/sources"

import "golang.org/x/crypto/acme/autocert"

/*

TODO:
* Clicking on the main window to select the input is nice, but try to come up with a way to keep it from deselecting your text selection each time.
* More colors for the log messages.

*/

func main() {
	cfgf, err := os.Open("./Monitor/cfg.json")
	if err != nil {
		// Create default configuration, print message, and exit.
		cfg := &MonitorConfig{
			ServerDir:        baseDir() + "/Binaries",
			DataDir:          baseDir() + "/GameData",
			LastSID:          0,
			HostName:         "localhost",
			Port:             "2660",
			AutoTLS:          false,
			Servers:          make(map[int]*ServerConfig),
			Versions:         make(map[GameVersion]BinaryStatus),
			Tokens:           make(map[string]MonitorUser),
			LaunchedHandlers: make(map[int]*ServerControler),
		}
		err := os.Mkdir(cfg.ServerDir, 0755)
		if err != nil && !os.IsExist(err) {
			fmt.Println("Error during setup:", err)
			os.Exit(1)
		}
		err = os.Mkdir(cfg.DataDir, 0755)
		if err != nil && !os.IsExist(err) {
			fmt.Println("Error during setup:", err)
			os.Exit(1)
		}
		err = os.Mkdir(baseDir()+"/Monitor", 0755)
		if err != nil && !os.IsExist(err) {
			fmt.Println("Error during setup:", err)
			os.Exit(1)
		}

		err = cfg.Dump()
		if err != nil {
			fmt.Println("Error during setup:", err)
			os.Exit(1)
		}
		fmt.Println("Default config file created, please edit '" + baseDir() + "/Monitor/cfg.json' to change the setting to fit your needs.")
		os.Exit(0)
	}

	cfg := &MonitorConfig{
		Servers:          make(map[int]*ServerConfig),
		Versions:         make(map[GameVersion]BinaryStatus),
		Tokens:           make(map[string]MonitorUser),
		LaunchedHandlers: make(map[int]*ServerControler),
	}
	dec := json.NewDecoder(cfgf)
	err = dec.Decode(&cfg)
	if err != nil {
		fmt.Println("Could not load config file:", err)
		os.Exit(1)
	}

	GlobalConfig = cfg

	// Spin up controllers for each defined server.
	cfg.Lock()
	for sid := range cfg.Servers {
		cfg.LaunchedHandlers[sid] = cfg.NewServerControler(sid)
	}
	cfg.Unlock()

	// Start the web UI server. This doesn't really interact with the monitor in any way except
	// for pointing web socket connection in the right general direction.
	go webUI(cfg.HostName, cfg.Port, cfg.AutoTLS)

	exitSignal := make(chan os.Signal)
	signal.Notify(exitSignal, os.Interrupt, os.Kill)
	<-exitSignal
}

func webUI(host, port string, autotls bool) {
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
	} else if !autotls {
		err := http.ListenAndServeTLS(":"+port, baseDir()+"/Monitor/cert.crt", baseDir()+"/Monitor/cert.key", nil)
		if err != nil {
			panic(err)
		}
	} else {
		certManager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(host),
			Cache:      autocert.DirCache(baseDir() + "/Monitor/certs"),
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
