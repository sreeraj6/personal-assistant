// Me Tracker starts the desktop tracker or configures Google Sheets sync.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"metracker/internal/app"
	"metracker/internal/config"
	"metracker/internal/googlesync"
	"metracker/internal/platform"
)

func main() {
	setup := flag.Bool("setup-google", false, "Connect your Google Sheet using browser authorization")
	sheet := flag.String("sheet", "", "Google Sheet URL or spreadsheet ID")
	tab := flag.String("tab", "Activities", "Dedicated Google Sheet tab for activity entries")
	credentials := flag.String("credentials", "", "Downloaded Desktop app OAuth client JSON path")
	flag.Parse()
	if *setup {
		if *sheet == "" || *credentials == "" {
			log.Fatal("use --setup-google --sheet URL --credentials /path/to/client.json [--tab Activities]")
		}
		paths, err := config.UserPaths()
		if err != nil {
			log.Fatal(err)
		}
		if err := os.MkdirAll(paths.Directory, 0700); err != nil {
			log.Fatal(err)
		}
		lock, err := platform.LockInstance(filepath.Join(paths.Directory, "tracker.lock"))
		if err != nil {
			log.Fatal("stop Me Tracker before Google setup: ", err)
		}
		defer lock.Close()
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		openBrowser := func(url string) error {
			fmt.Println("Open this Google authorization link yourself to connect your account:")
			fmt.Println(url)
			return nil
		}
		fmt.Println("Google setup does not launch a browser automatically.")
		if err := googlesync.Setup(ctx, paths.Directory, *sheet, *tab, *credentials, openBrowser); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Google Sheets connected. Start Me Tracker to sync queued and new entries.")
		return
	}
	if *sheet != "" || *credentials != "" {
		log.Fatal("--sheet and --credentials require --setup-google")
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
