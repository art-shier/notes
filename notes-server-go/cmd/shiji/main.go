package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"shiji/internal/api"
	"shiji/internal/backup"
	"shiji/internal/config"
	"shiji/internal/store"
	"syscall"
	"time"
)

func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(args []string) error {
	command := "serve"
	if len(args) > 0 {
		command = args[0]
		args = args[1:]
	}
	s, e := config.Load()
	if e != nil {
		return e
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	email := fs.String("email", "", "invited email")
	admin := fs.String("admin-email", "", "administrator email")
	output := fs.String("output", "", "new backup directory")
	source := fs.String("backup", "", "backup directory")
	stopped := fs.Bool("app-stopped", false, "all application instances are stopped")
	if e = fs.Parse(args); e != nil {
		return e
	}
	switch command {
	case "backup-create":
		if *output == "" {
			return fmt.Errorf("--output required")
		}
		v, e := backup.Create(s, *output, *stopped)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(v)
	case "backup-verify":
		if *source == "" {
			return fmt.Errorf("--backup required")
		}
		v, e := backup.Verify(*source)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(v)
	case "backup-restore":
		if *source == "" {
			return fmt.Errorf("--backup required")
		}
		v, e := backup.Restore(s, *source, *stopped)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(v)
	case "health":
		client := http.Client{Timeout: 5 * time.Second}
		res, e := client.Get("http://127.0.0.1:8000/api/v1/health/ready")
		if e != nil {
			return fmt.Errorf("service unavailable")
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return fmt.Errorf("service not ready")
		}
		return nil
	case "serve", "migrate", "bootstrap", "invite", "database-check", "bootstrap-status":
	default:
		return fmt.Errorf("unknown command %q", command)
	}
	db, e := store.Open(s.DatabaseURL)
	if e != nil {
		return e
	}
	defer store.Close(db)
	if command == "database-check" {
		return deploymentCheck(db)
	}
	if command == "migrate" {
		return store.Migrate(db)
	}
	if e = store.Check(db); e != nil {
		return e
	}
	if command == "bootstrap-status" {
		state, err := bootstrapState(db)
		if err != nil {
			return err
		}
		fmt.Println(state)
		return nil
	}
	if command == "bootstrap" || command == "invite" {
		if *email == "" {
			return fmt.Errorf("--email required")
		}
		raw, e := api.IssueInvitation(db, *email, *admin, command == "bootstrap")
		if e != nil {
			return e
		}
		query := url.Values{"invite": {raw}, "email": {*email}}
		fmt.Println(s.Origin + "/?" + query.Encode())
		return nil
	}
	if e = os.MkdirAll(s.AttachmentsDir, 0700); e != nil {
		return e
	}
	if e = os.MkdirAll(s.ExportsDir, 0700); e != nil {
		return e
	}
	app := api.New(db, s)
	server := &http.Server{Addr: s.Listen, Handler: app.Router(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if e = app.CleanupExports(); e != nil {
		return e
	}
	go app.RunExportCleanup(ctx)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	fmt.Println("拾记 Go API listening on " + s.Listen)
	e = server.ListenAndServe()
	if e == http.ErrServerClosed {
		return nil
	}
	return e
}
