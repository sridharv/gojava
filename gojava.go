package gojava

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

	"github.com/sridharv/gomobile-java/bind"
)

func runCommandWithEnv(env []string, cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Env = append(os.Environ(), env...)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s %s: %v: %s", strings.Join(env, " "), cmd, strings.Join(args, " "), err, string(out))
	}
	return nil
}

func runCommand(cmd string, args ...string) error {
	return runCommandWithEnv([]string{}, cmd, args...)
}

func createJar(target string, pkgs ...string) error {
	javaHome := os.Getenv("JAVA_HOME")
	if javaHome == "" {
		return fmt.Errorf("$JAVA_HOME not set")
	}
	tmpDir, err := ioutil.TempDir("", "gojava")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	// Load export data for the packages
	if err := runCommand("go", append([]string{"install"}, pkgs...)...); err != nil {
		return err
	}
	typePkgs := make([]*types.Package, len(pkgs))

	for i, p := range pkgs {
		var err error
		if typePkgs[i], err = importer.Default().Import(p); err != nil {
			return err
		}
	}

	bindDir := filepath.Join(tmpDir, "gojava_bind")
	mainDir := filepath.Join(bindDir, "main")
	mainFile := filepath.Join(mainDir, "main.go")
	javaDir := filepath.Join(tmpDir, "src/go")
	classDir := filepath.Join(tmpDir, "classes/go")
	for _, d := range []string{classDir, javaDir, mainDir} {
		if err := os.MkdirAll(d, 0700); err != nil {
			return err
		}
	}
	javacArgs := []string{"-d", filepath.Join(tmpDir, "classes"), "-sourcepath", filepath.Join(tmpDir, "src")}

	fs := token.NewFileSet()
	for _, p := range typePkgs {
		goFile := filepath.Join(bindDir, "go_"+p.Name()+"main.go")
		f, err := os.OpenFile(goFile, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return fmt.Errorf("failed to open: %s: %v", goFile, err)
		}
		conf := &bind.GeneratorConfig{Writer: f, Fset: fs, Pkg: p, AllPkg: typePkgs}
		if err := bind.GenGo(conf); err != nil {
			return fmt.Errorf("failed to bind %s:%v", p.Name(), err)
		}
		if err := f.Close(); err != nil {
			return err
		}
		javaFile := strings.Title(p.Name()) + ".java"
		if err := bindJava(javaDir, javaFile, conf, int(bind.Java)); err != nil {
			return err
		}
		if err := bindJava(bindDir, "java_"+p.Name()+".c", conf, int(bind.JavaC)); err != nil {
			return err
		}
		if err := bindJava(bindDir, p.Name()+".h", conf, int(bind.JavaH)); err != nil {
			return err
		}
		javacArgs = append(javacArgs, filepath.Join(javaDir, javaFile))
	}

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
	dylib := filepath.Join(classDir, "libgojava")
	back, err := cdTo(mainDir)
	if err != nil {
		return err
	}
	defer back()
	if err := runCommand("go", "build", "-o", dylib, "-buildmode=c-shared", "."); err != nil {
		return err
	}
	if err := os.Chdir(javaDir); err != nil {
		return err
	}
	javacArgs = append(javacArgs, filepath.Join(javaDir, "Seq.java"), filepath.Join(javaDir, "LoadJNI.java"))
	if err := runCommand("javac", javacArgs...); err != nil {
		return err
	}
	t, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	w := zip.NewWriter(t)
	jarDir := filepath.Join(tmpDir, "classes")
	if err := filepath.Walk(jarDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		fileName, err := filepath.Rel(jarDir, path)
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
	return nil
}

func cdTo(target string) (func(), error) {
	cur, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(target); err != nil {
		return nil, err
	}
	return func() {
		if err := os.Chdir(cur); err != nil {
			panic(err)
		}
	}, nil
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
