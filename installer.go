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
import "io"
import "fmt"
import "time"
import "bytes"
import "errors"
import "context"
import "runtime"
import "os/exec"
import "net/http"
import "io/ioutil"
import "crypto/md5"
import "encoding/hex"

const (
	vUnstableURL = "http://api.vintagestory.at/latestunstable.txt"
	vStableURL   = "http://api.vintagestory.at/lateststable.txt"
	downloadURL  = "https://account.vintagestory.at/files/%v/%v"

	catalog1URL = "http://api.vintagestory.at/stable.json"
	catalog2URL = "http://api.vintagestory.at/unstable.json"
)

var versionFetchError = errors.New("Error parsing retrieved version information.")
var invalidSIDError = errors.New("Invalid or non-existent SID.")
var versionValidError = errors.New("Version validation failed.")
var md5ValidError = errors.New("MD5 validation failed.")

func GetLatestGameVersion(stable bool) (string, error) {
	url := vStableURL
	if !stable {
		url = vUnstableURL
	}

	client := new(http.Client)
	r, err := client.Get(url)
	if r != nil {
		defer r.Body.Close()
	}
	if err != nil {
		return ErrorVersion, err
	}

	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return ErrorVersion, err
	}
	return string(bytes.TrimSpace(buf)), nil
}

func (c *MonitorConfig) InstallNewServer(stable bool, name string) (sid int, err error) {
	ver, err := GetLatestGameVersion(stable)
	if err != nil {
		return -1, err
	}

	return c.installNewServer(ver, stable, name)
}

func (c *MonitorConfig) InstallNewServerVersion(ver string, name string) (sid int, err error) {
	return c.installNewServer(ver, true, name)
}

func (c *MonitorConfig) installNewServer(ver string, stable bool, name string) (sid int, err error) {
	c.Lock()
	c.LastSID++
	sid = c.LastSID
	c.Servers[sid] = &ServerConfig{
		SID:     sid,
		Name:    name,
		Version: ver,
		Stable:  stable,
	}
	os.Mkdir(fmt.Sprintf("%v/%v %v", c.DataDir, name, sid), 0755) // Ignore error.
	bin := c.ServerDir
	dat := c.DataDir
	c.Unlock()

	err = c.FindOrDownload(ver)
	if err != nil {
		return sid, err
	}

	bin += fmt.Sprintf("/%v/VintagestoryServer.exe", ver)
	dat += fmt.Sprintf("/%v %v", name, sid)

	// HACK! But I'm feeling lazy, have pity on me.
	// This has a deadline to keep old versions that do not support --genconfig from hanging forever.
	var cmd *exec.Cmd
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, bin, "--dataPath", dat, "--genconfig")
	} else {
		cmd = exec.CommandContext(ctx, "mono", bin, "--dataPath", dat, "--genconfig")
	}

	err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return sid, nil
	}
	return sid, err
}

func (c *MonitorConfig) UpdateServer(sid int) error {
	c.RLock()
	sc, ok := c.Servers[sid]
	c.RUnlock()
	if !ok {
		return invalidSIDError
	}

	sc.RLock()
	stable := sc.Stable
	sc.RUnlock()

	ver, err := GetLatestGameVersion(stable)
	if err != nil {
		return err
	}
	return c.UpdateServerTo(ver, sid)
}

func (c *MonitorConfig) UpdateServerTo(ver string, sid int) error {
	c.RLock()
	sc, ok := c.Servers[sid]
	c.RUnlock()
	if !ok {
		return invalidSIDError
	}

	sc.Lock()
	sc.Version = ver
	sc.Unlock()
	return c.FindOrDownload(ver)
}

func (c *MonitorConfig) FindOrDownload(ver string) error {
	ok, stable, file, srmd5 := ValidateVersion(ver)
	if !ok {
		return versionValidError
	}
	srsum := make([]byte, 16)
	l, err := hex.Decode(srsum, srmd5)
	if err != nil || l != 16 {
		return md5ValidError
	}

	c.RLock()
	verinfo := c.Versions[ver]
	c.RUnlock()
	if verinfo == BinaryOK {
		return nil
	}
	c.Lock()
	c.Versions[ver] = BinaryCorrupted
	dir := c.ServerDir
	c.Unlock()

	dir += "/" + ver
	removeContents(dir) // Ignore errors here.

	s := "stable"
	if !stable {
		s = "unstable"
	}

	tr := &http.Transport{
		DisableCompression: true,
	}
	client := &http.Client{Transport: tr}
	r, err := client.Get(fmt.Sprintf(downloadURL, s, file))
	if r != nil {
		defer r.Body.Close()
	}
	if err != nil {
		return err
	}

	// Slurp the file into memory. This is a terrible idea, but oh well.
	buff := new(bytes.Buffer)
	dlmd5 := md5.New()
	multiWriter := io.MultiWriter(dlmd5, buff)
	_, err = io.Copy(multiWriter, r.Body)
	if err != nil {
		return err
	}
	dlsum := dlmd5.Sum(nil)
	if len(dlsum) != md5.Size || len(srsum) != md5.Size {
		return md5ValidError
	}
	for i := 0; i < md5.Size; i++ {
		if dlsum[i] != srsum[i] {
			return md5ValidError
		}
	}
	rdr := bytes.NewReader(buff.Bytes())

	err = ExtractTarGz(rdr, dir)
	if err != nil {
		return err
	}

	c.Lock()
	c.Versions[ver] = BinaryOK
	c.Unlock()
	return nil
}

func removeContents(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		err = os.RemoveAll(dir + "/" + name)
		if err != nil {
			return err
		}
	}
	return nil
}
