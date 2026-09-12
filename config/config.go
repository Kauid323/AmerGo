package config

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

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

	WebUI struct {
		Host string `yaml:"host"` // WebUI 监听地址，为空则继承 server.host
		Port int    `yaml:"port"` // WebUI 监听端口，为 0 则继承 server.port
	} `yaml:"webui"`
}

var (
	AppConfig      Config
	configMutex    sync.RWMutex
	configFilePath = "config.yaml"
	lastModTime    time.Time
	watcherOnce    sync.Once
)

// GetConfigPath returns the currently active config file path.
func GetConfigPath() string {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return configFilePath
}

// GetBlockedWords returns a thread-safe copy of all blocked words grouped by category.
func GetBlockedWords() map[string][]string {
	configMutex.RLock()
	defer configMutex.RUnlock()
	res := make(map[string][]string, len(AppConfig.BlockedWords))
	for k, v := range AppConfig.BlockedWords {
		wordsCopy := make([]string, len(v))
		copy(wordsCopy, v)
		res[k] = wordsCopy
	}
	return res
}

// LoadConfig loads configuration from path, sets up globals, and starts the file watcher.
func LoadConfig(path string) (*Config, error) {
	if path != "" {
		configMutex.Lock()
		configFilePath = path
		configMutex.Unlock()
	}

	cfg, err := ReloadConfig()
	if err != nil {
		return nil, err
	}

	// 启动后台文件监听器，自动检测 config.yaml 改动并热重载
	StartConfigWatcher()

	return cfg, nil
}

// ReloadConfig re-reads the config file and updates AppConfig in memory (Hot Reload).
func ReloadConfig() (*Config, error) {
	configMutex.Lock()
	defer configMutex.Unlock()

	path := configFilePath
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件元信息失败: %w", err)
	}

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
	if cfg.WebUI.Port == 0 {
		cfg.WebUI.Port = cfg.Server.Port
	}
	if cfg.WebUI.Host == "" {
		cfg.WebUI.Host = cfg.Server.Host
	}
	if cfg.BlockedWords == nil {
		cfg.BlockedWords = make(map[string][]string)
	}

	AppConfig = cfg
	lastModTime = fileInfo.ModTime()

	log.Printf("[Config] 成功热重载配置文件 (%s)，当前屏蔽词分类数: %d", path, len(cfg.BlockedWords))
	return &cfg, nil
}

// StartConfigWatcher starts a background ticker to watch for config file changes and hot-reload.
func StartConfigWatcher() {
	watcherOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				configMutex.RLock()
				path := configFilePath
				recordedModTime := lastModTime
				configMutex.RUnlock()

				fi, err := os.Stat(path)
				if err != nil {
					continue
				}

				if fi.ModTime().After(recordedModTime) {
					log.Printf("[Config Watcher] 检测到配置文件 %s 发生外部变动，正在热重载配置...", path)
					if _, err := ReloadConfig(); err != nil {
						log.Printf("[Config Watcher Error] 热重载配置文件失败: %v", err)
					} else {
						log.Printf("[Config Watcher] 配置文件热重载完成！最新配置已即时生效。")
					}
				}
			}
		}()
	})
}

// UpdateBlockedWords updates the blocked_words map in config.yaml preserving existing formatting/comments, and hot-reloads memory.
func UpdateBlockedWords(newBlockedWords map[string][]string) error {
	configMutex.Lock()
	defer configMutex.Unlock()

	path := configFilePath
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	var rootNode yaml.Node
	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return fmt.Errorf("解析 YAML 语法树失败: %w", err)
	}

	// 构造新的 blocked_words 节点
	var wordsNode yaml.Node
	wordsBytes, err := yaml.Marshal(newBlockedWords)
	if err != nil {
		return fmt.Errorf("序列化屏蔽词失败: %w", err)
	}
	if err := yaml.Unmarshal(wordsBytes, &wordsNode); err != nil {
		return fmt.Errorf("构建屏蔽词节点失败: %w", err)
	}

	// 在根 MappingNode 中查找 blocked_words 键
	updated := false
	if len(rootNode.Content) > 0 && rootNode.Content[0].Kind == yaml.MappingNode {
		mapping := rootNode.Content[0]
		for i := 0; i < len(mapping.Content); i += 2 {
			keyNode := mapping.Content[i]
			if keyNode.Value == "blocked_words" {
				if len(wordsNode.Content) > 0 {
					mapping.Content[i+1] = wordsNode.Content[0]
				} else {
					mapping.Content[i+1] = &yaml.Node{Kind: yaml.MappingNode}
				}
				updated = true
				break
			}
		}
		if !updated {
			// 如果不存在 blocked_words 节点，则追加
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "blocked_words"}
			valNode := &yaml.Node{Kind: yaml.MappingNode}
			if len(wordsNode.Content) > 0 {
				valNode = wordsNode.Content[0]
			}
			mapping.Content = append(mapping.Content, keyNode, valNode)
			updated = true
		}
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&rootNode); err != nil {
		return fmt.Errorf("重新编码 YAML 失败: %w", err)
	}
	_ = encoder.Close()

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	// 即时热更新内存中的 AppConfig
	AppConfig.BlockedWords = newBlockedWords
	if fi, err := os.Stat(path); err == nil {
		lastModTime = fi.ModTime()
	}

	log.Printf("[Config] 屏蔽词配置已持久化保存至 %s 并热更新生效 (分类数: %d)", path, len(newBlockedWords))
	return nil
}
