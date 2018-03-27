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

// For when strings.Split just isn't good enough...
func parseCommand(in []byte) []string {
	out := make([]string, 0)
	var buf []byte

	skipwhite := true
	quotes := false
	for _, b := range in {
		// Quoted things
		if quotes && b != '"' {
			buf = append(buf, b)
			continue
		}
		if b == '"' {
			quotes = !quotes
			out = append(out, string(buf))
			buf = nil
			continue
		}

		// White space
		if skipwhite && (b == ' ' || b == '\t') {
			continue
		}
		if b == ' ' || b == '\t' {
			skipwhite = true
			continue
		}
		if skipwhite {
			skipwhite = false
			if len(buf) > 0 {
				out = append(out, string(buf))
			}
			buf = nil
		}

		// Everything else
		buf = append(buf, b)
	}
	if len(buf) > 0 {
		out = append(out, string(buf))
	}
	return out
}
