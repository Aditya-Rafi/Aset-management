package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/auth"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/config"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/database"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/handlers"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/surel"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/token"
	"github.com/Aditya-Rafi/asset-management-system/backend/seed"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	db, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	penanda, err := token.NewPenanda(cfg.JWTSecret, cfg.AccessTokenTTL)
	if err != nil {
		log.Fatalf("token: %v", err)
	}
	authSrv := auth.NewServer(db, penanda)

	// Admin recovery: run once, e.g. `server pulihkan-admin admin` with the new
	// password on stdin, then exit without starting the HTTP server.
	if len(os.Args) >= 2 && os.Args[1] == "pulihkan-admin" {
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "pemakaian: server pulihkan-admin <nama_pengguna>  (sandi baru lewat stdin)")
			os.Exit(2)
		}
		if err := auth.PulihkanAdmin(authSrv.Store, os.Args[2], os.Stdin); err != nil {
			fmt.Fprintf(os.Stderr, "pulihkan-admin: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("sandi %s diganti dan seluruh sesinya dicabut\n", os.Args[2])
		return
	}

	if cfg.AutoSeed {
		if err := seed.Run(db); err != nil {
			log.Fatalf("seed: %v", err)
		}
	}
	// The first admin exists only when the users table is empty (never resets
	// an existing account) and there is no default password.
	created, err := auth.Bootstrap(authSrv.Store, cfg.AdminUser, cfg.AdminPassword)
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}
	if created && cfg.AdminEmail != "" {
		if err := auth.PasangEmailAdminPertama(authSrv.Store, cfg.AdminUser, cfg.AdminEmail); err != nil {
			log.Fatalf("ADMIN_EMAIL: %v", err)
		}
	}

	mandiri := auth.Mandiri{URLPublik: cfg.PublicBaseURL}
	if cfg.SMTP.Aktif() {
		mandiri.Surel = surel.NewAntrean(surel.SMTP{K: cfg.SMTP}, 64)
		log.Printf("auth: email lewat %s:%s", cfg.SMTP.Host, cfg.SMTP.Port)
	} else {
		log.Print("auth: SMTP_HOST kosong, lupa sandi lewat email dimatikan")
	}
	if cfg.GuestEnabled {
		pt, err := token.NewPenanda(cfg.JWTSecret, cfg.GuestTTL)
		if err != nil {
			log.Fatalf("token tamu: %v", err)
		}
		mandiri.PenandaTamu = pt
		log.Printf("auth: mode tamu aktif, umur sesi %s", cfg.GuestTTL)
	}
	authSrv.PasangMandiri(mandiri)

	router, err := handlers.NewRouter(cfg, db, authSrv)
	if err != nil {
		log.Fatalf("router: %v", err)
	}
	if err := os.MkdirAll(cfg.UploadDir, 0o755); err != nil {
		log.Fatalf("uploads: %v", err)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s (env=%s, db=%s)", cfg.Port, cfg.Env, cfg.DBDriver)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
