// Command devtoken 是 C 端 H5 开发联调用的**开发期**令牌签发工具。
//
// 背景（openspec change `add-uni-h5-client` 任务 4.7 / 第 13 组）：
// `POST /user/login` 强制消费型点选验证码校验，客户端无法在自动化/联调场景
// 走通真实登录拿令牌；`POST /user/login/mobile`（手机号 + 滑块）属后续 change。
// 因此在真实登录通道落地前，用本命令直接签发一枚合法令牌，解锁需要鉴权的
// 全部 C 端接口（抽卡 / 订单 / 关注）。
//
// 用法：
//
//	go run ./cmd/devtoken -username h5dev
//	go run ./cmd/devtoken -username h5dev -topup 500
//
// 行为：
//  1. 按 username 查 users（含软删过滤）；不存在则以固定开发密码创建，
//     status=1；
//  2. 可选按 -topup 给该用户充值（走 balance 定向更新，便于验证 402 余额不足）；
//  3. 用 internal/infra/token 的 Manager 签发 type=user 的真实令牌
//     （uuid v7 明文，库内只存 SHA256），TTL 默认 30 天（按"记住我"档）；
//  4. 把明文 token 打印到 stdout，可直接写入 `uni-h5/.env.local` 的
//     `VITE_DEV_TOKEN`。
//
// 安全边界：
//   - 本命令不是产品功能，**禁止**在生产环境使用；使用时请自行确认
//     `.env.yaml` 指向的是本地库。
//   - 令牌明文只打印一次，不做任何持久化。
//   - 创建用户时**不写** `register_source`：该列的写入归属
//     `POST /user/login/mobile` 的服务端实现（见 migration 000017 与
//     openspec change `add-uni-h5-client` 第 12 组），本工具不越界。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/infra/token"
	"ai-go-mall/internal/model"
)

// tokenTypeUser 与 internal/service/user 的私有常量 tokenTypeUser 取值一致。
//
// service 层未导出该常量，这里按字面量复刻；middleware.UserAuth() 校验的
// 也是同一个值（"user"），改动任一处需三处同步。
const tokenTypeUser = "user"

// devPassword 是 devtoken 自动建号时使用的固定开发密码。
//
// 仅用于本地联调，不打印、不对外；该账号不应出现在任何真实环境。
const devPassword = "Devtoken!2026"

// defaultTTL 默认令牌有效期：与用户登录"记住我"档（TokenTTLRemember）一致，
// 30 天，避免联调期间反复重新签发。
const defaultTTL = 30 * 24 * time.Hour

func main() {
	var (
		username = flag.String("username", "h5dev", "C 端用户名；不存在则自动创建")
		nickname = flag.String("nickname", "H5 联调账号", "自动创建时使用的昵称")
		ttl      = flag.Duration("ttl", defaultTTL, "令牌有效期（Go duration，如 720h）")
		topup    = flag.Float64("topup", 0, "可选：为该用户充值指定金额（元），仅 >0 时生效")
	)
	flag.Parse()

	if *username == "" {
		log.Fatalf("devtoken: -username 不能为空")
	}

	// 与 cmd/seed / cmd/es-sync 相同：初始化顺序 config → database → token
	// （token.Init 依赖 database.Get()）。
	if err := config.Init("."); err != nil {
		log.Fatalf("devtoken: init config: %v", err)
	}
	if err := database.Init(); err != nil {
		log.Fatalf("devtoken: init database: %v", err)
	}
	defer database.Reset()
	if err := token.Init(); err != nil {
		log.Fatalf("devtoken: init token: %v", err)
	}

	db := database.Get()
	ctx := context.Background()

	u, created, err := ensureUser(ctx, db, *username, *nickname)
	if err != nil {
		log.Fatalf("devtoken: ensure user: %v", err)
	}

	if *topup > 0 {
		if err := db.WithContext(ctx).
			Model(&model.User{}).
			Where("id = ?", u.ID).
			UpdateColumn("balance", gorm.Expr("balance + ?", *topup)).Error; err != nil {
			log.Fatalf("devtoken: topup: %v", err)
		}
	}

	// 签发令牌：uuid v7 明文 + 显式 TTL。库内落 SHA256，明文不落库。
	uid, err := uuid.NewV7()
	if err != nil {
		log.Fatalf("devtoken: new uuid: %v", err)
	}
	rawToken := uid.String()
	tok := &model.Token{
		Token:     rawToken,
		Type:      tokenTypeUser,
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(*ttl),
	}
	if err := token.Get().Create(ctx, tok); err != nil {
		log.Fatalf("devtoken: create token: %v", err)
	}

	// 复核金额（topup 后重新读一次，打印给联调的人看）。
	var fresh model.User
	if err := db.WithContext(ctx).First(&fresh, u.ID).Error; err != nil {
		log.Fatalf("devtoken: reload user: %v", err)
	}

	action := "复用"
	if created {
		action = "新建"
	}
	fmt.Printf("user      : %s (id=%d, %s)\n", fresh.Username, fresh.ID, action)
	fmt.Printf("balance   : %.2f\n", fresh.Balance)
	fmt.Printf("expires_at: %s\n", tok.ExpiresAt.Format(time.RFC3339))
	fmt.Printf("token     : %s\n", rawToken)
	fmt.Println()
	fmt.Println("写入 uni-h5/.env.local：")
	fmt.Printf("VITE_DEV_TOKEN=%s\n", rawToken)
}

// ensureUser 按 username 查用户；不存在则创建（固定开发密码 + status=1）。
//
// 返回 (用户, 是否新建, error)。查库带软删过滤（GORM 默认行为，DeletedAt 非空
// 的行不会被 First 命中；这正是我们要的 —— 软删账号视为不存在，重新建一个新行
// 会撞 username 唯一索引，故此处对"已软删同名账号"显式报错而不是静默建号）。
func ensureUser(ctx context.Context, db *gorm.DB, username, nickname string) (*model.User, bool, error) {
	var u model.User

	err := db.WithContext(ctx).Where("username = ?", username).First(&u).Error
	switch {
	case err == nil:
		return &u, false, nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, false, fmt.Errorf("query user %q: %w", username, err)
	}

	// 同名账号被软删：username 唯一索引仍占用，直接建号会报 1062，给出可操作提示。
	var softDeleted model.User
	if e := db.WithContext(ctx).Unscoped().
		Where("username = ? AND deleted_at IS NOT NULL", username).
		First(&softDeleted).Error; e == nil {
		return nil, false, fmt.Errorf(
			"username %q 已存在但被软删除（id=%d）：请换一个用户名，或手工恢复该行",
			username, softDeleted.ID)
	}

	hash, herr := bcrypt.GenerateFromPassword([]byte(devPassword), bcrypt.DefaultCost)
	if herr != nil {
		return nil, false, fmt.Errorf("hash dev password: %w", herr)
	}

	nu := &model.User{
		Username: username,
		Nickname: nickname,
		Password: string(hash),
		Status:   1,
		Balance:  0,
		// Mobile / Email / RegisterSource 均留空：
		// mobile 留空避免与真实手机号登录冲突；register_source 的写入
		// 归属 POST /user/login/mobile（见文件头注释）。
	}
	if cerr := db.WithContext(ctx).Create(nu).Error; cerr != nil {
		return nil, false, fmt.Errorf("create user %q: %w", username, cerr)
	}
	return nu, true, nil
}
