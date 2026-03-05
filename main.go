package main

import (
	"embed"
	"errors"
	"fmt"
	"log"
	"noti/internal/logging"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

var env string

// sentryEnabled reports whether Sentry should be active in this binary.
func sentryEnabled() bool { return env == "production" }

// fatalWithSentry captures msg to Sentry (flushing immediately) then calls
// log.Fatal so the process exits with a non-zero status code.
// Use this instead of log.Fatal / log.Fatalf anywhere after sentry.Init.
func fatalWithSentry(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if sentryEnabled() {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetLevel(sentry.LevelFatal)
			sentry.CaptureException(errors.New(msg))
		})
		sentry.Flush(2 * time.Second)
	}
	log.Fatal(msg)
}

func main() {
	if sentryEnabled() {
		// Initialize Sentry SDK early in application setup (production only).
		err := sentry.Init(sentry.ClientOptions{
			Dsn: "https://cf7b9fda532355b2262930ddbb4d85b6@o4510992653877248.ingest.de.sentry.io/4510992659447888",
		})
		if err != nil {
			// Sentry is not yet available, so we can only log locally.
			log.Fatalf("sentry.Init: %s", err)
		}
		// Flush buffered events before the program terminates.
		defer sentry.Flush(2 * time.Second)

		// Catch any panic in the main goroutine and report it to Sentry before
		// re-panicking so the process still exits with a non-zero status.
		defer func() {
			if r := recover(); r != nil {
				sentry.CurrentHub().Recover(r)
				sentry.Flush(2 * time.Second)
				panic(r)
			}
		}()

		log.Println("Sentry initialized successfully")
	} else {
		log.Println("Sentry disabled (debug build)")
	}

	// Create an instance of the app structure
	app := NewApp()

	// Create application menu
	appMenu := menu.NewMenu()

	// File menu
	fileMenu := appMenu.AddSubmenu("File")
	fileMenu.AddText("Settings", keys.CmdOrCtrl(","), func(_ *menu.CallbackData) {
		runtime.EventsEmit(app.ctx, "menu:settings")
	})
	fileMenu.AddSeparator()
	fileMenu.AddText("Quit", keys.CmdOrCtrl("Q"), func(_ *menu.CallbackData) {
		runtime.Quit(app.ctx)
	})

	// Choose the Wails logger based on build type.
	// In production, route Error/Fatal to Sentry; in debug, pass nil so Wails
	// uses its built-in logger and errors appear only in the terminal/DevTools.
	var wailsLogger logger.Logger
	if sentryEnabled() {
		wailsLogger = logging.NewSentryLogger()
	}

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "Noti",
		Width:  1200,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Menu:             appMenu,
		Bind: []interface{}{
			app,
		},
		Logger:   wailsLogger,
		LogLevel: logger.ERROR,
		Debug: options.Debug{
			OpenInspectorOnStartup: true,
		},
	})

	if err != nil {
		fatalWithSentry("%v", err)
	}
}
