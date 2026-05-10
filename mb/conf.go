package mb

import (
	"bytes"
	"encoding/json"
	"os"
)

const (
	ConfigFilePath = "mbuild.json"
)

func loadConfig() {
	logMsgf("CONF", "Loading configuration file: %q.", ConfigFilePath)
	cfg, err := os.ReadFile(ConfigFilePath)

	if err != nil {
		logErr("ERROR", err.Error())
		os.Exit(1)
	}

	dec := json.NewDecoder(bytes.NewReader(cfg))
	dec.DisallowUnknownFields()
	err = dec.Decode(&cfgFile)

	if err != nil {
		logErr("ERROR", err.Error())
		os.Exit(1)
	}
}

var (
	cfgFile ConfFile
)

type ConfFile struct {
	BuildDir       string   `json:"buildDir"`
	BinaryName     string   `json:"binaryName"`
	BundledPaths   []string `json:"bundledPaths"`
	InstallersPath string   `json:"installersPath"`

	RunConfigs map[string]RunConfig `json:"runConfigs"`
	Targets    map[string]Target    `json:"targets"`
	Hosts      map[string]Host      `json:"hosts"`
	Installers map[string]Installer `json:"installers"`
}

type RunConfig struct {
	Env         map[string]string `json:"env"`
	GoFlags     []string          `json:"goFlags"`
	MainPackage string            `json:"mainPackage"`
	Args        []string          `json:"args"`
}

type Target struct {
	Arch          string            `json:"arch"`
	Os            string            `json:"os"`
	CompilerFlags []string          `json:"compFlags"`
	Env           map[string]string `json:"env"`
	MainPackage   string            `json:"mainPackage"`
	Installer     string            `json:"installer"`
}

type Host struct {
	Target  string `json:"target"`
	Addr    string `json:"address"`
	Port    uint16 `json:"port"`
	User    string `json:"user"`
	Auth    string `json:"auth"`
	Passwd  string `json:"password"`
	KeyPath string `json:"keyPath"`

	DeployPath       string `json:"deployPath"`
	CreateDeployPath bool   `json:"createDeployPath"`
	RunInstaller     bool   `json:"runInstaller"`
}

type Installer struct {
	Path     string `json:"path"`
	MainFile string `json:"mainFile"`
}
