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

	Image struct {
		Scale     float64 `yaml:"scale"`      // 自定义缩小比例 (例如 0.5 或 50 表示缩小为原图的 50%；设为 0 则按 max_width 和 max_height 自动等比缩放)
		MaxWidth  int     `yaml:"max_width"`  // 最大显示宽度 (px)，默认 300
		MaxHeight int     `yaml:"max_height"` // 最大显示高度 (px)，默认 320
	} `yaml:"image"`
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
	if cfg.Image.MaxWidth <= 0 {
		cfg.Image.MaxWidth = 300
	}
	if cfg.Image.MaxHeight <= 0 {
		cfg.Image.MaxHeight = 320
	}

	AppConfig = cfg
	return &cfg, nil
}
