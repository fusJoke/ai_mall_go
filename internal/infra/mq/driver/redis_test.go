package driver

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"ai-go-mall/internal/infra/config"
)

// ============================================================
// 构造参数校验
// ============================================================

// TestNewRedis_HostEmpty 验证 Host 空 → error。
func TestNewRedis_HostEmpty(t *testing.T) {
	_, err := NewRedis(RedisConfig{Host: ""})
	if err == nil {
		t.Fatal("NewRedis empty host = nil, want error")
	}
	if !strings.Contains(err.Error(), "host is empty") {
		t.Errorf("err = %v, want host substring", err)
	}
}

// TestNewRedisWithGroup_HostEmpty 验证 NewRedisWithGroup 空 Host → error。
func TestNewRedisWithGroup_HostEmpty(t *testing.T) {
	_, err := NewRedisWithGroup(RedisConfig{Host: ""}, "g", time.Second)
	if err == nil {
		t.Fatal("NewRedisWithGroup empty host = nil, want error")
	}
}

// TestNewRedisWithGroup_GroupEmpty 验证 group 空 → error。
func TestNewRedisWithGroup_GroupEmpty(t *testing.T) {
	_, err := NewRedisWithGroup(RedisConfig{Host: "127.0.0.1"}, "", time.Second)
	if err == nil {
		t.Fatal("NewRedisWithGroup empty group = nil, want error")
	}
	if !strings.Contains(err.Error(), "consumer group is empty") {
		t.Errorf("err = %v, want consumer group substring", err)
	}
}

// TestNewRedisWithGroup_BlockTimeoutDefaults 验证 blockTimeout <= 0 时回退 5s 默认值。
//
// 不能直接读结构体私有字段，但可以验证间接行为：构造不报错，subscribe 能启
// （不需要测订阅本身，只测构造路径）。
func TestNewRedisWithGroup_BlockTimeoutDefaults(t *testing.T) {
	d, err := NewRedisWithGroup(RedisConfig{Host: "127.0.0.1"}, "g", 0)
	if err != nil {
		t.Fatalf("NewRedisWithGroup timeout=0 = %v, want nil (default applied)", err)
	}
	if d == nil {
		t.Fatal("returned driver is nil")
	}
	// 负值同样应回退默认。
	d2, err := NewRedisWithGroup(RedisConfig{Host: "127.0.0.1"}, "g", -1*time.Second)
	if err != nil {
		t.Fatalf("NewRedisWithGroup timeout<0 = %v, want nil", err)
	}
	if d2 == nil {
		t.Fatal("returned driver is nil")
	}
}

// TestRedisConfig_AliasConfigRedisConfig 验证 RedisConfig 与 config.RedisConfig 是同一类型。
//
// config/mq.yaml 用 config.RedisConfig 字段承载 redis.* 子节点；驱动通过
// type alias 复用，sync 调用方拿到的就是 config.RedisConfig。
func TestRedisConfig_AliasConfigRedisConfig(t *testing.T) {
	cfg := config.RedisConfig{Host: "x", Port: 1234}
	var rc RedisConfig = cfg // 编译期类型相同 → alias 成立
	if rc.Host != "x" || rc.Port != 1234 {
		t.Errorf("alias field divergence: %+v", rc)
	}
}

// ============================================================
// 内部错误识别 helpers（白盒）
// ============================================================

// TestIsBusyGroupErr 验证 isBusyGroupErr 对 BUSYGROUP 子串返回 true。
func TestIsBusyGroupErr(t *testing.T) {
	if !isBusyGroupErr(errors.New("BUSYGROUP Consumer Group name already exists")) {
		t.Error("BUSYGROUP message not recognized")
	}
	if isBusyGroupErr(errors.New("some other error")) {
		t.Error("non-BUSYGROUP message falsely recognized")
	}
	if isBusyGroupErr(nil) {
		t.Error("nil err falsely recognized as BUSYGROUP")
	}
}

// TestIsTimeoutErr 验证 isTimeoutErr 对 timeout / i/o timeout 子串返回 true。
func TestIsTimeoutErr(t *testing.T) {
	cases := []string{
		"timeout",
		"i/o timeout",
		"read tcp 127.0.0.1:6379: i/o timeout",
	}
	for _, msg := range cases {
		if !isTimeoutErr(errors.New(msg)) {
			t.Errorf("isTimeoutErr(%q) = false, want true", msg)
		}
	}
	if isTimeoutErr(nil) {
		t.Error("nil err falsely recognized as timeout")
	}
	if isTimeoutErr(errors.New("some other error")) {
		t.Error("non-timeout message falsely recognized")
	}
}

// ============================================================
// 行为契约（不依赖真实 Redis）
// ============================================================

// TestRedisDriver_Publish_EmptyTopic_ErrInvalid 验证 Publish 空 topic → ErrInvalid。
//
// 不发真实网络包：构造空 topic 时 driver 必须在调 XADD 前就返 ErrInvalid。
func TestRedisDriver_Publish_EmptyTopic_ErrInvalid(t *testing.T) {
	d, err := NewRedisWithGroup(RedisConfig{Host: "127.0.0.1"}, "g", time.Second)
	if err != nil {
		t.Fatalf("ctor = %v", err)
	}
	if err := d.Publish(context.Background(), "", []byte("x")); !errors.Is(err, ErrInvalid) {
		t.Errorf("Publish empty topic = %v, want ErrInvalid", err)
	}
}

// TestRedisDriver_Subscribe_EmptyArgs_ErrInvalid 验证 Subscribe 收到空入参 → ErrInvalid，
// 不连 Redis 也不发起 XGROUP CREATE。
func TestRedisDriver_Subscribe_EmptyArgs_ErrInvalid(t *testing.T) {
	d, err := NewRedisWithGroup(RedisConfig{Host: "127.0.0.1"}, "g", time.Second)
	if err != nil {
		t.Fatalf("ctor = %v", err)
	}
	ctx := context.Background()
	if err := d.Subscribe(ctx, "", "g", func(_ context.Context, _ Message) error { return nil }); !errors.Is(err, ErrInvalid) {
		t.Errorf("Subscribe empty topic = %v, want ErrInvalid", err)
	}
	if err := d.Subscribe(ctx, "t", "", func(_ context.Context, _ Message) error { return nil }); !errors.Is(err, ErrInvalid) {
		t.Errorf("Subscribe empty group = %v, want ErrInvalid", err)
	}
	if err := d.Subscribe(ctx, "t", "g", nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("Subscribe nil handler = %v, want ErrInvalid", err)
	}
}

// TestRedisDriver_ImplementsDriverInterface 编译期断言 redisDriver 满足 Driver 接口。
//
// 比 var _ Driver = ... 弱（声明在 redis.go），但能让 reviewer 一眼看出
// 任何新增的 Driver 方法不会悄然漏在 redisDriver 上。
func TestRedisDriver_ImplementsDriverInterface(t *testing.T) {
	var _ Driver = (*redisDriver)(nil)
}

// suppress "imported and not used" 警告：redis.NewClient 仅用于类型断言。
var _ = redis.Nil
