// Copyright (c) 2019 Open2b Software Snc. All rights reserved.
// https://www.open2b.com

// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package template

import (
	"bytes"
	"html"
	"io"
	"strings"
)

type urlWriter struct {
	path   bool
	query  bool
	addAmp bool
	isSet  bool
	w      io.Writer
}

// Write handle *ast.Show nodes in context URL.
func (w *urlWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	s := html.UnescapeString(string(p)) // TODO(Gianluca): optimize.
	sw := newStringWriter(w.w)
	if w.path {
		if strings.Contains(s, "?") {
			w.path = false
			w.addAmp = s[len(s)-1] != '?' && s[len(s)-1] != '&'
		}
		return 0, pathEscape(sw, s, true) // TODO(Gianluca): quote?
	}
	if w.query {
		return 0, queryEscape(sw, s)
	}
	return 0, nil
}

// WriteText handle the *ast.Text nodes in context URL.
func (w *urlWriter) WriteText(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if !w.query {
		if bytes.ContainsAny(p, "?#") {
			if p[0] == '?' && !w.path {
				if w.addAmp {
					_, err := io.WriteString(w.w, "&amp;")
					if err != nil {
						return 0, err
					}
				}
				p = p[1:]
			}
			w.path = false
			w.query = true
		}
		if w.isSet && bytes.ContainsRune(p, ',') {
			w.path = true
			w.query = false
		}
	}
	return w.w.Write(p)
}

func (w *urlWriter) Reset() {
	w.path = true
	w.query = false
	w.addAmp = false
	// TODO(Gianluca): isSet should be node.Attribute == "srcset". See
	// https://github.com/open2b/commerceready/blob/18d101b986d8ae53cf316a66ba8b4e57d2849242/reports-cgi/open2b/template/rendering.go#L141-L149
	w.isSet = false
}

type stringWriter interface {
	Write(b []byte) (int, error)
	WriteString(s string) (int, error)
}
