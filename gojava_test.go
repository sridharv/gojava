package main

import (
	"testing"

	"io/ioutil"
	"path/filepath"
	"os/exec"
	"os"
	"flag"
)

var javaTest = flag.String("javatest", ".*", "Run only java tests matching the regular expression")

func TestJavaBind(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "gojavatest")
	if err != nil {
		t.Fatal(err)
	}
	jar := filepath.Join(tmpDir, "gojavatest.jar")
	if err := bindToJar(jar,
		"github.com/sridharv/gomobile-java/bind/testpkg",
		"github.com/sridharv/gomobile-java/bind/testpkg/secondpkg",
		"github.com/sridharv/gomobile-java/bind/testpkg/simplepkg",
	); err != nil {
		t.Fatal(err)
	}

	toCopy := []filePair{
		{filepath.Join(tmpDir, "MoreAsserts.java"), "MoreAsserts.java"},
		{filepath.Join(tmpDir, "SeqTest.java"), "../gomobile-java/bind/java/SeqTest.java"},
	}
	if err := copyFiles(toCopy); err != nil {
		t.Fatal(err)
	}
	if err := runCommand("javac", "-cp", jar, "-d", tmpDir, toCopy[0].dst, toCopy[1].dst); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("java", "-cp", jar + ":" + tmpDir, "go.MoreAsserts", *javaTest)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}
