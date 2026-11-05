package main

import (
	"embed"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 支持把文件夹拖到 exe 图标上启动，或命令行带目录参数启动
	startupDir := ""
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		startupDir = strings.Trim(arg, `"`)
	}

	app := NewApp(startupDir)

	winTheme := windows.Dark
	if app.settings.Theme == "light" {
		winTheme = windows.Light
	}

	err := wails.Run(&options.App{
		Title:     "GitDesk",
		Width:     1120,
		Height:    760,
		MinWidth:  880,
		MinHeight: 580,
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true, // 拖放统一由 Wails 事件处理，避免 WebView 直接打开文件
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			Theme:                winTheme,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
