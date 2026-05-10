package mb

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

func createBuildDir() error {
	if dirExists(cfgFile.BuildDir) {
		return nil
	}
	logMsg("DIR", "Creating build directory.")
	return os.MkdirAll(cfgFile.BuildDir, 0700)
}

// Cleans all build artifacts
func Clean() {
	mg.Deps(loadConfig)
	logMsg("CLEAN", "Clearing all build artifacts.")
	os.RemoveAll(cfgFile.BuildDir)
}

// Generates all build artifacts
func Build() {
	mg.Deps(loadConfig)
	mg.Deps(createBuildDir)

	tgts := make([]any, 0, len(cfgFile.Targets))
	for k := range cfgFile.Targets {
		tgts = append(tgts, mg.F(func(v string) error {
			return buildTargetByName(v)
		}, k))
	}
	mg.Deps(tgts...)
	mg.Deps(prepareBundle)
	mg.Deps(createInstallers)
}

// Deploys to host. If passed 'all', will deploy to all hosts.
func Deploy(host string) {
	mg.Deps(loadConfig)
	mg.Deps(Build)

	switch host {
	case "all":
		for k := range cfgFile.Hosts {
			err := deployHost(k)
			if err != nil {
				logErrf("ERROR", "Unable to deploy to host %q: %v.", k, err.Error())
			}
		}
	default:
		_, ok := cfgFile.Hosts[host]
		if !ok {
			logErrf("ERROR", "Host %q is not defined.", host)
			return
		}
		err := deployHost(host)
		if err != nil {
			logErrf("ERROR", "Unable to deploy to host %q: %v.", host, err.Error())
		}
	}
}

// Creates an example configuration file
func Example() {
	path := fmt.Sprintf("example.%s", ConfigFilePath)

	logMsgf("CONF", "Creating example configuration file: %q.", path)

	f, err := os.Create(path)
	if err != nil {
		logErr("ERROR", err.Error())
		os.Exit(0)
	}
	defer f.Close()
	example := generateExample(&ConfFile{})

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "\t")
	encoder.Encode(example)
}

// Runs a runConfiguration through 'go run'. Selects config 'default' by default.
func Run(cfg *string) {
	mg.Deps(loadConfig)
	var cfgSelector string
	switch cfg {
	case nil:
		cfgSelector = "default"
	default:
		cfgSelector = *cfg
	}

	c, ok := cfgFile.RunConfigs[cfgSelector]

	if !ok {
		logErrf("ERROR", "Run config %q does not exist.", cfgSelector)
		return
	}

	args := []string{"run"}
	args = append(args, c.GoFlags...)
	args = append(args, c.MainPackage)

	logMsgf("RUN", "Running config %q.", cfgSelector)

	sh.RunWithV(c.Env, "go", args...)
}
