# MBuild

Multi-architecture, multi-host binary deployment system with autogenerating installers.


## Commands

```bash
$ mage -l
Targets:
  build      Generates all build artifacts
  clean      Cleans all build artifacts
  deploy     Deploys to host.
  example    Creates an example configuration file
```

## Setup

Import the library.

```bash
go get -u github.com/2000gmod/mbuild@latest
```

Create your _magefile_ and import the library's targets.

```go
// magefiles/mage.go or just a mage.go
package main

import (
    //mage:import
    _ "github.com/2000gmod/mbuild/mb"
)
```

Create a file named `mbuild.json`, this will be the main configuration.

Example:

```json
{
    "buildDir": "build",
    "binaryName": "mbtest",
    "bundledPaths": [],
    "installersPath": "installers",
    "targets": {
        "amd64-linux": {
            "arch": "amd64",
            "os": "linux",
            "compFlags": ["-trimpath", "-ldflags=-s -w"],
            "env": {},
            "mainPackage": ".",
            "installer": "generic"
        },
        "arm64-linux": {
            "arch": "arm64",
            "os": "linux",
            "compFlags": ["-trimpath", "-ldflags=-s -w"],
            "env": {},
            "mainPackage": ".",
            "installer": "generic"
        },
        "mipsle-linux": {
            "arch": "mipsle",
            "os": "linux",
            "compFlags": ["-trimpath", "-ldflags=-s -w"],
            "env": {
                "GOMIPS": "softfloat"
            },
            "mainPackage": ".",
            "installer": "generic"
        }
    },
    "hosts": {
        "raspberry1": {
            "target": "arm64-linux",
            "address": "192.168.1.100",
            "port": 22,
            "user": "michael",
            "auth": "password",
            "password": "12345",
            "keyPath": "",
            "deployPath": "/home/michael/mbtest",
            "createDeployPath": true,
            "runInstaller": true
        },
        "raspberry2": {
            "target": "arm64-linux",
            "address": "192.168.1.101",
            "port": 22,
            "user": "miguel",
            "auth": "password",
            "password": "12345",
            "keyPath": "",
            "deployPath": "/home/miguel/mbtest",
            "createDeployPath": true,
            "runInstaller": true
        }
    },
    "installers": {
        "generic": {
            "path": "installers/generic",
            "mainFile": "generic.go"
        }
    }
}

```

Create an installer template:

```go
//installers/generic/generic.go
package main

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/2000gmod/mbuild/iutils"
)

//go:embed bundle
var bundle []byte

//go:embed binary
var binary []byte

func main() {
	exe, err := os.Executable()
	if err != nil {
		panic(err)
	}
	installer := filepath.Base(exe)
	fmt.Println(installer)

	entries, err := os.ReadDir(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read directory: %v\n", err)
		os.Exit(1)
	}

	for _, entry := range entries {
		name := entry.Name()
		if name == installer {
			continue // skip the binary itself
		}
		// RemoveAll works for both files and directories.
		if err := os.RemoveAll(name); err != nil {
			fmt.Fprintf(os.Stderr, "failed to remove %s: %v\n", name, err)
			// Decide whether to stop or continue.
		}
	}

	bundleZip, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))

	if err != nil {
		panic(err)
	}
	fmt.Println("Extracting bundle")
	if err := iutils.ExtractZipFS(bundleZip, "."); err != nil {
		panic(err)
	}
	
	fmt.Println("Extracting binary")
	os.WriteFile("mbtest", binary, 0700)

	fmt.Println("Installation complete, ready to run :)")
}

```


Note: the embedded `bundle` and `binary` files contain a zip archive of the bundled assets and the main target binary respectivelly.

With this system, it is possible to embed arbitrary logic into the install process.
