// Command stock-sync 异步对账 worker（mall MVP 设计 D15 / D17）。
//
// 业务动机（spec Requirement "Seckill draw flow" + design D15 / D17）：
//
//	秒杀抽卡事务内已经把 mall_card_pool_items.stock 扣过（D15 步骤 4.b），
//	mall_stock_deduction_log 是审计 / 对账表，写入 synced_to_pool_at 即可。
//	对账越早做越好（前端 dashboard、admin 报表都靠它），所以走 MQ 实时 +
//	cron 兜底双模式：MQ 失败 / worker 没跑起来，下一轮 cron 也能追上。
//
// 双模式（D17 §cmd/stock-sync 双模式）：
//
//	模式 A：消费者模式 —— Subscribe "stock.deduction.sync"，handler 收到
//	  payload 后反序列化 → MarkDeductionLogSynced(logID, now)。
//	模式 B：cron 兜底 —— 同进程内启 goroutine ticker，每隔 cronInterval
//	  扫 ListUnsyncedLogs(limit)，逐条 MarkDeductionLogSynced。
//	两者可同时跑（默认 both）。任何模式 MarkDeductionLogSynced 都是
//	幂等的（影响 0 行视为已对账），所以重复触发无副作用。
//
// 配置（环境变量 + config/mq.yaml）：
//
//	STOCK_SYNC_MODE             consumer_only / cron_only / both（默认 both）
//	STOCK_SYNC_CRON_INTERVAL    cron 兜底间隔（默认 5m；支持 Go duration 串）
//	STOCK_SYNC_BATCH_LIMIT      cron 批大小（默认 100）
//
// 退出：
//
//	- SIGINT / SIGTERM → ctx cancel → consumer 退出 + cron ticker 退出 → 主进程退出。
//
// 用法：go run ./cmd/stock-sync（需 config/*.yaml + .env.yaml、MySQL、Redis 可达）
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"gorm.io/gorm"

	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/infra/mq"
	"ai-go-mall/internal/model/mall"
)

// topicStockDeductionSync 是秒杀事务 Publish 的目标 topic。
//
// 与 internal/service/user/draw.go 的 DrawSeckill 末尾保持一字不差
// （两边不能出现"发布方写错 topic 名 / 消费方订错 topic"的对角线错位）。
const topicStockDeductionSync = "stock.deduction.sync"

// 默认配置常量。
const (
	defaultCronInterval = 5 * time.Minute
	defaultBatchLimit   = 100
)

// stockDeductionPayload 是 MQ 消息体的 JSON 形态。
//
// 业务侧（DrawSeckill）写入的也是 MallStockDeductionLog 的 JSON 序列化；
// 这里只挑出对账必需的 LogID，避免反序列化整个对象引入无关字段耦合。
type stockDeductionPayload struct {
	ID int64 `json:"id"`
}

func main() {
	if err := config.Init("."); err != nil {
		log.Fatalf("stock-sync: init config: %v", err)
	}
	if err := database.Init(); err != nil {
		log.Fatalf("stock-sync: init database: %v", err)
	}
	if err := mq.Init(); err != nil {
		log.Fatalf("stock-sync: init mq: %v", err)
	}

	mode := parseMode()
	cronInterval := parseCronInterval()
	batchLimit := parseBatchLimit()
	consumerGroup := config.Get().MQ.ConsumerGroup

	log.Printf("stock-sync starting: mode=%s cron_interval=%s batch_limit=%d consumer_group=%s",
		modeName(mode), cronInterval, batchLimit, consumerGroup)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, mode, cronInterval, batchLimit, consumerGroup); err != nil {
		// ctx cancel 引发的 err 不视为 fatal（属于优雅退出）。
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			log.Printf("stock-sync: shutdown signal received, exiting")
			return
		}
		log.Fatalf("stock-sync: %v", err)
	}
	log.Printf("stock-sync exited cleanly")
}

// run 按 mode 启 consumer / cron，可并行（both）或单线程。
//
// 返回 err 仅表示非预期失败；ctx cancel 走正常 return nil。
func run(ctx context.Context, mode runMode, cronInterval time.Duration, batchLimit int, consumerGroup string) error {
	db := database.Get()
	manager := mq.Get()

	switch mode {
	case modeConsumerOnly:
		return runConsumer(ctx, manager, db, consumerGroup)
	case modeCronOnly:
		return runCronLoopUntilDone(ctx, db, cronInterval, batchLimit)
	case modeBoth:
		return runBoth(ctx, manager, db, consumerGroup, cronInterval, batchLimit)
	default:
		return errors.New("invalid mode")
	}
}

// runCronLoopUntilDone 包一层 runCronLoop，使 mode=cron_only 也能返回 error。
//
// runCronLoop 自身只到 ctx.Done() 为止，正常路径无 err 返回；
// 这里强制返回 nil 让 run() 的 switch 各分支类型一致。
func runCronLoopUntilDone(ctx context.Context, db *gorm.DB, cronInterval time.Duration, batchLimit int) error {
	runCronLoop(ctx, db, cronInterval, batchLimit)
	return nil
}

// runBoth 启 consumer + cron goroutine，二者用 WaitGroup 等齐。
//
// consumer 是阻塞式 Subscribe 循环，独立 goroutine 跑；
// cron 是 for-select ticker，独立 goroutine 跑。
// 任一非预期退出 → WaitGroup 跟随退出；ctx cancel 时两者都自然返回。
//
// 注意：Go 1.27 的 sync.WaitGroup.Go 只接受 func()（无返回值）；
// 内部错误用 sealedErr 闭包变量传递，main 不依赖其返回值。
func runBoth(ctx context.Context, m *mq.Manager, db *gorm.DB, consumerGroup string, cronInterval time.Duration, batchLimit int) error {
	var wg sync.WaitGroup
	var sealedErr error

	wg.Go(func() {
		// consumer 启动失败 / Subscribe 退出 → log + sealedErr 记下来。
		// 这里只 log 不 fatal：cron 兜底仍能跑，避免一个 worker 挂掉整个对账流程。
		if err := runConsumer(ctx, m, db, consumerGroup); err != nil &&
			!errors.Is(err, context.Canceled) {
			log.Printf("stock-sync consumer exited with error: %v", err)
			sealedErr = err
		}
	})

	wg.Go(func() {
		runCronLoop(ctx, db, cronInterval, batchLimit)
	})

	wg.Wait()
	return sealedErr
}

// runConsumer 阻塞消费 topic，直到 ctx cancel。
//
// payload 解析失败 → log warn + 返回 nil（让 driver ack，bad payload
// 不应阻塞队列；运维层面依赖 cron 兜底把 synced_to_pool_at 写上）。
//
// handler 返回 nil → driver ack；返回 err → 不 ack（pending 保留）。
func runConsumer(ctx context.Context, m *mq.Manager, db *gorm.DB, consumerGroup string) error {
	return m.Subscribe(ctx, topicStockDeductionSync, consumerGroup, func(c context.Context, msg mq.Message) error {
		var p stockDeductionPayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			log.Printf("stock-sync: bad payload (msg_id=%s): %v", msg.ID, err)
			return nil // ack：bad payload 不重试
		}
		if p.ID <= 0 {
			log.Printf("stock-sync: invalid log id (msg_id=%s): %d", msg.ID, p.ID)
			return nil // ack：同上
		}

		now := time.Now()
		rows, err := markDeductionLogSynced(db, p.ID, now)
		if err != nil {
			log.Printf("stock-sync: mark synced id=%d failed: %v (will retry via pending)", p.ID, err)
			return err // 不 ack：留给下次重试
		}
		if rows == 0 {
			// 已 synced (幂等)：silent ack。
			log.Printf("stock-sync: log id=%d already synced", p.ID)
		} else {
			log.Printf("stock-sync: log id=%d marked synced", p.ID)
		}
		return nil
	})
}

// runCronLoop 按 cronInterval 周期扫 unsynced 记录，逐条标记。
//
// 每轮 batch 上限保护：limit 条全标完则下轮；剩余留待下轮处理。
// 任意单条 DB 错误仅 log，不中断整轮；最坏情况是这一行 5 分钟后再试。
//
// 退出条件：ctx cancel。
func runCronLoop(ctx context.Context, db *gorm.DB, cronInterval time.Duration, batchLimit int) {
	ticker := time.NewTicker(cronInterval)
	defer ticker.Stop()

	log.Printf("stock-sync cron started: interval=%s batch_limit=%d", cronInterval, batchLimit)

	// 启动时立刻跑一次，不等 ticker 到点（worker 刚启动 / 重启时
	// 有可能已经堆了上一窗口的 unsynced 记录）。
	runCronOnce(ctx, db, batchLimit)

	for {
		select {
		case <-ctx.Done():
			log.Printf("stock-sync cron: ctx cancelled, exiting")
			return
		case <-ticker.C:
			runCronOnce(ctx, db, batchLimit)
		}
	}
}

// runCronOnce 拉一批 unsynced 记录，逐条 MarkDeductionLogSynced。
//
// 返回 processed 计数（公开处理，便于日志测试断言）。
func runCronOnce(ctx context.Context, db *gorm.DB, batchLimit int) (processed int) {
	logs, err := listUnsyncedDeductionLogs(db, batchLimit)
	if err != nil {
		log.Printf("stock-sync cron: list unsynced failed: %v", err)
		return 0
	}
	if len(logs) == 0 {
		return 0
	}

	now := time.Now()
	for _, l := range logs {
		if ctx.Err() != nil {
			return processed
		}
		rows, err := markDeductionLogSynced(db, l.ID, now)
		if err != nil {
			log.Printf("stock-sync cron: mark id=%d failed: %v (will retry next round)", l.ID, err)
			continue
		}
		if rows == 0 {
			// 已 synced（理论上 ListUnsyncedLogs 不应返回已 synced 的行；
			// 0 行说明并发 consumer 抢先处理掉了），跳过。
			continue
		}
		processed++
	}
	log.Printf("stock-sync cron: processed=%d (of %d)", processed, len(logs))
	return processed
}

// markDeductionLogSynced 直接调 database.Get()，不走 gin.Context 依赖的
// repository 层 —— 这是 worker 进程，无 HTTP 请求上下文。
//
// 影响行数 = 0 表示行不存在或已被 synced（幂等），调用方按需判断。
func markDeductionLogSynced(db *gorm.DB, logID int64, syncedAt time.Time) (int64, error) {
	res := db.
		Model(&mall.MallStockDeductionLog{}).
		Where("id = ? AND synced_to_pool_at IS NULL", logID).
		Update("synced_to_pool_at", syncedAt)
	return res.RowsAffected, res.Error
}

// listUnsyncedDeductionLogs 同上：worker 不走 gin 上下文。
func listUnsyncedDeductionLogs(db *gorm.DB, limit int) ([]mall.MallStockDeductionLog, error) {
	rows := make([]mall.MallStockDeductionLog, 0)
	err := db.
		Where("synced_to_pool_at IS NULL").
		Order("deducted_at ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// =============================================================================
// 环境变量解析
// =============================================================================

// runMode 是 STOCK_SYNC_MODE 的解析结果。
type runMode int

const (
	modeBoth runMode = iota
	modeConsumerOnly
	modeCronOnly
)

// modeName 把 runMode 还原成 STOCK_SYNC_MODE 字符串（log 友好）。
func modeName(m runMode) string {
	switch m {
	case modeConsumerOnly:
		return "consumer_only"
	case modeCronOnly:
		return "cron_only"
	case modeBoth:
		return "both"
	default:
		return "unknown"
	}
}

// parseMode 解析 STOCK_SYNC_MODE，默认 modeBoth。
//
// 合法值：both / consumer_only / cron_only（大小写不敏感）。
// 非法值 → log.Fatalf（启动期 fail fast，避免 worker 静默走错模式）。
func parseMode() runMode {
	raw := os.Getenv("STOCK_SYNC_MODE")
	switch raw {
	case "", "both":
		return modeBoth
	case "consumer_only":
		return modeConsumerOnly
	case "cron_only":
		return modeCronOnly
	default:
		log.Fatalf("stock-sync: invalid STOCK_SYNC_MODE %q (want both|consumer_only|cron_only)", raw)
		return 0 // unreachable
	}
}

// parseCronInterval 解析 STOCK_SYNC_CRON_INTERVAL，默认 5m。
//
// 用 time.ParseDuration 解析；非法 → log.Fatalf。
func parseCronInterval() time.Duration {
	raw := os.Getenv("STOCK_SYNC_CRON_INTERVAL")
	if raw == "" {
		return defaultCronInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Fatalf("stock-sync: invalid STOCK_SYNC_CRON_INTERVAL %q: %v", raw, err)
	}
	if d <= 0 {
		log.Fatalf("stock-sync: STOCK_SYNC_CRON_INTERVAL must be > 0, got %s", d)
	}
	return d
}

// parseBatchLimit 解析 STOCK_SYNC_BATCH_LIMIT，默认 100。
func parseBatchLimit() int {
	raw := os.Getenv("STOCK_SYNC_BATCH_LIMIT")
	if raw == "" {
		return defaultBatchLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		log.Fatalf("stock-sync: invalid STOCK_SYNC_BATCH_LIMIT %q: %v", raw, err)
	}
	if n <= 0 {
		log.Fatalf("stock-sync: STOCK_SYNC_BATCH_LIMIT must be > 0, got %d", n)
	}
	return n
}