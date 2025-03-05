package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/* index.html
var webFS embed.FS

// GetFileSystem 返回嵌入的文件系统
func GetFileSystem() http.FileSystem {
	fsys, err := fs.Sub(webFS, ".")
	if err != nil {
		panic(err)
	}
	return http.FS(fsys)
}
