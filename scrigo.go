package scrigo

import (
	"fmt"
	"reflect"
	"strings"

	"scrigo/internal/compiler"
	"scrigo/internal/compiler/ast"
	"scrigo/native"
	"scrigo/vm"
)

type Program struct {
	Fn      *vm.ScrigoFunction
	globals []compiler.Global
}

func Compile(path string, reader compiler.Reader, packages map[string]*native.GoPackage) (*Program, error) {
	p := NewParser(reader, packages)
	tree, err := p.Parse(path)
	if err != nil {
		return nil, err
	}
	tci := map[string]*compiler.PackageInfo{}
	err = compiler.CheckPackage(tree, packages, tci)
	if err != nil {
		return nil, err
	}
	main, returnedGlobals := compiler.EmitPackage(tree.Nodes[0].(*ast.Package), packages, tci[path].TypeInfo, tci[path].IndirectVars)
	// TODO(gianluca): EmitPackage must returns the globals.
	globals := make([]compiler.Global, len(main.Globals))
	for i, global := range returnedGlobals {
		globals[i].Pkg = global.Pkg
		globals[i].Name = global.Name
		globals[i].Type = global.Type
		globals[i].Value = global.Value
	}
	return &Program{Fn: main, globals: globals}, nil
}

func Execute(p *Program) error {
	vmm := vm.New()
	if n := len(p.globals); n > 0 {
		globals := make([]interface{}, n)
		for i, global := range p.globals {
			if global.Value == nil {
				globals[i] = reflect.New(global.Type).Interface()
			} else {
				globals[i] = global.Value
			}
		}
		vmm.SetGlobals(globals)
	}
	_, err := vmm.Run(p.Fn)
	return err
}

// Parser implements a parser that reads the tree from a Reader and expands
// the nodes Extends, Import and Include. The trees are compiler.Cached so only one
// call per combination of path and context is made to the reader even if
// several goroutines parse the same paths at the same time.
//
// Returned trees can only be transformed if the parser is no longer used,
// because it would be the compiler.Cached trees to be transformed and a data race can
// occur. In case, use the function Clone in the astutil package to create a
// clone of the tree and then transform the clone.
type Parser struct {
	reader   compiler.Reader
	packages map[string]*native.GoPackage
	trees    *compiler.Cache
	// TODO (Gianluca): does packageInfos need synchronized access?
	packageInfos map[string]*compiler.PackageInfo // key is path.
	typeCheck    bool
}

// NewParser returns a new Parser that reads the trees from the reader r. typeCheck
// indicates if a type-checking must be done after parsing.
func NewParser(r compiler.Reader, packages map[string]*native.GoPackage) *Parser {
	p := &Parser{
		reader:   r,
		packages: packages,
		trees:    &compiler.Cache{},
	}
	return p
}

// Parse reads the source at path, with the reader, in the ctx context,
// expands the nodes Extends, Import and Include and returns the expanded tree.
//
// Parse is safe for concurrent use.
func (p *Parser) Parse(path string) (*ast.Tree, error) {

	// Path must be absolute.
	if path == "" {
		return nil, compiler.ErrInvalidPath
	}
	if path[0] == '/' {
		path = path[1:]
	}
	// Cleans the path by removing "..".
	path, err := compiler.ToAbsolutePath("/", path)
	if err != nil {
		return nil, err
	}

	pp := &expansion{p.reader, p.trees, p.packages, []string{}}

	tree, err := pp.parsePath(path)
	if err != nil {
		if err2, ok := err.(*compiler.SyntaxError); ok && err2.Path == "" {
			err2.Path = path
		} else if err2, ok := err.(compiler.CycleError); ok {
			err = compiler.CycleError(path + "\n\t" + string(err2))
		}
		return nil, err
	}
	if len(tree.Nodes) == 0 {
		return nil, &compiler.SyntaxError{"", ast.Position{1, 1, 0, 0}, fmt.Errorf("expected 'package' or script, found 'EOF'")}
	}

	return tree, nil
}

// TypeCheckInfos returns the type-checking infos collected during
// type-checking.
func (p *Parser) TypeCheckInfos() map[string]*compiler.PackageInfo {
	return p.packageInfos
}

// expansion is an expansion state.
type expansion struct {
	reader   compiler.Reader
	trees    *compiler.Cache
	packages map[string]*native.GoPackage
	paths    []string
}

// abs returns path as absolute.
func (pp *expansion) abs(path string) (string, error) {
	var err error
	if path[0] == '/' {
		path, err = compiler.ToAbsolutePath("/", path[1:])
	} else {
		parent := pp.paths[len(pp.paths)-1]
		dir := parent[:strings.LastIndex(parent, "/")+1]
		path, err = compiler.ToAbsolutePath(dir, path)
	}
	return path, err
}

// parsePath parses the source at path in context ctx. path must be absolute
// and cleared.
func (pp *expansion) parsePath(path string) (*ast.Tree, error) {

	// Checks if there is a cycle.
	for _, p := range pp.paths {
		if p == path {
			return nil, compiler.CycleError(path)
		}
	}

	// Checks if it has already been parsed.
	if tree, ok := pp.trees.Get(path, ast.ContextNone); ok {
		return tree, nil
	}
	defer pp.trees.Done(path, ast.ContextNone)

	src, err := pp.reader.Read(path, ast.ContextNone)
	if err != nil {
		return nil, err
	}

	tree, err := compiler.ParseSource(src, ast.ContextNone)
	if err != nil {
		return nil, err
	}
	tree.Path = path

	// Expands the nodes.
	pp.paths = append(pp.paths, path)
	err = pp.expand(tree.Nodes)
	if err != nil {
		if e, ok := err.(*compiler.SyntaxError); ok && e.Path == "" {
			e.Path = path
		}
		return nil, err
	}
	pp.paths = pp.paths[:len(pp.paths)-1]

	// Adds the tree to the compiler.Cache.
	pp.trees.Add(path, ast.ContextNone, tree)

	return tree, nil
}

// expand expands the nodes parsing the sub-trees in context ctx.
func (pp *expansion) expand(nodes []ast.Node) error {

	for _, node := range nodes {

		switch n := node.(type) {

		case *ast.Package:

			err := pp.expand(n.Declarations)
			if err != nil {
				return err
			}

		case *ast.Import:

			absPath, err := pp.abs(n.Path)
			if err != nil {
				return err
			}
			found := false
			for path := range pp.packages {
				if path == n.Path {
					found = true
					break
				}
			}
			if found {
				continue
			}
			n.Tree, err = pp.parsePath(absPath + ".go")
			if err != nil {
				if err == compiler.ErrInvalidPath {
					err = fmt.Errorf("invalid path %q at %s", n.Path, n.Pos())
				} else if err == compiler.ErrNotExist {
					err = &compiler.SyntaxError{"", *(n.Pos()), fmt.Errorf("cannot find package \"%s\"", n.Path)}
				} else if err2, ok := err.(compiler.CycleError); ok {
					err = compiler.CycleError("imports " + string(err2))
				}
				return err
			}

		}

	}

	return nil
}
