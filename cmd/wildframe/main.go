package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"wildframe/internal/application"
	"wildframe/internal/evidence"
	"wildframe/internal/persistence"
	webtransport "wildframe/internal/web"
)

func main() {
	if err := run(); err != nil {
		log.Printf("wildframe 退出：%v", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := parseConfig()
	if err != nil {
		return err
	}
	if cfg.selfcheck {
		defer os.RemoveAll(cfg.dataDir)
	}
	repo, err := persistence.Open(cfg.dataDir)
	if err != nil {
		return fmt.Errorf("恢复存储: %w", err)
	}
	blobs, err := persistence.NewBlobStore(cfg.dataDir)
	if err != nil {
		return fmt.Errorf("打开载荷库: %w", err)
	}
	signer, err := loadSigner(cfg.dataDir)
	if err != nil {
		return fmt.Errorf("加载签名密钥: %w", err)
	}
	service := application.NewService(repo, blobs, evidence.NewQualityEngine(), signer)
	handler := webtransport.NewServer(service).Handler()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 4 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 45 * time.Second}
	listener, err := net.Listen("tcp", cfg.address)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", cfg.address, err)
	}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	log.Printf("野镜影像发布工作台已监听 http://%s", listener.Addr())
	if cfg.selfcheck {
		ctx, cancel := context.WithTimeout(context.Background(), 14*time.Second)
		defer cancel()
		checkErr := runSelfcheck(ctx, "http://"+listener.Addr().String())
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
		serveErr := <-serveErrors
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		if checkErr != nil {
			return checkErr
		}
		log.Print("selfcheck 通过：批次已发布，Ed25519 凭据有效")
		return nil
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case signalValue := <-stop:
		log.Printf("收到 %s，开始优雅关闭", signalValue)
	case serveErr := <-serveErrors:
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func loadSigner(dataDir string) (*evidence.Signer, error) {
	path := filepath.Join(dataDir, "release-ed25519.key")
	raw, err := os.ReadFile(path)
	if err == nil {
		return evidence.NewSigner(raw)
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	signer, err := evidence.NewSigner(nil)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, signer.PrivateKey(), 0o600); err != nil {
		return nil, err
	}
	return signer, nil
}
