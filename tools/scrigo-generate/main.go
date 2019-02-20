package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"scrigo/ast"
	"scrigo/parser"
)

func printErrorAndQuit(err interface{}) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func goImports(path string) error {
	cmd := exec.Command("goimports", "-w", path)
	stderr := bytes.Buffer{}
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("goimports: %s", stderr.String())
	}
	return nil
}

func usage() string {
	return "scrigo-generate imports-file variable-name"
}

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) != 2 {
		printErrorAndQuit(usage())
	}
	importsFile := flag.Arg(0)
	customVariableName := flag.Arg(1)
	if importsFile == "" || customVariableName == "" {
		printErrorAndQuit(usage())
	}
	src, err := ioutil.ReadFile(importsFile)
	if err != nil {
		panic(err)
	}
	tree, err := parser.ParseSource(src, ast.ContextNone)
	if err != nil {
		panic(err)
	}

	packages := []string{}
	if len(tree.Nodes) != 1 {
		printErrorAndQuit("imports file must be a package definition")
	}
	pkg, ok := tree.Nodes[0].(*ast.Package)
	if !ok {
		printErrorAndQuit("imports file must be a package definition")
	}
	for _, n := range pkg.Declarations {
		imp, ok := n.(*ast.Import)
		if !ok {
			printErrorAndQuit(fmt.Errorf("only imports are allowed in imports file %s", importsFile))
		}
		packages = append(packages, imp.Path)
	}

	out := generateMultiplePackages(packages, customVariableName)

	importsFileBase := filepath.Base(importsFile)
	newBase := "_" + importsFileBase
	outPath := filepath.Join(filepath.Dir(importsFile), newBase)

	f, err := os.Create(outPath)
	if err != nil {
		printErrorAndQuit(err)
	}
	_, err = f.WriteString(out)
	if err != nil {
		printErrorAndQuit(err)
	}
	err = goImports(outPath)
	if err != nil {
		printErrorAndQuit(err)
	}
}
