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

// SetForTest 注入测试用配置。仅供测试代码使用，生产代码不应调用。
func SetForTest(c *Config) {
	cfg = c
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
	Token    TokenConfig    `mapstructure:"token"`
	CORS     CORSConfig     `mapstructure:"cors"`
	Captcha  CaptchaConfig  `mapstructure:"captcha"`
	Upload   UploadConfig   `mapstructure:"upload"`
	Cache    CacheConfig    `mapstructure:"cache"`
	Search   SearchConfig   `mapstructure:"search"`
	Mall     MallConfig     `mapstructure:"mall"`
	MQ       MQConfig       `mapstructure:"mq"`
	Channel  ChannelConfig  `mapstructure:"channel"`
}

// TokenConfig token 存储配置。
//
// 当前只支持 driver 切换；后续若加 TTL / 刷新策略等，再追加字段。
type TokenConfig struct {
	Driver string `mapstructure:"driver"`
}

// CaptchaConfig 点选验证码配置。
//
// 所有字段均必须由 captcha.yaml 提供；缺失即视为配置错误，由调用方报错。
// 不在 applyDefaults 里硬编码兜底，避免"代码默认值"和"yaml 配置"两处真相。
type CaptchaConfig struct {
	Elements    []string `mapstructure:"元素"`
	Length      int      `mapstructure:"长度"`
	NoiseLength int      `mapstructure:"混淆点长度"`
	TTLSeconds  int      `mapstructure:"过期时间"`

	// ChineseChars 启用「中文文字」元素时的可选字符集；
	// 若 Elements 含 ElementChinese 则此项必填（不能为空）。
	ChineseChars []string `mapstructure:"中文字符集"`

	// BackgroundDir 背景图目录（含 1.png / 2.png 等）。
	BackgroundDir string `mapstructure:"背景图目录"`

	// IconDir ICON 图目录（含 *.png，文件名即元素名）。
	IconDir string `mapstructure:"ICON目录"`

	// FontPath 字体文件绝对或相对路径（项目根目录的相对路径）。
	FontPath string `mapstructure:"字体路径"`
}

// UploadConfig 文件上传配置。
//
// 通过 driver 名字切换不同存储后端实现；当前仅实现 local 磁盘驱动。
// 详见 internal/infra/upload。
type UploadConfig struct {
	Driver      string            `mapstructure:"driver"`
	MaxSize     int64             `mapstructure:"max_size"`
	MaxSizeUnit string            `mapstructure:"max_size_unit"`
	Suffixes    []string          `mapstructure:"suffixes"`
	Format      string            `mapstructure:"format"`
	Local       UploadLocalConfig `mapstructure:"local"`
}

// UploadLocalConfig 本地磁盘驱动专属配置。
//
// 仅当 UploadConfig.Driver == "local" 时生效。
type UploadLocalConfig struct {
	BaseDir   string `mapstructure:"base_dir"`
	URLPrefix string `mapstructure:"url_prefix"`
}

// CacheConfig 缓存配置（mall MVP 设计 D5 / D16）。
//
// Driver 决定 Init 装配哪种 Cache 实现：
//   - "redis"（默认）：仅 Redis（单层，兼容旧部署）
//   - "multi"：L1 内存 + L2 Redis 双层（Phase 10 落地后推荐）
//
// Cache 接口保持稳定：业务代码不感知层级，仅 Init 根据 driver 选实现。
type CacheConfig struct {
	Driver string            `mapstructure:"driver"`
	Redis  RedisConfig       `mapstructure:"redis"`
	Memory MemoryCacheConfig `mapstructure:"memory"`
	TTL    CacheTTLConfig    `mapstructure:"ttl"`
}

// MemoryCacheConfig L1 进程内内存缓存配置（D16）。
//
// 选型 github.com/patrickmn/go-cache，详见 internal/infra/cache/driver/memory.go。
type MemoryCacheConfig struct {
	DefaultTTL      time.Duration `mapstructure:"default_ttl"`
	CleanupInterval time.Duration `mapstructure:"cleanup_interval"`
	// MaxSize 容量上限（条目数）。当前 go-cache v2.1.0 未启用，
	// 保留字段供后续版本或自定义实现接入。
	MaxSize int `mapstructure:"max_size"`
}

// CacheTTLConfig 各业务场景的 L1 / L2 TTL（D5 场景表）。
//
// 业务代码读取此配置决定每次 Set 传什么 TTL：
//   - BlindBoxDetail.L1 / BlindBoxDetail.L2 分别传给 L1 和 L2（MultiLevelCache
//     不感知本配置，业务侧自行处理两层差异）。
//   - DrawRate.L2 是「限流场景」专属：限流永远走 L2 Redis（不走 L1），故仅 L2 字段。
//
// 为什么放在配置而非业务常量：TTL 是「部署可调参数」，不同环境（开发 / 灰度 / 生产）
// 可以走不同值。生产可调短（控内存），开发可调长（方便调试）。
type CacheTTLConfig struct {
	BlindBoxDetail BlindBoxDetailTTL `mapstructure:"blindbox_detail"`
	HomeFeed       HomeFeedTTL       `mapstructure:"home_feed"`
	DrawRate       DrawRateTTL       `mapstructure:"draw_rate"`
}

// BlindBoxDetailTTL 盲盒详情缓存 TTL。
type BlindBoxDetailTTL struct {
	L1 time.Duration `mapstructure:"l1"`
	L2 time.Duration `mapstructure:"l2"`
}

// HomeFeedTTL 首页 feed 缓存 TTL。
type HomeFeedTTL struct {
	L1 time.Duration `mapstructure:"l1"`
	L2 time.Duration `mapstructure:"l2"`
}

// DrawRateTTL 抽卡限流 TTL（仅 L2）。
type DrawRateTTL struct {
	L2 time.Duration `mapstructure:"l2"`
}

// RedisConfig 单实例 Redis 连接配置。
type RedisConfig struct {
	Driver   string `mapstructure:"driver"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

// SearchConfig Elasticsearch 搜索配置（mall MVP 设计 D6）。
type SearchConfig struct {
	Elasticsearch ElasticsearchConfig `mapstructure:"elasticsearch"`
}

// ElasticsearchConfig ES 客户端连接配置。
type ElasticsearchConfig struct {
	Addresses   []string `mapstructure:"addresses"`
	Username    string   `mapstructure:"username"`
	Password    string   `mapstructure:"password"`
	IndexPrefix string   `mapstructure:"index_prefix"`
}

// MallConfig mall 业务配置。
//
// 当前仅暴露 default_commission_rate；其他 mall 业务配置
// （如抽卡限流 N、活动价策略等）按需扩展。
type MallConfig struct {
	DefaultCommissionRate float64 `mapstructure:"default_commission_rate"`
}

// MQConfig 消息队列配置（mall MVP 设计 D17）。
//
// Driver 决定 Init 装配哪种 MQ 实现：
//   - "redis"（默认）：Redis Stream（生产推荐）
//   - "mock"：进程内 mock（仅单测用，禁止生产）
//
// 后续扩展：kafka / rabbitmq（同 cache 模式，仅新增 driver + 切换 driver 字段）。
type MQConfig struct {
	Driver        string        `mapstructure:"driver"`
	Redis         RedisConfig   `mapstructure:"redis"`
	ConsumerGroup string        `mapstructure:"consumer_group"`
	BlockTimeout  time.Duration `mapstructure:"block_timeout"`
}

// ChannelConfig 第三方渠道配置（mall MVP 设计 D18）。
//
// 三个字段是各渠道的 driver 名，取值域 = internal/infra/channel/factory.go
// 的 drivers 注册表（当前 mock / bank）；未知名由 channel.Init() fail fast。
type ChannelConfig struct {
	Payment string `mapstructure:"payment"`
	Bank    string `mapstructure:"bank"`
	SMS     string `mapstructure:"sms"`
}

// CORSConfig 跨域配置。
//
// 当前仅暴露 allow_origins；方法 / 请求头 / 预检缓存时间 / Credentials
// 等策略固定在 internal/middleware/cors.go，未来若需可配置化再扩字段。
type CORSConfig struct {
	AllowOrigins []string `mapstructure:"allow_origins"`
}

// ServerConfig HTTP 服务配置。
//
// Mode 决定进程运行形态："dev" 时挂载 /swagger/*any 与更详细的运行日志，
// "release"（默认）则只暴露业务路由。早期版本曾借 gin.Mode()（GIN_MODE
// 环境变量）判定，但 gin 的默认 mode 即 debug——任何未显式设置 GIN_MODE=release
// 的部署都会把 swagger UI 暴露出去，不符合"仅 dev 暴露"的承诺。改为显式应用层
// 配置后，默认 release 必须由部署侧显式开启 dev 才会暴露 API 文档。
type ServerConfig struct {
	Name string `mapstructure:"name"`
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`

	// CDNURL 是资源 CDN 域名（末尾不带 `/`）。
	// 缺省 / 空串 → 视为关闭 CDN，FullURL / fullURL 走当前请求域名。
	// 详见 internal/kit/urlx.FullURL。
	CDNURL string `mapstructure:"cdn_url"`

	// CDNURLParams 是拼在 CDN 域名后的固定子路径（如 `format/heif`）。
	// 缺省 / 空串 → 仅拼 CDN 域名。
	CDNURLParams string `mapstructure:"cdn_url_params"`
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
	if c.Token.Driver == "" {
		c.Token.Driver = "database"
	}
	// Server.Mode 默认 release：dev-only 能力（如 swagger UI）必须由部署侧
	// 显式设 server.mode: dev 才会暴露，避免任何"忘设环境变量"导致意外开放。
	if c.Server.Mode == "" {
		c.Server.Mode = "release"
	}
	// CaptchaConfig 不做兜底：所有字段必须由 captcha.yaml 显式提供。
	// 缺失字段由使用方（infra/captcha）做语义级校验并返回 ErrInvalidInput。

	// Upload 默认值：所有 upload 字段均允许缺省，缺省走代码内置的合理默认值
	// （10MB / 6 个常见后缀 / 默认 format 模板 / local storage/uploads 目录）。
	// 这样 config/upload.yaml 即使缺失文件也能启动（spec ADDED Requirement 1）。
	if c.Upload.Driver == "" {
		c.Upload.Driver = "local"
	}
	if c.Upload.MaxSize == 0 {
		c.Upload.MaxSize = 10
	}
	if c.Upload.MaxSizeUnit == "" {
		c.Upload.MaxSizeUnit = "MB"
	}
	if len(c.Upload.Suffixes) == 0 {
		c.Upload.Suffixes = []string{"jpg", "jpeg", "png", "gif", "webp", "pdf"}
	}
	if c.Upload.Format == "" {
		c.Upload.Format = "/{topic}/{year}{mon}{day}/{fileName}{fileSha1}{.suffix}"
	}
	if c.Upload.Local.BaseDir == "" {
		c.Upload.Local.BaseDir = "storage/uploads"
	}
	if c.Upload.Local.URLPrefix == "" {
		c.Upload.Local.URLPrefix = "/uploads"
	}

	// Cache 默认值：driver 默认 "redis"（向后兼容旧部署）；
	// memory 用 D16 推荐值（default_ttl=10s / cleanup=60s）。
	if c.Cache.Driver == "" {
		c.Cache.Driver = "redis"
	}
	if c.Cache.Memory.DefaultTTL == 0 {
		c.Cache.Memory.DefaultTTL = 10 * time.Second
	}
	if c.Cache.Memory.CleanupInterval == 0 {
		c.Cache.Memory.CleanupInterval = 60 * time.Second
	}
	// 各场景 TTL 缺省：L1 短 / L2 长（D5 表推荐值）。
	if c.Cache.TTL.BlindBoxDetail.L1 == 0 {
		c.Cache.TTL.BlindBoxDetail.L1 = 10 * time.Second
	}
	if c.Cache.TTL.BlindBoxDetail.L2 == 0 {
		c.Cache.TTL.BlindBoxDetail.L2 = 10 * time.Minute
	}
	if c.Cache.TTL.HomeFeed.L1 == 0 {
		c.Cache.TTL.HomeFeed.L1 = 5 * time.Second
	}
	if c.Cache.TTL.HomeFeed.L2 == 0 {
		c.Cache.TTL.HomeFeed.L2 = 5 * time.Minute
	}
	if c.Cache.TTL.DrawRate.L2 == 0 {
		c.Cache.TTL.DrawRate.L2 = 60 * time.Second
	}

	// MQ 默认值：driver 默认 redis；consumer_group 默认 mall-stock-sync；block_timeout 5s。
	if c.MQ.Driver == "" {
		c.MQ.Driver = "redis"
	}
	if c.MQ.ConsumerGroup == "" {
		c.MQ.ConsumerGroup = "mall-stock-sync"
	}
	if c.MQ.BlockTimeout == 0 {
		c.MQ.BlockTimeout = 5 * time.Second
	}

	// Channel 默认值：三渠道全 mock（MVP 模拟支付 / 手动 mark_paid，D18）；
	// config/channel.yaml 缺失时也能启动。
	if c.Channel.Payment == "" {
		c.Channel.Payment = "mock"
	}
	if c.Channel.Bank == "" {
		c.Channel.Bank = "mock"
	}
	if c.Channel.SMS == "" {
		c.Channel.SMS = "mock"
	}
}
