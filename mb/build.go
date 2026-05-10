package mb

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/melbahja/goph"
	"github.com/otiai10/copy"
)

func deployHost(h string) error {
	logMsgf("DEPLOY", "Deploying to host %q.", h)
	host := cfgFile.Hosts[h]

	target, ok := cfgFile.Targets[host.Target]
	if !ok {
		return fmt.Errorf("target %q not found for host %q", host.Target, h)
	}

	var auth goph.Auth
	switch host.Auth {
	case "password":
		auth = goph.Password(host.Passwd)
	default:
		logErrf("ERROR", "Invalid auth type %q (host %q)", host.Auth, h)
		return fmt.Errorf("invalid auth type %q", host.Auth)
	}

	client, err := goph.New(host.User, host.Addr, auth)

	if err != nil {
		return err
	}

	defer client.Close()

	if target.NoInstaller {
		binaryName := fmt.Sprintf("%s_%s", cfgFile.BinaryName, host.Target)
		localBinary := filepath.Join(cfgFile.BuildDir, "bin", binaryName)
		remotePath := filepath.Join(host.DeployPath, binaryName)

		logMsgf("DEPLOY", "Uploading binary (no installer) to %q", h)

		if host.CreateDeployPath {
			_, err := client.Run(fmt.Sprintf("mkdir -p %s", host.DeployPath))
			if err != nil {
				return err
			}
		}

		if err := client.Upload(localBinary, remotePath); err != nil {
			return err
		}
		if _, err := client.Run(fmt.Sprintf("chmod +x %s", remotePath)); err != nil {
			return err
		}
		return nil
	}

	installerName := fmt.Sprintf("install_%s", host.Target)

	installerPath := filepath.Join(cfgFile.BuildDir, "dist", installerName)
	targetPath := filepath.Join(host.DeployPath, installerName)

	logMsgf("DEPLOY", "Uploading installer to remote %q", h)

	if host.CreateDeployPath {
		_, err := client.Run(fmt.Sprintf("mkdir -p %s", host.DeployPath))
		if err != nil {
			return err
		}
	}

	err = client.Upload(installerPath, targetPath)

	if err != nil {
		return err
	}

	_, err = client.Run(fmt.Sprintf("chmod +x %s", targetPath))

	if err != nil {
		return err
	}

	if host.RunInstaller {
		ssh, err := client.NewSession()

		if err != nil {
			return err
		}
		defer ssh.Close()

		sout, err := ssh.StdoutPipe()
		if err != nil {
			return err
		}
		serr, err := ssh.StderrPipe()
		if err != nil {
			return err
		}
		logMsgf("DEPLOY", "Running installer on remote %q", h)
		err = ssh.Start(
			fmt.Sprintf(
				"cd %s && ./%s",
				host.DeployPath,
				installerName,
			),
		)

		if err != nil {
			return err
		}

		grayFaint := color.New(color.FgWhite, color.Faint)

		combined := combineReadersWithPrefix(sout, serr, PrefixConfig{
			StdoutPrefix: grayFaint.Sprint(fmt.Sprintf("|%15s: stdout | ", h)),
			StderrPrefix: grayFaint.Sprint(fmt.Sprintf("|%15s: stderr | ", h)),
		})
		io.Copy(os.Stdout, combined)
		err = ssh.Wait()
		if err != nil {
			return err
		}

	}
	return nil
}

func buildTargetByName(tgt string) error {
	target, ok := cfgFile.Targets[tgt]
	if !ok {
		return fmt.Errorf("no such target: %s", tgt)
	}

	binaryName := fmt.Sprintf("%s_%s", cfgFile.BinaryName, tgt)
	logMsgf("BUILD", "Building target %q.", binaryName)

	binDir := filepath.Join(cfgFile.BuildDir, "bin")
	if !dirExists(binDir) {
		os.MkdirAll(binDir, 0700)
	}
	outPath := filepath.Join(binDir, binaryName)

	oldID := ""
	if fileExists(outPath) {
		cmd := exec.Command("go", "tool", "buildid", outPath)
		out, err := cmd.Output()
		if err == nil {
			oldID = strings.TrimSpace(string(out))
		}
	}

	env := map[string]string{
		"GOARCH": target.Arch,
		"GOOS":   target.Os,
	}
	maps.Copy(env, target.Env)

	args := []string{"build"}
	args = append(args, "-o", outPath)
	if len(target.CompilerFlags) != 0 {
		args = append(args, target.CompilerFlags...)
	}
	args = append(args, target.MainPackage)

	if err := sh.RunWithV(env, "go", args...); err != nil {
		return err
	}

	cmd := exec.Command("go", "tool", "buildid", outPath)
	newOut, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get new build ID: %w", err)
	}
	newID := strings.TrimSpace(string(newOut))

	if oldID != newID {
		stampFile := outPath + ".stamp"
		if err := os.WriteFile(stampFile, []byte(newID), 0644); err != nil {
			return err
		}
		logMsgf("CACHE", "Binary %q changed (new build ID).", binaryName)
	} else {
		logMsgf("CACHE", "Binary %q unchanged (cache hit).", binaryName)
	}

	return nil
}

func prepareBundle() error {
	bundlePath := fmt.Sprintf("%s/bundle.zip", cfgFile.BuildDir)

	sources := []string{}
	sources = append(sources, cfgFile.BundledPaths...)
	sources = append(sources, ConfigFilePath)

	newer, err := isAnySourceNewer(bundlePath, sources)

	if err != nil {
		return err
	}

	if !newer {
		return nil
	}

	logMsg("BUNDLE", "Creating bundle.")

	mg.Deps(createBuildDir)
	zipFile, err := os.Create(bundlePath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, dir := range cfgFile.BundledPaths {
		baseName := filepath.Base(dir)
		if baseName == "." || baseName == "/" {
			return fmt.Errorf("invalid directory base for %q", dir)
		}

		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relPath, err := filepath.Rel(dir, path)

			if err != nil {
				return fmt.Errorf("failed to get relative path for %q: %w", path, err)
			}

			if relPath == "." {
				return nil
			}

			zipEntryName := filepath.Join(baseName, relPath)
			zipEntryName = filepath.ToSlash(zipEntryName)

			if d.IsDir() {
				_, err = zipWriter.Create(zipEntryName + "/")
				if err != nil {
					return fmt.Errorf("failed to create zip directory entry: %w", err)
				}
				return nil
			}

			srcFile, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open %q: %w", path, err)
			}
			defer srcFile.Close()

			destFile, err := zipWriter.Create(zipEntryName)
			if err != nil {
				return fmt.Errorf("failed to create zip entry for %q: %w", zipEntryName, err)
			}

			if _, err := io.Copy(destFile, srcFile); err != nil {
				return fmt.Errorf("failed to copy %q to zip: %w", path, err)
			}
			return nil
		})

		if err != nil {
			return fmt.Errorf("error walking directory %q: %w", dir, err)
		}

	}
	return nil
}

func createInstallers() error {
	src := cfgFile.InstallersPath
	dest := filepath.Join(cfgFile.BuildDir, src)

	copy.Copy(src, dest)

	distDir := filepath.Join(cfgFile.BuildDir, "dist")

	if !dirExists(distDir) {
		os.MkdirAll(distDir, 0700)
	}

	for k, v := range cfgFile.Targets {
		target := v

		if target.NoInstaller {
			continue
		}

		installCfg := cfgFile.Installers[v.Installer]
		insPath := filepath.Join(cfgFile.BuildDir, installCfg.Path)

		binPath := filepath.Join(cfgFile.BuildDir, "bin", fmt.Sprintf("%s_%s", cfgFile.BinaryName, k))
		bundlePath := filepath.Join(cfgFile.BuildDir, "bundle.zip")

		outFile := filepath.Join(distDir, fmt.Sprintf("install_%s", k))

		newer, err := isAnySourceNewer(outFile, []string{
			bundlePath,
			binPath + ".stamp",
			src,
			ConfigFilePath,
		})

		if err != nil {
			return err
		}

		if !newer {
			continue
		}

		logMsgf("INSTALL", "Building installer for target %q.", k)

		// --- New: remove any existing stubs so os.Link won't fail ---
		for _, fname := range []string{"binary", "bundle"} {
			fpath := filepath.Join(insPath, fname)
			if err := os.Remove(fpath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("removing old installer artefact %s: %w", fpath, err)
			}
		}

		//err = sh.RunV("ln", binPath, filepath.Join(insPath, "binary"))
		err = os.Link(binPath, filepath.Join(insPath, "binary"))
		if err != nil {
			return err
		}

		//err = sh.RunV("ln", bundlePath, filepath.Join(insPath, "bundle"))
		err = os.Link(bundlePath, filepath.Join(insPath, "bundle"))
		if err != nil {
			return err
		}

		env := map[string]string{
			"GOARCH": target.Arch,
			"GOOS":   target.Os,
		}

		maps.Copy(env, target.Env)

		args := []string{
			"build",
		}
		args = append(args, "-o", outFile)

		if len(target.CompilerFlags) != 0 {
			args = append(args, target.CompilerFlags...)
		}

		route := fmt.Sprintf("./%s", filepath.Join(insPath, installCfg.MainFile))
		args = append(args, route)

		err = sh.RunWithV(env, "go", args...)
		if err != nil {
			return err
		}
		sh.RunV("rm", filepath.Join(insPath, "binary"))
		sh.RunV("rm", filepath.Join(insPath, "bundle"))
	}

	return nil
}
