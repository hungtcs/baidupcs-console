package main

import (
	"embed"
	"io/fs"
)

// 注意：embed 指令只能引用当前包目录或其子目录，所以内嵌声明必须放在仓库根包这里，
// 而不能放进 internal/httpapi。
//
// 用 all: 前缀是为了把 .gitkeep 也一起嵌进来——这样即使前端还没构建过，
// go build 也不会因为目录为空而失败。
//
//go:embed all:webapp/dist
var webAssets embed.FS

// webFS 返回以 webapp/dist 为根的前端产物文件系统。
func webFS() fs.FS {
	sub, err := fs.Sub(webAssets, "webapp/dist")
	if err != nil {
		panic(err)
	}
	return sub
}

// webBuilt 表示前端产物是否已经构建进二进制。
func webBuilt() bool {
	_, err := fs.Stat(webFS(), "index.html")
	return err == nil
}
