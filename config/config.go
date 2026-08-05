package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	AdminUserID string `yaml:"admin_user_id"`
	TempFolder  string `yaml:"temp_folder"`

	Server struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"server"`

	QQ struct {
		AdminQQ string `yaml:"admin_qq"`
		BotQQ   string `yaml:"bot_qq"`
		BotName string `yaml:"bot_name"`
	} `yaml:"qq"`

	YH struct {
		AdminID string `yaml:"admin_id"`
		Token   string `yaml:"token"`
		Mode    string `yaml:"mode"` // "webhook" or "websocket"
		Webhook struct {
			Path string `yaml:"path"`
		} `yaml:"webhook"`
		WebSocket struct {
			URL string `yaml:"url"`
		} `yaml:"websocket"`
	} `yaml:"yh"`

	BlockedWords map[string][]string `yaml:"blocked_words"`

	Redis struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		DB       int    `yaml:"db"`
		Password string `yaml:"password"`
	} `yaml:"redis"`

	SQLite struct {
		DBPath string `yaml:"db_path"`
	} `yaml:"sqlite"`

	Messages struct {
		MessageYH         string `yaml:"message_yh"`
		MessageYHFollowed string `yaml:"message_yh_followed"`
	} `yaml:"messages"`
}

var AppConfig Config

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	if cfg.YH.Mode == "" {
		cfg.YH.Mode = "websocket"
	}
	if cfg.YH.WebSocket.URL == "" {
		cfg.YH.WebSocket.URL = "wss://ws.jwzhd.com/subscribe?token="
	}
	if cfg.YH.Webhook.Path == "" {
		cfg.YH.Webhook.Path = "/yh/webhook"
	}

	AppConfig = cfg
	return &cfg, nil
}
