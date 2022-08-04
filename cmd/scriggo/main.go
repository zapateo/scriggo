// Copyright 2019 The Scriggo Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
)

//go:embed init_main.go
var runSource []byte

var simpleScriggofileContent = []byte("\nIMPORT STANDARD LIBRARY\n")

// If the SCRIGGO_INIT_EMPTY_SCRIGGOFILE is set, the Scriggofile generated by
// 'scriggo init' will be empty.
// This option is currently used for testing, it is not documented and may be
// removed in a future version of Scriggo.
func init() {
	if os.Getenv("SCRIGGO_INIT_EMPTY_SCRIGGOFILE") != "" {
		simpleScriggofileContent = []byte{}
	}
}

func main() {

	flag.Usage = commandsHelp["scriggo"]

	// No command provided.
	if len(os.Args) == 1 {
		flag.Usage()
		exit(0)
		return
	}

	cmdArg := os.Args[1]
	// Used by flag.Parse.
	os.Args = append(os.Args[:1], os.Args[2:]...)
	cmd, ok := commands[cmdArg]
	if !ok {
		stderr(
			fmt.Sprintf("scriggo %s: unknown command", cmdArg),
			`Run 'scriggo help' for usage.`,
		)
		exit(1)
		return
	}
	cmd()

	return
}

// TestEnvironment is true when testing the scriggo command, false otherwise.
var TestEnvironment = false

// exit causes the current program to exit with the given status code. If
// running in a test environment, every exit call is a no-op.
func exit(status int) {
	if !TestEnvironment {
		os.Exit(status)
	}
}

// stderr prints lines on stderr.
func stderr(lines ...string) {
	for _, l := range lines {
		_, _ = fmt.Fprint(os.Stderr, l+"\n")
	}
}

// exitError prints an error message on stderr with a bold red color and exits
// with status code 1.
func exitError(format string, a ...interface{}) {
	msg := fmt.Errorf(format, a...)
	stderr(msg.Error())
	exit(1)
	return
}

// commandsHelp maps a command name to a function that prints the help for
// that command.
var commandsHelp = map[string]func(){
	"scriggo": func() {
		txtToHelp(helpScriggo)
		flag.PrintDefaults()
	},
	// Help topics.
	"Scriggofile": func() {
		txtToHelp(helpScriggofile)
	},

	// Commands helps.
	"bug": func() {
		stderr(
			`usage: scriggo bug`,
			`Bug opens the default browser and starts a new bug report.`,
			`The report includes useful system information.`,
		)
	},
	"import": func() {
		txtToHelp(helpImport)
	},
	"init": func() {
		txtToHelp(helpInit)
	},
	"run": func() {
		txtToHelp(helpRun)
	},
	"serve": func() {
		txtToHelp(helpServe)
	},
	"limitations": func() {
		txtToHelp(helpLimitations)
	},
	"stdlib": func() {
		stderr(
			`usage: scriggo stdlib`,
			``,
			`Stdlib prints to the standard output the paths of the packages of the Go`,
			`standard library imported by the instruction 'IMPORT STANDARD LIBRARY' in the`,
			`Scriggofile.`)

	},
	"version": func() {
		stderr(
			`usage: scriggo version`,
		)
	},
}

// commands maps a command name to a function that executes that command.
// Commands are called by command-line using:
//
//	scriggo command
var commands = map[string]func(){
	"bug": func() {
		flag.Usage = commandsHelp["bug"]
		fmt.Fprintf(os.Stdout, "If you encountered an issue, report it at:\n\n\thttps://github.com/open2b/scriggo/issues/new\n\n")
		exit(0)
	},
	"init": func() {
		flag.Usage = commandsHelp["init"]
		f := flag.String("f", "", "path of the Scriggofile.")
		x := flag.Bool("x", false, "print the commands.")
		flag.Parse()
		var path string
		switch n := len(flag.Args()); n {
		case 0:
		case 1:
			path = flag.Arg(0)
		default:
			flag.Usage()
			exitError(`bad number of arguments`)
		}
		err := _init(path, buildFlags{f: *f, x: *x})
		if err != nil {
			exitError("%s", err)
		}
		exit(0)
	},
	"import": func() {
		flag.Usage = commandsHelp["import"]
		f := flag.String("f", "", "path of the Scriggofile.")
		v := flag.Bool("v", false, "print the names of packages as the are imported.")
		x := flag.Bool("x", false, "print the commands.")
		o := flag.String("o", "", "write the source to the named file instead of stdout.")
		flag.Parse()
		var path string
		switch n := len(flag.Args()); n {
		case 0:
		case 1:
			path = flag.Arg(0)
		default:
			flag.Usage()
			exitError(`bad number of arguments`)
		}
		err := _import(path, buildFlags{f: *f, v: *v, x: *x, o: *o})
		if err != nil {
			exitError("%s", err)
		}
		exit(0)
	},
	"run": func() {
		flag.Usage = commandsHelp["run"]
		root := flag.String("root", "", "set the root directory to named dir instead of the file's directory.")
		var consts []string
		flag.Func("const", "run with global constants with the given names and values.", func(s string) error {
			consts = append(consts, s)
			return nil
		})
		format := flag.String("format", "", "force run to use the named file format.")
		s := flag.Int("S", 0, "print assembly listing. n determines the length of Text instructions.")
		metrics := flag.Bool("metrics", false, "print metrics about file execution.")
		o := flag.String("o", "", "write the resulting code to the named file or directory instead of stdout.")
		flag.Parse()
		asm := -2 // -2: no assembler
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "S" {
				asm = *s
				if asm < -1 {
					asm = -1
				}
			}
		})
		var name string
		switch len(flag.Args()) {
		case 0:
			exitError("%s", "missing file name")
		case 1:
			name = flag.Arg(0)
		default:
			exitError("%s", "too many file names")
		}
		err := run(name, buildFlags{consts: consts, format: *format, metrics: *metrics, o: *o, root: *root, s: asm})
		if err != nil {
			exitError("%s", err)
		}
		exit(0)
	},
	"serve": func() {
		flag.Usage = commandsHelp["serve"]
		s := flag.Int("S", 0, "print assembly listing. n determines the length of Text instructions.")
		metrics := flag.Bool("metrics", false, "print metrics about file executions.")
		flag.Parse()
		asm := -2 // -2: no assembler
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "S" {
				asm = *s
				if asm < -1 {
					asm = -1
				}
			}
		})
		err := serve(asm, *metrics)
		if err != nil {
			exitError("%s", err)
		}
		exit(0)
	},
	"stdlib": func() {
		flag.Usage = commandsHelp["stdlib"]
		flag.Parse()
		if len(flag.Args()) > 0 {
			flag.Usage()
			exitError(`bad number of arguments`)
		}
		err := stdlib()
		if err != nil {
			exitError("%s", err)
		}
		exit(0)
	},
	"help": func() {
		if len(os.Args) == 1 {
			flag.Usage()
			exit(0)
			return
		}
		topic := os.Args[1]
		help, ok := commandsHelp[topic]
		if !ok {
			_, _ = fmt.Fprintf(os.Stderr, "scriggo help %s: unknown help topic. Run 'scriggo help'.\n", topic)
			exit(1)
			return
		}
		help()
	},
	"version": func() {
		flag.Usage = commandsHelp["version"]
		fmt.Printf("scriggo version %s (%s)\n", version(), runtime.Version())
	},
}

// Version holds the version of the scriggo command, or the empty string if it
// has not been set when building.
// It may be set at compile time by passing the 'ldflags' to 'go build' and
// should have the form 'v<major>.<minor>.<patch>'.
var Version string

// version returns the scriggo command version.
func version() string {
	// First: read the version from 'runtime/debug', that holds a valid
	// semantic version if the scriggo command is compiled through 'go
	// install'.
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "(devel)" {
			return v
		}
	}
	// Second: read the version from the package-level variable Version, that
	// is set passing the ldflags to 'go build'.
	if Version != "" {
		return Version
	}
	// None of the versions above have been set.
	return "unknown"
}

// _import executes the sub commands "import":
//
//	scriggo import
func _import(path string, flags buildFlags) (err error) {

	_, err = exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("scriggo: \"go\" executable file not found in $PATH\nIf not installed, " +
			"download and install Go: https://go.dev/dl/\n")
	}

	goos := os.Getenv("GOOS")
	if goos == "" {
		goos = runtime.GOOS
	}

	// Run in module-aware mode.
	if flags.x {
		_, _ = fmt.Fprintln(os.Stderr, "export GO111MODULE=on")
	}
	if err := os.Setenv("GO111MODULE", "on"); err != nil {
		return fmt.Errorf("scriggo: can't set environment variable \"GO111MODULE\" to \"on\": %s", err)
	}

	var modDir string

	if path == "" {
		// path is current directory.
		modDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("scriggo: can't get current directory: %s", err)
		}
	} else if modfile.IsDirectoryPath(path) {
		// path is a local path.
		fi, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				err = fmt.Errorf("scriggo: directory %s does not exist in:\n\t%s", path, modDir)
			}
			return err
		}
		modDir, err = filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("scriggo: can't get absolute path of %s: %s", path, err)
		}
		if !fi.IsDir() {
			return fmt.Errorf("scriggo: %s is not a directory:\n\t%s", path, modDir)
		}
	} else {
		return fmt.Errorf("scriggo: path, if not empty, must be rooted or must start with '.%c' or '..%c'",
			os.PathSeparator, os.PathSeparator)
	}

	// Get the absolute Scriggofile's path.
	var sfPath string
	if flags.f == "" {
		sfPath = filepath.Join(modDir, "Scriggofile")
	} else {
		sfPath, err = filepath.Abs(flags.f)
		if err != nil {
			return fmt.Errorf("scriggo: can't get absolute path of %s: %s", flags.f, err)
		}
	}

	// Read the Scriggofile.
	scriggofile, err := os.Open(sfPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("scriggo: no Scriggofile in:\n\t%s", sfPath)
		}
		return err
	}
	defer scriggofile.Close()

	// Parse the Scriggofile.
	sf, err := parseScriggofile(scriggofile, goos)
	if err != nil {
		return err
	}
	err = scriggofile.Close()
	if err != nil {
		return err
	}

	// Create the package declarations file.
	out, err := getOutputFlag(flags.o)
	if err != nil {
		return err
	}
	if out != nil {
		defer func() {
			err2 := out.Close()
			if err == nil {
				err = err2
			}
		}()
	}
	err = renderPackages(out, modDir, sf, goos, flags)

	return err
}

type buildFlags struct {
	metrics, work, v, x, w bool
	f, format, o, root     string
	consts                 []string
	s                      int
}

// _init executes the sub commands "init":
//
//	scriggo init
func _init(path string, flags buildFlags) error {

	var err error

	var modDir string

	if path == "" {
		// path is current directory.
		modDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("scriggo: can't get current directory: %s", err)
		}
	} else if modfile.IsDirectoryPath(path) {
		// path is a local path.
		fi, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				err = fmt.Errorf("scriggo: directory %s does not exist in:\n\t%s", path, modDir)
			}
			return err
		}
		modDir, err = filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("scriggo: can't get absolute path of %s: %s", path, err)
		}
		if !fi.IsDir() {
			return fmt.Errorf("scriggo: %s is not a directory:\n\t%s", path, modDir)
		}
	} else {
		return fmt.Errorf("scriggo: path, if not empty, must be rooted or must start with '.%c' or '..%c'",
			os.PathSeparator, os.PathSeparator)
	}

	// Verify that module dir does not contain "main.go", "packages.go", "Scriggofile" files or a vendor directory.
	entries, err := os.ReadDir(modDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			switch entry.Name() {
			case "main.go", "packages.go", "Scriggofile":
				if path == "" {
					return fmt.Errorf("scriggo: current directory already contains %q file", entry.Name())
				}
				return fmt.Errorf("scriggo: directory %q already contains %q file", path, entry.Name())
			}
		}
		if entry.IsDir() && entry.Name() == "vendor" {
			if path == "" {
				return fmt.Errorf("scriggo: current directory contains a vendor directory")
			}
			return fmt.Errorf("scriggo: directory %q contains a vendor directory", path)
		}
	}

	// Get the absolute Scriggofile's path.
	var sfPath string
	if flags.f == "" {
		sfPath = filepath.Join(modDir, "Scriggofile")
	} else {
		sfPath, err = filepath.Abs(flags.f)
		if err != nil {
			return fmt.Errorf("scriggo: can't get absolute path of %s: %s", flags.f, err)
		}
	}

	// Create the go.mod file if it does not exist.
	modPath := filepath.Base(modDir)
	modFile := filepath.Join(modDir, "go.mod")
	fi, err := os.OpenFile(modFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
	if err == nil {
		// Write the go.mod file.
		_, err = fmt.Fprintf(fi, "module %s\n", strconv.Quote(modPath))
		if err == nil {
			err = fi.Close()
		}
	}
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}

	// Create the Scriggofile.
	err = os.WriteFile(sfPath, simpleScriggofileContent, 0666)
	if err != nil {
		return err
	}

	// Create the main.go file.
	mainPath := filepath.Join(modDir, "main.go")
	mainSource := bytes.Replace(runSource, []byte("func _main() {"), []byte("func main() {"), 1)
	err = os.WriteFile(mainPath, mainSource, 0666)
	if err != nil {
		return err
	}

	// Embed the packages.go file.
	flags.o = filepath.Join(modDir, "packages.go")
	flags.v = true
	err = _import(path, flags)
	if err != nil {
		return err
	}

	// Execute 'go mod tidy'.
	if flags.x {
		_, _ = fmt.Fprintln(os.Stderr, "go mod tidy")
	}
	_, err = execGoCommand(modDir, "mod", "tidy")
	if err != nil {
		return fmt.Errorf("go init: %s", err)
	}

	return nil
}

func stdlib() (err error) {
	for _, path := range stdLibPaths() {
		_, err = fmt.Println(path)
		if err != nil {
			break
		}
	}
	return err
}

// downloadModule downloads a module, if not cached, given path and version
// and returns the local directory of its source and the version. workDir is
// the working directory and flags are the command flags.
func downloadModule(path, version, workDir string, flags buildFlags) (string, string, error) {

	// Create the go.mod file for 'go download'.
	dir := filepath.Join(workDir, "download")
	sep := string(os.PathSeparator)
	if flags.x {
		_, _ = fmt.Fprintf(os.Stderr, "mkdir -p $WORK%sdownload\n", sep)
	}
	err := os.Mkdir(dir, 0777)
	if err != nil {
		return "", "", fmt.Errorf("scriggo: can't make diretory %s: %s", dir, err)
	}
	goModPath := filepath.Join(dir, "go.mod")
	goModData := "module scriggo.download\nrequire " + path + " " + version
	if flags.x {
		_, _ = fmt.Fprintf(os.Stderr, "cat >$WORK%sdownload%sgo.mod << 'EOF'\n%s\nEOF\n", sep, sep, goModData)
	}
	err = os.WriteFile(goModPath, []byte(goModData), 0666)
	if err != nil {
		return "", "", err
	}

	// Download the module.
	type jsonModule struct {
		Version string
		Dir     string
	}
	if flags.x {
		_, _ = fmt.Fprintf(os.Stderr, "chdir $WORK%sdownload\n", sep)
		_, _ = fmt.Fprintln(os.Stderr, "go mod download -json")
	}
	out, err := execGoCommand(dir, "mod", "download", "-json")
	if err != nil {
		if e, ok := err.(stdError); ok {
			s := e.Error()
			if strings.Contains(s, "go: errors parsing go.mod:\n") {
				if strings.Contains(s, "invalid module version \"latest\":") {
					return "", "", fmt.Errorf("scriggo: can't find module %s", path)
				} else if strings.Contains(s, "invalid module version \""+version+"\":") {
					return "", "", fmt.Errorf("scriggo: can't find version %s of module %s", version, path)
				}
			}
		}
		return "", "", err
	}

	// Read the module's directory.
	dec := json.NewDecoder(out)
	mod := &jsonModule{}
	err = dec.Decode(mod)
	if err != nil {
		return "", "", fmt.Errorf("scriggo: can't read response from 'go mod download': %v", err)
	}

	return mod.Dir, mod.Version, nil
}

type stdError string

func (e stdError) Error() string {
	return string(e)
}

// execGoCommand executes the command 'go' with dir as current directory and
// args as arguments. Returns the standard output if no error occurs.
func execGoCommand(dir string, args ...string) (out io.Reader, err error) {
	if os.Getenv("GO111MODULE") != "on" {
		panic("GO111MODULE must be 'on'")
	}
	cmd := exec.Command("go", args...)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Dir = dir
	err = cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return nil, stdError(stderr.String())
		}
		return nil, err
	}
	return stdout, nil
}

// stdLibPaths returns a copy of stdlibPaths with the packages for the runtime
// Go version.
func stdLibPaths() []string {
	version := goBaseVersion(runtime.Version())
	paths := make([]string, 0, len(stdlibPaths))
	for _, path := range stdlibPaths {
		switch path {
		case "debug/buildinfo", "net/netip":
			if version != "go1.18" {
				continue
			}
		}
		paths = append(paths, path)
	}
	return paths
}

// stdlibPaths contains the paths of the packages of the Go standard library
// except the packages "database", "plugin", "testing", "runtime/cgo",
// "runtime/race",  "syscall", "unsafe" and their sub packages.
var stdlibPaths = []string{
	"archive/tar",
	"archive/zip",
	"bufio",
	"bytes",
	"compress/bzip2",
	"compress/flate",
	"compress/gzip",
	"compress/lzw",
	"compress/zlib",
	"container/heap",
	"container/list",
	"container/ring",
	"context",
	"crypto",
	"crypto/aes",
	"crypto/cipher",
	"crypto/des",
	"crypto/dsa",
	"crypto/ecdsa",
	"crypto/elliptic",
	"crypto/hmac",
	"crypto/md5",
	"crypto/rand",
	"crypto/rc4",
	"crypto/rsa",
	"crypto/sha1",
	"crypto/sha256",
	"crypto/sha512",
	"crypto/subtle",
	"crypto/tls",
	"crypto/x509",
	"crypto/x509/pkix",
	"debug/buildinfo", // Go version 1.18
	"debug/dwarf",
	"debug/elf",
	"debug/gosym",
	"debug/macho",
	"debug/pe",
	"debug/plan9obj",
	"encoding",
	"encoding/ascii85",
	"encoding/asn1",
	"encoding/base32",
	"encoding/base64",
	"encoding/binary",
	"encoding/csv",
	"encoding/gob",
	"encoding/hex",
	"encoding/json",
	"encoding/pem",
	"encoding/xml",
	"errors",
	"expvar",
	"flag",
	"fmt",
	"go/ast",
	"go/build",
	"go/constant",
	"go/doc",
	"go/format",
	"go/importer",
	"go/parser",
	"go/printer",
	"go/scanner",
	"go/token",
	"go/types",
	"hash",
	"hash/adler32",
	"hash/crc32",
	"hash/crc64",
	"hash/fnv",
	"hash/maphash",
	"html",
	"html/template",
	"image",
	"image/color",
	"image/color/palette",
	"image/draw",
	"image/gif",
	"image/jpeg",
	"image/png",
	"index/suffixarray",
	"io",
	"io/fs",
	"io/ioutil",
	"log",
	"log/syslog",
	"math",
	"math/big",
	"math/bits",
	"math/cmplx",
	"math/rand",
	"mime",
	"mime/multipart",
	"mime/quotedprintable",
	"net",
	"net/http",
	"net/http/cgi",
	"net/http/cookiejar",
	"net/http/fcgi",
	"net/http/httptest",
	"net/http/httptrace",
	"net/http/httputil",
	"net/http/pprof",
	"net/mail",
	"net/netip", // Go version 1.18
	"net/rpc",
	"net/rpc/jsonrpc",
	"net/smtp",
	"net/textproto",
	"net/url",
	"os",
	"os/exec",
	"os/signal",
	"os/user",
	"path",
	"path/filepath",
	"reflect",
	"regexp",
	"regexp/syntax",
	"runtime",
	"runtime/debug",
	"runtime/metrics",
	"runtime/pprof",
	"runtime/trace",
	"sort",
	"strconv",
	"strings",
	"sync",
	"sync/atomic",
	"text/scanner",
	"text/tabwriter",
	"text/template",
	"text/template/parse",
	"time",
	"time/tzdata",
	"unicode",
	"unicode/utf16",
	"unicode/utf8",
}
