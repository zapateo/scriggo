package main

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/token"
	"os"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/tools/go/loader"
)

type constant struct {
	expression string
	isTyped    bool
}

func getAllConstantExpressions(pkgPath string) (map[string]constant, error) {
	config := loader.Config{}
	config.Import(pkgPath)
	program, err := config.Load()
	constants := make(map[string]constant)
	if err != nil {
		return nil, err
	}
	pkgInfo := program.Package(pkgPath)
	for _, file := range pkgInfo.Files {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				if genDecl.Tok != token.CONST {
					continue
				}
				for _, spec := range genDecl.Specs {
					if valueSpec, ok := spec.(*ast.ValueSpec); ok {
						for i, name := range valueSpec.Names {
							if !isExported(name.Name) {
								continue
							}
							if i > len(valueSpec.Values)-1 {
								continue
							}
							expr := valueSpec.Values[i]
							c := constant{
								expression: strconv.Quote(pkgInfo.Types[expr].Value.ExactString()),
								isTyped:    valueSpec.Type != nil,
							}
							constants[name.Name] = c
						}
					}
				}
			}
		}
	}
	return constants, nil
}

func mapEntry(key, value string) string {
	return fmt.Sprintf("\t\"%s\": %s,\n", key, value)
}

func isExported(name string) bool {
	return unicode.Is(unicode.Lu, []rune(name)[0])
}

var generatedSkel = `[generatedWarning]

package [pkgName]

import (
	[explicitImports]
)

import "scrigo"

func init() {
	[predefinedTypes]
	[customVariableName] = map[string]*parser.GoPackage{
		[pkgContent]
	}
}
`

func generateMultiplePackages(pkgs []string, sourceFile, customVariableName, pkgName string) string {

	explicitImports := ""
	for _, p := range pkgs {
		explicitImports += strings.Replace(p, "/", "_", -1) + `"` + p + `"` + "\n"
	}

	predefinedTypes := map[string]string{}

	pkgContent := ""
	for _, p := range pkgs {
		out, predefTypes := generatePackage(p)
		for _, t := range predefTypes {
			switch t {
			case "intType":
				predefinedTypes["intType"] = "reflect.TypeOf(0)"
			case "stringType":
				predefinedTypes["stringType"] = "reflect.TypeOf(\"\")"
			default:
				panic(fmt.Errorf("unkown predefined type: %s", t))
			}
		}
		pkgContent += out
	}

	pt := ""
	for name, val := range predefinedTypes {
		pt += name + " := " + val + "\n"
	}

	r := strings.NewReplacer(
		"[generatedWarning]", "// Code generated by scrigo-generate, based on file \""+sourceFile+"\". DO NOT EDIT.",
		"[pkgName]", pkgName,
		"[explicitImports]", explicitImports,
		"[customVariableName]", customVariableName,
		"[predefinedTypes]", pt,
		"[pkgContent]", pkgContent,
	)
	return r.Replace(generatedSkel)
}

func generatePackage(pkgPath string) (string, []string) {
	predefinedTypes := []string{}
	register := func(t string) {
		for _, pt := range predefinedTypes {
			if t == pt {
				return
			}
		}
		predefinedTypes = append(predefinedTypes, t)
	}
	pkg, err := importer.Default().Import(pkgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "importer error: %s\n", err)
		return "", nil
	}
	pkgBase := strings.Replace(pkgPath, "/", "_", -1)
	var pkgContent string
	for _, name := range pkg.Scope().Names() {
		if !isExported(name) {
			continue
		}
		obj := pkg.Scope().Lookup(name)
		objSign := obj.String()
		objPath := pkgBase + "." + name
		switch {

		// It's a variable.
		case strings.HasPrefix(objSign, "var"):
			pkgContent += mapEntry(name, "&"+objPath)

		// It's a function.
		case strings.HasPrefix(objSign, "func"):
			pkgContent += mapEntry(name, objPath)

		// It's a type definition.
		case strings.HasPrefix(objSign, "type"):
			parts := strings.Fields(objSign)
			var value = "reflect.TypeOf(new(" + objPath + ")).Elem()"
			if len(parts) == 3 {
				typ := parts[2]
				switch typ {
				case "int", "string":
					value = typ + "Type"
					register(value)
				}
			}
			pkgContent += mapEntry(name, value)

		// It's a constant.
		case strings.HasPrefix(objSign, "const"):
			// Added later.

		// Unknown package element.
		default:
			fmt.Fprintf(os.Stderr, "unknown: %s (obj: %s)\n", name, obj.String())
		}
	}

	constants, err := getAllConstantExpressions(pkgPath)
	if err != nil {
		panic(err)
	}
	for name, constant := range constants {
		typ := "nil"
		if constant.isTyped {
			typ = "reflect.TypeOf(" + pkgBase + "." + name + ")"
		}
		pkgContent += mapEntry(name, fmt.Sprintf("scrigo.Constant(%s, %s)", constant.expression, typ))
	}

	skel := `
		"[pkgPath]": &parser.GoPackage{
			Name: "[pkg.Name()]",
			Declarations: map[string]interface{}{
				[pkgContent]
			},
		},`

	repl := strings.NewReplacer(
		"[pkgPath]", pkgPath,
		"[pkgContent]", pkgContent,
		"[pkg.Name()]", pkg.Name(),
	)

	return repl.Replace(skel), predefinedTypes
}
