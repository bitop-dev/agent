package plugin

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	loaderutil "github.com/ncecere/agent/internal/loader"
	plg "github.com/ncecere/agent/pkg/plugin"
)

func InstallLocal(source, destinationRoot string, link bool) (plg.Manifest, string, error) {
	manifestPath, sourceDir, err := resolveSource(source)
	if err != nil {
		return plg.Manifest{}, "", err
	}
	manifest, err := loaderutil.LoadYAML[plg.Manifest](manifestPath)
	if err != nil {
		return plg.Manifest{}, "", err
	}
	if err := ValidateManifest(manifest); err != nil {
		return plg.Manifest{}, "", err
	}
	if err := os.MkdirAll(destinationRoot, 0o755); err != nil {
		return plg.Manifest{}, "", err
	}
	destinationDir := filepath.Join(destinationRoot, manifest.Metadata.Name)
	if _, err := os.Lstat(destinationDir); err == nil {
		return plg.Manifest{}, "", fmt.Errorf("plugin %s already installed at %s", manifest.Metadata.Name, destinationDir)
	} else if !os.IsNotExist(err) {
		return plg.Manifest{}, "", err
	}
	if link {
		if err := os.Symlink(sourceDir, destinationDir); err != nil {
			return plg.Manifest{}, "", err
		}
	} else {
		if err := copyDir(sourceDir, destinationDir); err != nil {
			return plg.Manifest{}, "", err
		}
	}
	return manifest, destinationDir, nil
}

func RemoveInstalled(name, destinationRoot string) (string, error) {
	destinationDir := filepath.Join(destinationRoot, name)
	if _, err := os.Lstat(destinationDir); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("plugin %s is not installed in %s", name, destinationRoot)
		}
		return "", err
	}
	if err := os.RemoveAll(destinationDir); err != nil {
		return "", err
	}
	return destinationDir, nil
}

func resolveSource(source string) (manifestPath string, sourceDir string, err error) {
	absSource, err := filepath.Abs(source)
	if err != nil {
		return "", "", err
	}
	source = absSource
	info, err := os.Stat(source)
	if err != nil {
		return "", "", err
	}
	if info.IsDir() {
		for _, candidate := range []string{"plugin.yaml", "plugin.yml"} {
			manifestPath = filepath.Join(source, candidate)
			if _, err := os.Stat(manifestPath); err == nil {
				return manifestPath, source, nil
			}
		}
		return "", "", fmt.Errorf("no plugin manifest found in %s", source)
	}
	base := filepath.Base(source)
	if base != "plugin.yaml" && base != "plugin.yml" {
		return "", "", fmt.Errorf("plugin source must be a directory or plugin manifest")
	}
	return source, filepath.Dir(source), nil
}

func copyDir(sourceDir, destinationDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destinationDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(sourcePath, destinationPath string) error {
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer target.Close()
	if _, err := io.Copy(target, source); err != nil {
		return err
	}
	return target.Chmod(0o644)
}
