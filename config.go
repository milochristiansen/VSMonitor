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
import "sync"
import "errors"
import "net/http"
import "encoding/json"

//import "github.com/blang/semver"

var unmarshalVersionError = errors.New("Could not unmarshal version number.")

var GlobalConfig *MonitorConfig

// MonitorConfig is the configuration file type. In actual usage it holds all the monitor state and acts
// as a central clearing house for each server controller.
type MonitorConfig struct {
	ServerDir string
	DataDir   string
	LastSID   int

	HostName string
	Port     string // Not used if AutoTLS is set.
	AutoTLS  bool

	// What servers are installed.
	Servers map[int]*ServerConfig

	// What versions of the game are currently installed and what is their status.
	Versions map[string]BinaryStatus

	// Tokens to users.
	Tokens map[string]*MonitorUser

	// Servers that currently have running monitors.
	LaunchedHandlers map[int]*ServerController `json:"-"`

	sync.RWMutex `json:"-"`
}

func (c *MonitorConfig) Dump() error {
	cf, err := os.Create(baseDir() + "/Monitor/cfg.json")
	if err != nil {
		return err
	}

	c.RLock()
	defer c.RUnlock()

	enc := json.NewEncoder(cf)
	enc.SetIndent("", "\t")
	err = enc.Encode(c)
	if err != nil {
		return err
	}
	return nil
}

type MonitorUser struct {
	Name    string
	IsAdmin bool
	Servers map[int]bool
}

// BinaryStatus is the status of a set of server binaries.
type BinaryStatus int

const (
	BinaryNotInstalled BinaryStatus = iota
	BinaryOK
	BinaryCorrupted // Set if the install failed part way through for example.
)

// ServerConfig tracks information for individual servers.
type ServerConfig struct {
	SID     int
	Name    string // Server data is stored in: DataDir/Name_SID
	Version string // The current version used for this server.
	Stable  bool   // Should this server track stable or unstable versions?

	sync.RWMutex `json:"-"`
}

var ErrorVersion = "-1.-1.-1.-1"

// ValidateVersion checks if the game version could be found in the version catalog.
func ValidateVersion(v string) (ok bool, stable bool, file string, md5 []byte) {
	ok, file, md5 = validateVersion(catalog1URL, v)
	if ok {
		return ok, true, file, md5
	}
	ok, file, md5 = validateVersion(catalog2URL, v)
	return ok, false, file, md5
}

func validateVersion(url string, v string) (bool, string, []byte) {
	client := new(http.Client)
	r, err := client.Get(url)
	if r != nil {
		defer r.Body.Close()
	}
	if err != nil {
		return false, "", []byte{}
	}

	dec := json.NewDecoder(r.Body)
	catalog := make(map[string]map[string]vercatinfo)
	err = dec.Decode(&catalog)
	if err != nil {
		return false, "", []byte{}
	}

	vcat, found := catalog[v]
	if found {
		dat, ok := vcat["server"]
		if !ok {
			return false, "", []byte{}
		}
		return true, dat.File, []byte(dat.MD5)
	}
	return false, "", []byte{}
}

type vercatinfo struct {
	File string `json:"filename"`
	MD5  string `json:"md5"`
}
