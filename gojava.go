package main

import (
	"go/build"
	"path/filepath"
	"reflect"

	"fmt"
	"go/importer"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"strings"

	"io/ioutil"

	"archive/zip"
	"runtime"

	"flag"
	"github.com/sridharv/gomobile-java/bind"
)

func runCommand(cmd string, args ...string) error {
	if out, err := exec.Command(cmd, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %v: %s", cmd, strings.Join(args, " "), err, string(out))
	}
	return nil
}

var javaHome = os.Getenv("JAVA_HOME")
var cwd string

func initBuild() (string, func(), error) {
	if javaHome == "" {
		return "", nil, fmt.Errorf("$JAVA_HOME not set")
	}
	var err error
	if cwd, err = os.Getwd(); err != nil {
		return "", nil, err
	}
	tmpDir, err := ioutil.TempDir("", "gojava")
	if err != nil {
		return "", nil, err
	}
	return tmpDir, func() {
		os.RemoveAll(tmpDir)
		os.Chdir(cwd)
	}, nil
}

func loadExportData(pkgs []string) ([]*types.Package, error) {
	// Load export data for the packages
	if err := runCommand("go", append([]string{"install"}, pkgs...)...); err != nil {
		return nil, err
	}
	typePkgs := make([]*types.Package, len(pkgs))

	for i, p := range pkgs {
		buildPkg, err := build.Import(p, cwd, build.AllowBinary)
		if err != nil {
			return nil, err
		}
		if typePkgs[i], err = importer.Default().Import(buildPkg.ImportPath); err != nil {
			return nil, err
		}
	}
	return typePkgs, nil
}

func createDirs(dirs ...string) error {
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0700); err != nil {
			return err
		}
	}
	return nil
}

func bindPackages(bindDir, javaDir string, pkgs []*types.Package) ([]string, error) {
	fs, javaFiles := token.NewFileSet(), make([]string, 0)
	for _, p := range pkgs {
		goFile := filepath.Join(bindDir, "go_"+p.Name()+"main.go")
		f, err := os.OpenFile(goFile, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, fmt.Errorf("failed to open: %s: %v", goFile, err)
		}
		conf := &bind.GeneratorConfig{Writer: f, Fset: fs, Pkg: p, AllPkg: pkgs}
		if err := bind.GenGo(conf); err != nil {
			return nil, fmt.Errorf("failed to bind %s:%v", p.Name(), err)
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
		javaFile := strings.Title(p.Name()) + ".java"
		if err := bindJava(javaDir, javaFile, conf, int(bind.Java)); err != nil {
			return nil, err
		}
		if err := bindJava(bindDir, "java_"+p.Name()+".c", conf, int(bind.JavaC)); err != nil {
			return nil, err
		}
		if err := bindJava(bindDir, p.Name()+".h", conf, int(bind.JavaH)); err != nil {
			return nil, err
		}
		javaFiles = append(javaFiles, filepath.Join(javaDir, javaFile))
	}
	return javaFiles, nil
}

func addExtraFiles(javaDir, scanDir string) error {
	if scanDir == "" {
		return nil
	}
	var files []filePair
	filepath.Walk(scanDir+"/", func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		fileName, err := filepath.Rel(scanDir, path)
		if err != nil {
			return err
		}
		if strings.HasSuffix(fileName, ".java") {
			files = append(files, filePair{filepath.Join(javaDir, fileName), path})
		}
		return nil
	})
	return copyFiles(files)
}

func createSupportFiles(bindDir, javaDir, mainFile string) error {
	bindPkg, err := build.Import(reflect.TypeOf(bind.ErrorList{}).PkgPath(), "", build.FindOnly)
	if err != nil {
		return err
	}
	bindJavaPkgDir := filepath.Join(bindPkg.Dir, "java")
	toCopy := []filePair{
		{filepath.Join(bindDir, "seq.go"), filepath.Join(bindPkg.Dir, "seq.go.support")},
		{filepath.Join(bindDir, "seq_java.go"), filepath.Join(bindJavaPkgDir, "seq_android.go.support")},
		{filepath.Join(bindDir, "seq.c"), filepath.Join(bindJavaPkgDir, "seq_android.c.support")},
		{filepath.Join(bindDir, "seq.h"), filepath.Join(bindJavaPkgDir, "seq.h")},
		{filepath.Join(javaDir, "Seq.java"), filepath.Join(bindJavaPkgDir, "Seq.java")},
		{filepath.Join(javaDir, "LoadJNI.java"), filepath.Join(bindPkg.Dir, "..", "..", "gojava", "LoadJNI.java")},
	}
	if err := copyFiles(toCopy); err != nil {
		return err
	}
	if err := ioutil.WriteFile(mainFile, []byte(fmt.Sprintf(javaMain, bindPkg.ImportPath)), 0600); err != nil {
		return err
	}
	inc1, inc2 := filepath.Join(javaHome, "include"), filepath.Join(javaHome, "include", runtime.GOOS)
	flagFile := filepath.Join(bindDir, "gojavacimport.go")
	if err := ioutil.WriteFile(flagFile, []byte(fmt.Sprintf(javaInclude, inc1, inc2)), 0600); err != nil {
		return err
	}
	return nil
}

func buildGo(classDir, mainDir string) error {
	dylib := filepath.Join(classDir, "libgojava")
	if err := os.Chdir(mainDir); err != nil {
		return err
	}
	return runCommand("go", "build", "-o", dylib, "-buildmode=c-shared", ".")
}

func buildJava(jarDir, javaDir string, javaFiles []string) error {
	if err := os.Chdir(javaDir); err != nil {
		return err
	}
	javaFiles = append(javaFiles, filepath.Join(javaDir, "Seq.java"), filepath.Join(javaDir, "LoadJNI.java"))
	return runCommand("javac", append([]string{
		"-d", jarDir,
		"-sourcepath", filepath.Join(javaDir, ".."),
	}, javaFiles...)...)
}

func createJar(target, jarDir string) error {
	if err := os.Chdir(cwd); err != nil {
		return err
	}
	t, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	w := zip.NewWriter(t)
	fmt.Printf("Building %s\n", target)
	if err := filepath.Walk(jarDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		fileName, err := filepath.Rel(jarDir, path)
		fmt.Printf("Adding %s\n", fileName)
		if err != nil {
			return err
		}
		f, err := w.Create(fileName)
		if err != nil {
			return err
		}
		d, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := f.Write(d); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	if err := t.Close(); err != nil {
		return err
	}
	fmt.Printf("Finished building %s\n", target)
	return nil
}

func bindToJar(target string, scanDir string, pkgs ...string) error {
	tmpDir, cleanup, err := initBuild()
	if err != nil {
		return err
	}
	defer cleanup()

	typePkgs, err := loadExportData(pkgs)
	if err != nil {
		return err
	}

	bindDir := filepath.Join(tmpDir, "gojava_bind")
	mainDir := filepath.Join(bindDir, "main")
	mainFile := filepath.Join(mainDir, "main.go")
	javaDir := filepath.Join(tmpDir, "src/go")
	jarDir := filepath.Join(tmpDir, "classes")
	classDir := filepath.Join(tmpDir, "classes/go")

	if err := createDirs(classDir, javaDir, mainDir); err != nil {
		return err
	}

	javaFiles, err := bindPackages(bindDir, javaDir, typePkgs)
	if err != nil {
		return err
	}
	if err := addExtraFiles(javaDir, scanDir); err != nil {
		return err
	}
	if err := createSupportFiles(bindDir, javaDir, mainFile); err != nil {
		return err
	}

	if err := buildGo(classDir, mainDir); err != nil {
		return err
	}
	if err := buildJava(jarDir, javaDir, javaFiles); err != nil {
		return err
	}
	return createJar(target, jarDir)
}

func copyFile(dst, src string) error {
	d, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(dst, d, 0600)
}

type filePair struct {
	dst string
	src string
}

func copyFiles(files []filePair) error {
	for _, p := range files {
		if err := copyFile(p.dst, p.src); err != nil {
			return err
		}
	}
	return nil
}

func bindJava(dir, file string, conf *bind.GeneratorConfig, ft int) error {
	path := filepath.Join(dir, file)
	w, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("failed to generate %s: %v", path, err)
	}
	conf.Writer = w
	defer func() { conf.Writer = nil }()

	switch ft {
	case int(bind.Java):
		err = bind.GenJava(conf, "", bind.Java)
	case int(bind.JavaH):
		err = bind.GenJava(conf, "", bind.JavaH)
	case int(bind.JavaC):
		err = bind.GenJava(conf, "", bind.JavaC)
	default:
		err = fmt.Errorf("unsupported bind type: %d", ft)
	}
	if err != nil {
		return err
	}
	return w.Close()
}

const javaInclude = `package gojava_bind

// #cgo CFLAGS: -Wall -I%s -I%s
import "C"

`
const javaMain = `package main

import (
	_ %q
	_ ".."
)

func main() {}
`

const usage = `gojava is a tool for creating Java bindings to Go

Usage:

	gojava [-o <jar>] [-s <dir>] build [<pkg1>, [<pkg2>...]]

This generates a jar containing Java bindings to the specified Go packages.
`

func main() {
	o := flag.String("o", "libgojava.jar", "Path to the generated jar file")
	s := flag.String("s", "", "Additional path to scan for Java files")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, usage)
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 2 || flag.Args()[0] != "build" {
		flag.Usage()
		os.Exit(1)
	}
	if err := bindToJar(*o, *s, flag.Args()[1:]...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
