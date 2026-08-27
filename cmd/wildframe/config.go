package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type config struct {
	address   string
	dataDir   string
	selfcheck bool
}

func parseConfig() (config, error) {
	var cfg config
	flag.StringVar(&cfg.address, "addr", "", "HTTP 回环监听地址")
	flag.StringVar(&cfg.dataDir, "data", "./wildframe-data", "本地事件与影像数据目录")
	flag.BoolVar(&cfg.selfcheck, "selfcheck", false, "在真实监听上执行完整流程后退出")
	flag.Parse()
	if cfg.address == "" {
		if portText := strings.TrimSpace(os.Getenv("PORT")); portText != "" {
			port, err := strconv.Atoi(portText)
			if err != nil || port < 1 || port > 65535 {
				return config{}, fmt.Errorf("PORT 必须是 1 到 65535 的端口号")
			}
			cfg.address = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		} else {
			cfg.address = "127.0.0.1:19081"
		}
	}
	if err := validateAddress(cfg.address); err != nil {
		return config{}, err
	}
	if cfg.selfcheck {
		temporary, err := os.MkdirTemp("", "wildframe-selfcheck-*")
		if err != nil {
			return config{}, err
		}
		cfg.dataDir = temporary
	} else {
		absolute, err := filepath.Abs(cfg.dataDir)
		if err != nil {
			return config{}, err
		}
		cfg.dataDir = absolute
	}
	return cfg, nil
}

func validateAddress(address string) error {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("-addr 必须为 host:port：%w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("监听端口无效")
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("为保护本地影像，监听地址必须是回环地址")
	}
	return nil
}
