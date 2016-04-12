## GoJava - Java bindings to Go packages [![Build Status](https://travis-ci.org/sridharv/gojava.svg?branch=master)](https://travis-ci.org/sridharv/gojava)

GoJava uses a [forked version of gomobile](https://github.com/sridharv/gomobile-java) to generate Java bindings to Go packages.
The same set of types are supported. Details on how the binding works can be found [here](https://godoc.org/golang.org/x/mobile/cmd/gobind).

### Usage

```
	gojava [-v] [-o <jar>] [-s <dir>] build [<pkg1>, [<pkg2>...]]

	This generates a jar containing Java bindings to the specified Go packages.

	-o string
	    Path to write the generated jar file. (default "libgojava.jar")
	-s string
	    Additional path to scan for Java files. These files will be compiled and
	    included in the final jar.
	-v  Verbose output.
```

You can include the generated jar in your build using the build tool of your choice.
The jar contains a native library (built for the build platform) which is loaded automatically.
Cross platform builds are not currently supported.

NOTE: This has only been tested on an OSX developer machine and Linux (on Travis) and not in production.
