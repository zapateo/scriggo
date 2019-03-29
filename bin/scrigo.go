// Copyright (c) 2019 Open2b Software Snc. All rights reserved.
// https://www.open2b.com

// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"scrigo"
	"scrigo/parser"
)

var packages map[string]*parser.GoPackage

func main() {

	if len(os.Args) != 2 {
		fmt.Printf("usage: %s filename\n", os.Args[0])
		os.Exit(-1)
	}

	file := os.Args[1]
	ext := filepath.Ext(file)
	if ext != ".go" && ext != ".gos" && ext != ".html" {
		fmt.Printf("%s: extension must be \".go\" for main packages, \".gos\" for scripts and \".html\" for template pages\n", file)
		os.Exit(-1)
	}

	absFile, err := filepath.Abs(file)
	if err != nil {
		fmt.Printf("%s: %s\n", file, err)
		os.Exit(-1)
	}

	switch ext {
	case ".gos":
		src, err := ioutil.ReadFile(absFile)
		if err != nil {
			fmt.Println(err)
			os.Exit(-1)
		}
		r := bytes.NewReader(src)
		s, err := scrigo.CompileScript(r, &parser.GoPackage{})
		if err != nil {
			fmt.Println(err)
			os.Exit(-1)
		}
		err = scrigo.ExecuteScript(s, nil)
		if err != nil {
			fmt.Println(err)
			os.Exit(-1)
		}
	case ".go":
		r := parser.DirReader(filepath.Dir(absFile))
		compiler := scrigo.NewCompiler(r, packages)
		f, err := os.Open(file)
		if err != nil {
			fmt.Println(err)
			os.Exit(-1)
		}
		program, err := compiler.Compile(f)
		if err != nil {
			fmt.Println(err)
			os.Exit(-1)
		}
		f.Close()
		err = scrigo.Execute(program)
		if err != nil {
			fmt.Println(err)
			os.Exit(-1)
		}
	case ".html":
		r := parser.DirReader(filepath.Dir(absFile))
		template := scrigo.NewTemplate(r)
		path := "/" + filepath.Base(absFile)
		page, err := template.Compile(path, nil, scrigo.ContextHTML)
		if err != nil {
			fmt.Println(err)
			os.Exit(-1)
		}
		err = scrigo.Render(os.Stdout, page, nil)
		if err != nil {
			fmt.Println(err)
			os.Exit(-1)
		}
	}

}
