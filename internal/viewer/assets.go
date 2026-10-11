package viewer

import (
	"embed"
	"io/fs"
	"strings"
)

//go:embed assets
var embedded embed.FS

//go:embed notices
var notices embed.FS

func assets() fs.FS { sub, _ := fs.Sub(embedded, "assets"); return sub }

func Licenses() string {
	var text strings.Builder
	text.WriteString("AHV Console license and third-party notices\nOriginal code: MIT, Copyright (c) 2026 Kurt Telep.\nBundled dependencies retain their own licenses.\nExact noVNC source is embedded at /novnc/SOURCE.tar.gz and included in release bundles.\n\n")
	fs.WalkDir(notices, "notices", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, e := notices.ReadFile(path)
		if e != nil {
			return e
		}
		text.WriteString("=== " + d.Name() + " ===\n")
		text.Write(b)
		text.WriteString("\n\n")
		return nil
	})
	return text.String()
}
