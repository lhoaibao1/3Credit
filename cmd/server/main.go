// Command server runs the 3Credit platform.
//
//	ADDR             listen address (default :8080)
//	BASE_URL         public base URL, used for cookies and postback docs
//	DATA_DIR         directory for the SQLite database and uploads (default ./data)
//	SESSION_SECRET   secret for CSRF/flash signing (required in production)
//	DATA_KEY         secret used to encrypt lead payloads (required in production)
//	ADMIN_USER / ADMIN_PASSWORD   admin login (admin disabled when empty)
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/threecredit/platform/internal/db"
	"github.com/threecredit/platform/internal/leads"
	"github.com/threecredit/platform/internal/server"
	"github.com/threecredit/platform/internal/store"
	"github.com/threecredit/platform/internal/tracking"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	dataDir := env("DATA_DIR", "./data")
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		log.Fatal(err)
	}
	sess, key := os.Getenv("SESSION_SECRET"), os.Getenv("DATA_KEY")
	if sess == "" || key == "" {
		if os.Getenv("APP_ENV") == "production" {
			log.Fatal("SESSION_SECRET và DATA_KEY là bắt buộc khi APP_ENV=production")
		}
		log.Print("CẢNH BÁO: dùng secret mặc định cho môi trường dev")
		if sess == "" {
			sess = "dev-session-secret"
		}
		if key == "" {
			key = "dev-data-key"
		}
	}
	d, err := db.Open(filepath.Join(dataDir, "3credit.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer d.Close()
	st := store.New(d)
	if err := st.Seed(); err != nil {
		log.Fatal(err)
	}
	tr := tracking.New(d, sess)
	lr := leads.New(d, key)
	srv, err := server.New(server.Config{
		BaseURL: env("BASE_URL", "http://localhost:8080"), DataDir: dataDir, SessionSecret: sess,
		AdminUser: os.Getenv("ADMIN_USER"), AdminPassword: os.Getenv("ADMIN_PASSWORD"),
	}, st, tr, lr)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go lr.Run(ctx, 30*time.Second)

	hs := &http.Server{Addr: env("ADDR", ":8080"), Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second}
	go func() {
		log.Printf("3Credit đang chạy tại %s", hs.Addr)
		if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	<-ctx.Done()
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hs.Shutdown(sctx)
}
