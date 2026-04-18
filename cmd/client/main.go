package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cry0404/MyWechatRss/internal/accounts"
	"github.com/cry0404/MyWechatRss/internal/articles"
	"github.com/cry0404/MyWechatRss/internal/auth"
	"github.com/cry0404/MyWechatRss/internal/config"
	"github.com/cry0404/MyWechatRss/internal/crypto"
	"github.com/cry0404/MyWechatRss/internal/httpapi"
	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/notify"
	"github.com/cry0404/MyWechatRss/internal/rss"
	"github.com/cry0404/MyWechatRss/internal/store"
	"github.com/cry0404/MyWechatRss/internal/subs"
	"github.com/cry0404/MyWechatRss/internal/upstream"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("warning: no .env file loaded: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	codec, err := crypto.New(cfg.AppSecret)
	if err != nil {
		log.Fatalf("crypto: %v", err)
	}

	st, err := store.Open(cfg.DBPath, codec)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	ctxBoot := context.Background()
	hasUser, err := st.HasAnyUser(ctxBoot)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	if !hasUser && cfg.BootstrapPassword != "" {
		hash, err := auth.HashPassword(cfg.BootstrapPassword)
		if err != nil {
			log.Fatalf("bootstrap: hash password: %v", err)
		}
		u := &model.User{
			Username:     cfg.BootstrapUsername,
			Email:        cfg.BootstrapEmail,
			PasswordHash: hash,
			IsAdmin:      true,
		}
		if err := st.CreateUser(ctxBoot, u); err != nil {
			log.Fatalf("bootstrap user: %v", err)
		}
		log.Printf("bootstrap: created first user %q — change password in Settings", cfg.BootstrapUsername)
	}

	up := upstream.New(cfg.UpstreamBaseURL, cfg.UpstreamAPIKeyID, cfg.UpstreamAPISecret)
	caller := &accounts.Caller{Store: st, Upstream: up}

	accSvc := accounts.NewService(st, up, cfg.DefaultDeviceName)
	subSvc := subs.NewService(st, caller)
	artSvc := articles.NewService(st, caller, string(cfg.ContentFetchMode))

	signer := auth.NewSigner(cfg.JWTSecret)
	feedEnc := rss.NewFeedIDEncoder(cfg.FeedIDSalt)

	notifier := notify.NewStoreNotifier(st)
	log.Printf("notifier: using dynamic store notifier (configurable via frontend)")

	rescanURL := strings.TrimRight(cfg.PublicBaseURL, "/") + "/accounts"
	st.SetDeadHook(func(ctx context.Context, userID int64, lastErr string) {
		activeN, err := st.CountActiveAccounts(ctx, userID)
		if err != nil {
			log.Printf("dead-hook: count active accounts user=%d: %v", userID, err)
			return
		}
		if activeN > 0 {
			return
		}
		user, err := st.GetUserByID(ctx, userID)
		if err != nil {
			log.Printf("dead-hook: load user %d: %v", userID, err)
			return
		}
		ev := notify.AccountsDeadEvent{
			UserID:    user.ID,
			Username:  user.Username,
			Email:     user.Email,
			LastErr:   lastErr,
			RescanURL: rescanURL,
		}
		if err := notifier.AccountsAllDead(ctx, ev); err != nil {
			log.Printf("dead-hook: notify user %d: %v", userID, err)
		}
	})

	router := httpapi.NewRouter(cfg, st, signer, accSvc, subSvc, artSvc, feedEnc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scheduler := articles.NewScheduler(st, artSvc)
	go scheduler.Run(ctx)

	alerter := articles.NewAlerter(st, notifier)
	go alerter.Run(ctx)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		log.Printf("wechatread-client listening on %s", cfg.ListenAddr)
		errc <- srv.ListenAndServe()
	}()

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errc:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	case s := <-sigc:
		log.Printf("received signal %v, shutting down...", s)
		cancel()
		sctx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer scancel()
		if err := srv.Shutdown(sctx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}
}
