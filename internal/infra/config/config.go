// Package config 基于 viper 提供配置加载能力。
//
// Init 接收项目根目录，扫描其中的 config/ 子目录下全部 .yaml 文件并合并到 viper，
// 随后再合并 rootDir/.env.yaml（本地覆盖，优先级最高）。缺失则跳过。
//
// 合并规则：
//   - config/*.yaml 按文件名升序合并（字母序在前者先生效，作为基础配置）
//   - .env.yaml 在所有 config/*.yaml 之后合并，覆盖同名键
//   - 后缀为 ".example" 的文件（如 .env.yaml.example）视为模板，跳过不加载
//
// 新增 / 拆分配置文件只需放入 config/ 目录，无需修改 loader 调用方代码。
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// EnvFile 是位于 rootDir 下的本地覆盖配置文件名（gitignored），加载优先级最高。
const EnvFile = ".env.yaml"

// configSubdir 是 Init 在 rootDir 内扫描配置文件的子目录。
const configSubdir = "config"

var cfg *Config

// Init 扫描 rootDir 下 config 目录的全部 .yaml 文件并合并到 viper，
// 然后再合并 rootDir/.env.yaml（若存在）作为本地覆盖。
//
// 同一进程重复调用是 no-op；如需强制重载，调用 Reset 后再调用 Init。
func Init(rootDir string) error {
	if cfg != nil {
		return nil
	}

	configDir := filepath.Join(rootDir, configSubdir)
	entries, err := os.ReadDir(configDir)
	if err != nil {
		return fmt.Errorf("read config dir %q: %w", configDir, err)
	}

	names := collectConfigFiles(entries)
	if len(names) == 0 {
		return fmt.Errorf("no config file (*.yaml / *.yml) found in %q", configDir)
	}

	sort.Strings(names) // 字母序在前者先生效

	viper.Reset()
	viper.SetConfigType("yaml")

	for i, name := range names {
		path := filepath.Join(configDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read config file %q: %w", path, err)
		}

		if i == 0 {
			if err := viper.ReadConfig(bytes.NewReader(data)); err != nil {
				return fmt.Errorf("parse config file %q: %w", path, err)
			}
			continue
		}
		if err := viper.MergeConfig(bytes.NewReader(data)); err != nil {
			return fmt.Errorf("merge config file %q: %w", path, err)
		}
	}

	// .env.yaml 在 config/*.yaml 之后合并，缺失则跳过。
	envPath := filepath.Join(rootDir, EnvFile)
	if data, err := os.ReadFile(envPath); err == nil {
		if err := viper.MergeConfig(bytes.NewReader(data)); err != nil {
			return fmt.Errorf("merge env file %q: %w", envPath, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read env file %q: %w", envPath, err)
	}

	loaded := &Config{}
	if err := viper.Unmarshal(loaded); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}
	loaded.applyDefaults()

	cfg = loaded
	return nil
}

// Get 返回已加载的配置。Init 未调用过或失败时返回 nil。
func Get() *Config {
	return cfg
}

// Reset 清空已缓存的配置与 viper 状态。
// 主要用于测试；也可在需要运行时热重载时使用。
func Reset() {
	cfg = nil
	viper.Reset()
}

// collectConfigFiles 从目录条目中筛选可加载的 yaml 配置名（保留扩展名）。
// 后缀为 ".example" 的文件视为模板，直接跳过。
//
// 同 base 名（如 a.yaml + a.yml）视为同一份配置，仅保留先扫描到的那份，
// 避免同名配置被合并两次。
func collectConfigFiles(entries []os.DirEntry) []string {
	allowedExt := map[string]struct{}{".yaml": {}, ".yml": {}}
	var names []string
	seen := make(map[string]struct{})
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := filepath.Ext(name)
		if _, ok := allowedExt[ext]; !ok {
			continue
		}
		base := strings.TrimSuffix(name, ext)
		if filepath.Ext(base) == ".example" {
			continue
		}
		if _, dup := seen[base]; dup {
			continue
		}
		seen[base] = struct{}{}
		names = append(names, name)
	}
	return names
}

// Config 是配置文件的强类型映射。
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
}

// ServerConfig HTTP 服务配置。
type ServerConfig struct {
	Name string `mapstructure:"name"`
	Port int    `mapstructure:"port"`
}

// DatabaseConfig 数据库配置。Type 与 Prefix 对写库与读库共享。
//
// 读副本通过 Read.Enabled 开关：false 时所有读写都走 Write。
type DatabaseConfig struct {
	Type   string           `mapstructure:"type"`
	Prefix string           `mapstructure:"prefix"`
	Write  DBInstanceConfig `mapstructure:"write"`
	Read   DBReadConfig     `mapstructure:"read"`
}

// DBInstanceConfig 单实例连接配置，写库与读库共用字段。
//
// 连接超时字段使用 Go 标准 duration 字符串（"1h"、"10m"、"30s"）。
type DBInstanceConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	Username        string        `mapstructure:"username"`
	Password        string        `mapstructure:"password"`
	DBName          string        `mapstructure:"dbname"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
}

// DBReadConfig 读副本配置。Enabled=false 时所有读请求走写库。
type DBReadConfig struct {
	Enabled          bool `mapstructure:"enabled"`
	DBInstanceConfig `mapstructure:",squash"`
}

// applyDefaults 填充关键字段的默认值。
func (c *Config) applyDefaults() {
	if c.Database.Type == "" {
		c.Database.Type = "mysql"
	}
}
