// Copyright (c) 2019 Open2b Software Snc. All rights reserved.
// https://www.open2b.com

// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scriggo

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"sort"

	"github.com/open2b/scriggo/ast"
	"github.com/open2b/scriggo/env"
	"github.com/open2b/scriggo/internal/compiler"
	"github.com/open2b/scriggo/internal/runtime"
)

// EnvStringer is like fmt.Stringer where the String method takes an env.Env
// parameter.
type EnvStringer interface {
	String(env.Env) string
}

// HTMLStringer is implemented by values that are not escaped in HTML context.
type HTMLStringer interface {
	HTML() HTML
}

// HTMLEnvStringer is like HTMLStringer where the HTML method takes a
// env.Env parameter.
type HTMLEnvStringer interface {
	HTML(env.Env) HTML
}

// CSSStringer is implemented by values that are not escaped in CSS context.
type CSSStringer interface {
	CSS() CSS
}

// CSSEnvStringer is like CSSStringer where the CSS method takes an env.Env
// parameter.
type CSSEnvStringer interface {
	CSS(env.Env) CSS
}

// JSStringer is implemented by values that are not escaped in JavaScript
// context.
type JSStringer interface {
	JS() JS
}

// JSEnvStringer is like JSStringer where the JS method takes an env.Env
// parameter.
type JSEnvStringer interface {
	JS(env.Env) JS
}

// JSONStringer is implemented by values that are not escaped in JSON context.
type JSONStringer interface {
	JSON() JSON
}

// JSONEnvStringer is like JSONStringer where the JSON method takes an env.Env
// parameter.
type JSONEnvStringer interface {
	JSON(env.Env) JSON
}

// MarkdownStringer is implemented by values that are not escaped in Markdown
// context.
type MarkdownStringer interface {
	Markdown() Markdown
}

// MarkdownEnvStringer is like MarkdownStringer where the Markdown method
// takes an env.Env parameter.
type MarkdownEnvStringer interface {
	Markdown(env.Env) Markdown
}

// Format types.
type (
	HTML     string // the html type in templates.
	CSS      string // the css type in templates.
	JS       string // the js type in templates.
	JSON     string // the json type in templates.
	Markdown string // the markdown type in templates.
)

// A Format represents a content format.
type Format int

const (
	FormatText Format = iota
	FormatHTML
	FormatCSS
	FormatJS
	FormatJSON
	FormatMarkdown
)

// String returns the name of the format.
func (format Format) String() string {
	return ast.Format(format).String()
}

// Declarations contains variable, constant, function, type and package
// declarations.
type Declarations map[string]interface{}

type BuildTemplateOptions struct {
	DisallowGoStmt       bool
	NoParseShortShowStmt bool
	TreeTransformer      func(*ast.Tree) error // if not nil transforms tree after parsing.

	// DollarIdentifier, when true, keeps the backward compatibility by
	// supporting the dollar identifier.
	//
	// NOTE: the dollar identifier is deprecated and will be removed in a
	// future version of Scriggo.
	DollarIdentifier bool

	// MarkdownConverter converts a Markdown source code to HTML.
	MarkdownConverter Converter

	// Globals declares constants, types, variables and functions that are
	// accessible from the code in the template.
	Globals Declarations

	// Packages is a PackageLoader that makes precompiled packages available
	// in the template through the 'import' statement.
	//
	// Note that an import statement refers to a precompiled package read from
	// Packages if its path has no extension.
	//
	//     {%  import  "my/package"   %}    Import a precompiled package.
	//     {%  import  "my/file.html  %}    Import a template file.
	//
	Packages PackageLoader
}

// Converter is implemented by format converters.
type Converter func(src []byte, out io.Writer) error

type Template struct {
	fn          *runtime.Function
	typeof      runtime.TypeOfFunc
	globals     []compiler.Global
	mdConverter Converter
}

// FormatFS is the interface implemented by a file system that can determine
// the file format from a path name.
type FormatFS interface {
	fs.FS
	Format(name string) (Format, error)
}

// formatTypes contains the format types added to the universe block.
var formatTypes = map[ast.Format]reflect.Type{
	ast.FormatHTML:     reflect.TypeOf((*HTML)(nil)).Elem(),
	ast.FormatCSS:      reflect.TypeOf((*CSS)(nil)).Elem(),
	ast.FormatJS:       reflect.TypeOf((*JS)(nil)).Elem(),
	ast.FormatJSON:     reflect.TypeOf((*JSON)(nil)).Elem(),
	ast.FormatMarkdown: reflect.TypeOf((*Markdown)(nil)).Elem(),
}

// BuildTemplate builds the named template file rooted at the given file
// system.
//
// If fsys implements FormatFS, the file format is read from its Format
// method, otherwise it depends on the file name extension
//
//   HTML       : .html
//   CSS        : .css
//   JavaScript : .js
//   JSON       : .json
//   Markdown   : .md .mkd .mkdn .mdown .markdown
//   Text       : all other extensions
//
// If the named file does not exist, Build returns an error satisfying
// errors.Is(err, fs.ErrNotExist). If a build error occurs, it returns a
// *BuildError error.
func BuildTemplate(fsys fs.FS, name string, options *BuildTemplateOptions) (*Template, error) {
	co := compiler.Options{
		FormatTypes: formatTypes,
	}
	var mdConverter Converter
	if options != nil {
		co.Globals = compiler.Declarations(options.Globals)
		co.TreeTransformer = options.TreeTransformer
		co.DisallowGoStmt = options.DisallowGoStmt
		co.NoParseShortShowStmt = options.NoParseShortShowStmt
		co.DollarIdentifier = options.DollarIdentifier
		co.Packages = options.Packages
		co.MDConverter = compiler.Converter(options.MarkdownConverter)
		mdConverter = options.MarkdownConverter
	}
	code, err := compiler.BuildTemplate(fsys, name, co)
	if err != nil {
		if e, ok := err.(compilerError); ok {
			err = &BuildError{err: e}
		}
		return nil, err
	}
	return &Template{fn: code.Main, typeof: code.TypeOf, globals: code.Globals, mdConverter: mdConverter}, nil
}

// Run runs the template and write the rendered code to out. vars contains
// the values of the global variables.
func (t *Template) Run(out io.Writer, vars map[string]interface{}, options *RunOptions) error {
	if out == nil {
		return errors.New("invalid nil out")
	}
	vm := runtime.NewVM()
	if options != nil {
		if options.Context != nil {
			vm.SetContext(options.Context)
		}
		if options.PrintFunc != nil {
			vm.SetPrint(options.PrintFunc)
		}
	}
	renderer := newRenderer(out, t.mdConverter)
	vm.SetRenderer(renderer)
	_, err := vm.Run(t.fn, t.typeof, initGlobalVariables(t.globals, vars))
	if p, ok := err.(*runtime.Panic); ok {
		err = &Panic{p}
	}
	return err
}

// MustRun is like Run but panics if the execution fails.
func (t *Template) MustRun(out io.Writer, vars map[string]interface{}, options *RunOptions) {
	err := t.Run(out, vars, options)
	if err != nil {
		panic(err)
	}
}

// Disassemble disassembles a template and returns its assembly code.
//
// n determines the maximum length, in runes, of a disassembled text:
//
//   n > 0: at most n runes; leading and trailing white space are removed
//   n == 0: no text
//   n < 0: all text
//
func (t *Template) Disassemble(n int) []byte {
	assemblies := compiler.Disassemble(t.fn, t.globals, n)
	return assemblies["main"]
}

// UsedVars returns the names of the global variables used in the template.
func (t *Template) UsedVars() []string {
	vars := make([]string, len(t.globals))
	for i, global := range t.globals {
		vars[i] = global.Name
	}
	sort.Strings(vars)
	return vars
}

var emptyInit = map[string]interface{}{}

// initGlobalVariables initializes the global variables and returns their
// values. It panics if init is not valid.
//
// This function is a copy of the function in the scripts package.
func initGlobalVariables(variables []compiler.Global, init map[string]interface{}) []reflect.Value {
	n := len(variables)
	if n == 0 {
		return nil
	}
	if init == nil {
		init = emptyInit
	}
	values := make([]reflect.Value, n)
	for i, variable := range variables {
		if variable.Pkg == "main" {
			if value, ok := init[variable.Name]; ok {
				if variable.Value.IsValid() {
					panic(fmt.Sprintf("variable %q already initialized", variable.Name))
				}
				if value == nil {
					panic(fmt.Sprintf("variable initializer %q cannot be nil", variable.Name))
				}
				val := reflect.ValueOf(value)
				if typ := val.Type(); typ == variable.Type {
					v := reflect.New(typ).Elem()
					v.Set(val)
					values[i] = v
				} else {
					if typ.Kind() != reflect.Ptr || typ.Elem() != variable.Type {
						panic(fmt.Sprintf("variable initializer %q must have type %s or %s, but have %s",
							variable.Name, variable.Type, reflect.PtrTo(variable.Type), typ))
					}
					if val.IsNil() {
						panic(fmt.Sprintf("variable initializer %q cannot be a nil pointer", variable.Name))
					}
					values[i] = reflect.ValueOf(value).Elem()
				}
				continue
			}
		}
		if variable.Value.IsValid() {
			values[i] = variable.Value
		} else {
			values[i] = reflect.New(variable.Type).Elem()
		}
	}
	return values
}
