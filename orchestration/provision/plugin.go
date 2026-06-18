package provision

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const sdkImage = "mcr.microsoft.com/dotnet/sdk:10.0"

// reconcilePlugin rebuilds InspectGive when its source changed, the CSS API it
// links changed, or it isn't installed (a mod-bundle rebuild wipes addons/,
// taking the plugin with it). cssTag is the CSS release we just installed; the
// plugin always builds against it so its API matches the runtime.
func reconcilePlugin(p paths, rec receipt, cssTag string) (pluginReceipt, error) {
	hash, err := hashDir(p.pluginSrc)
	if err != nil {
		return rec.Plugin, fmt.Errorf("hash plugin source: %w", err)
	}

	_, statErr := os.Stat(filepath.Join(p.pluginsDst, "InspectGive"))
	if statErr == nil && hash == rec.Plugin.SourceHash && cssTag == rec.Plugin.BuiltAgainstCSS {
		logf("plugin up to date")
		return rec.Plugin, nil
	}

	if err := buildPlugin(p, cssTag); err != nil {
		return rec.Plugin, err
	}
	return pluginReceipt{SourceHash: hash, BuiltAgainstCSS: cssTag}, nil
}

func buildPlugin(p paths, cssTag string) error {
	if err := os.MkdirAll(p.pluginsDst, 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(p.pluginsDst, "InspectGive")); err != nil {
		return err
	}

	cssVersion := strings.TrimPrefix(cssTag, "v")
	logf("building InspectGive against CounterStrikeSharp.API %s with %s", cssVersion, sdkImage)
	uidGid := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	return run("docker", "run", "--rm",
		"--user", uidGid,
		"-e", "HOME=/tmp",
		"-e", "DOTNET_CLI_HOME=/tmp",
		"-e", "DOTNET_NOLOGO=1",
		"-e", "NUGET_PACKAGES=/tmp/nuget",
		"-v", p.pluginSrc+":/src:ro",
		"-v", p.pluginsDst+":/out",
		sdkImage,
		"bash", "-c",
		"cp -r /src /tmp/InspectGive && cd /tmp/InspectGive && dotnet publish -c Release -p:CssApiVersion="+cssVersion+" -o /out/InspectGive",
	)
}

// hashDir returns a stable content hash of dir.
func hashDir(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "bin", "obj":
				return filepath.SkipDir
			}
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)

	h := sha256.New()
	for _, f := range files {
		rel, _ := filepath.Rel(dir, f)
		io.WriteString(h, rel+"\x00")
		data, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
