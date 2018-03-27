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
import "os"
import "errors"
import "archive/tar"
import "compress/gzip"

var tarUnknownTypeErr = errors.New("Unknown record type in tar.gz file.")

func ExtractTarGz(r io.Reader, to string) error {
	tarstream, err := gzip.NewReader(r)
	if err != nil {
		return err
	}

	return ExtractTar(tarstream, to)
}

func ExtractTar(r io.Reader, to string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			err := os.MkdirAll(to+"/"+hdr.Name, 0755)
			if err != nil {
				return err
			}
		case tar.TypeReg:
			file, err := os.Create(to + "/" + hdr.Name)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.Copy(file, tr)
			if err != nil {
				return err
			}
		default:
			return tarUnknownTypeErr
		}
	}
	return nil
}
