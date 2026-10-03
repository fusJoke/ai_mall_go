package draw_check

import (
	"errors"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/domain/chain"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
)

// =============================================================================
// stub
// =============================================================================

type stubSeckillRepo struct {
	getByIDFunc func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error)
}

func (s *stubSeckillRepo) GetByID(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
	if s.getByIDFunc != nil {
		return s.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}

func (s *stubSeckillRepo) Create(c *gin.Context, e *mall.MallSeckillActivity) (int64, error) {
	return 0, nil
}
func (s *stubSeckillRepo) Update(c *gin.Context, e *mall.MallSeckillActivity) error { return nil }
func (s *stubSeckillRepo) Delete(c *gin.Context, id int64) error                    { return nil }
func (s *stubSeckillRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
	return nil, 0, nil
}
func (s *stubSeckillRepo) Get(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
	return nil, nil
}
func (s *stubSeckillRepo) MarkRedisInitialized(c *gin.Context, id int64) error { return nil }
func (s *stubSeckillRepo) ListActive(c *gin.Context, now time.Time, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
	return nil, 0, nil
}
func (s *stubSeckillRepo) ListBySupplier(c *gin.Context, sid int64, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
	return nil, 0, nil
}
func (s *stubSeckillRepo) ToggleStatus(c *gin.Context, id int64, st mall.MallBlindBoxStatus) error {
	return nil
}
func (s *stubSeckillRepo) SumPoolStock(c *gin.Context, bbID int64) (int, error) { return 0, nil }
func (s *stubSeckillRepo) InsertDeductionLog(c *gin.Context, log *mall.MallStockDeductionLog) error {
	return nil
}
func (s *stubSeckillRepo) CountDeductionLogsBySeckillIDs(c *gin.Context, ids []int64) (map[int64]int64, error) {
	return nil, nil
}
func (s *stubSeckillRepo) MarkDeductionLogSynced(c *gin.Context, id int64, at time.Time) (int64, error) {
	return 0, nil
}
func (s *stubSeckillRepo) ListUnsyncedLogs(c *gin.Context, limit int) ([]mall.MallStockDeductionLog, error) {
	return nil, nil
}

var _ mallRepo.SeckillRepository = (*stubSeckillRepo)(nil)

// =============================================================================
// 单测
// =============================================================================

func TestTimeWindowCheck_NormalSource_FastReturn(t *testing.T) {
	// normal 链路：time_window 不查 seckill，seckill mock 不会被触发。
	check := NewTimeWindowCheck(&stubSeckillRepo{})

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if err != nil {
		t.Fatalf("normal source should fast return nil, got err = %v", err)
	}
}

func TestTimeWindowCheck_SeckillInWindow_Pass(t *testing.T) {
	now := time.Now()
	sec := &stubSeckillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
			return &mall.MallSeckillActivity{
				ID:               id,
				Status:           mall.StatusActive,
				RedisInitialized: true,
				StartAt:          now.Add(-1 * time.Hour),
				EndAt:            now.Add(1 * time.Hour),
			}, nil
		},
	}
	check := NewTimeWindowCheck(sec)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceSeckill, UserID: 1, SeckillID: 7, Now: now,
	})
	if err != nil {
		t.Fatalf("seckill in window should pass, got err = %v", err)
	}
}

func TestTimeWindowCheck_SeckillNotStarted_Reject(t *testing.T) {
	now := time.Now()
	sec := &stubSeckillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
			return &mall.MallSeckillActivity{
				ID:               id,
				Status:           mall.StatusActive,
				RedisInitialized: true,
				StartAt:          now.Add(1 * time.Hour), // 未来
				EndAt:            now.Add(2 * time.Hour),
			}, nil
		},
	}
	check := NewTimeWindowCheck(sec)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceSeckill, UserID: 1, SeckillID: 7, Now: now,
	})
	if !errors.Is(err, chain.ErrSeckillNotInWindow) {
		t.Fatalf("err = %v, want chain.ErrSeckillNotInWindow", err)
	}
}

func TestTimeWindowCheck_SeckillAlreadyEnded_Reject(t *testing.T) {
	now := time.Now()
	sec := &stubSeckillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
			return &mall.MallSeckillActivity{
				ID:               id,
				Status:           mall.StatusActive,
				RedisInitialized: true,
				StartAt:          now.Add(-2 * time.Hour),
				EndAt:            now.Add(-1 * time.Hour), // 已结束
			}, nil
		},
	}
	check := NewTimeWindowCheck(sec)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceSeckill, UserID: 1, SeckillID: 7, Now: now,
	})
	if !errors.Is(err, chain.ErrSeckillNotInWindow) {
		t.Fatalf("err = %v, want chain.ErrSeckillNotInWindow", err)
	}
}

func TestTimeWindowCheck_SeckillNotFound_Reject(t *testing.T) {
	sec := &stubSeckillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	check := NewTimeWindowCheck(sec)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceSeckill, UserID: 1, SeckillID: 7, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrSeckillNotAvailable) {
		t.Fatalf("err = %v, want chain.ErrSeckillNotAvailable", err)
	}
}

func TestTimeWindowCheck_SeckillDisabled_Reject(t *testing.T) {
	now := time.Now()
	sec := &stubSeckillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
			return &mall.MallSeckillActivity{
				ID:               id,
				Status:           mall.StatusDisabled,
				RedisInitialized: true,
				StartAt:          now.Add(-1 * time.Hour),
				EndAt:            now.Add(1 * time.Hour),
			}, nil
		},
	}
	check := NewTimeWindowCheck(sec)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceSeckill, UserID: 1, SeckillID: 7, Now: now,
	})
	if !errors.Is(err, chain.ErrSeckillNotAvailable) {
		t.Fatalf("err = %v, want chain.ErrSeckillNotAvailable", err)
	}
}

func TestTimeWindowCheck_RedisNotInitialized_Reject(t *testing.T) {
	now := time.Now()
	sec := &stubSeckillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
			return &mall.MallSeckillActivity{
				ID:               id,
				Status:           mall.StatusActive,
				RedisInitialized: false, // 关键：未 init Redis
				StartAt:          now.Add(-1 * time.Hour),
				EndAt:            now.Add(1 * time.Hour),
			}, nil
		},
	}
	check := NewTimeWindowCheck(sec)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceSeckill, UserID: 1, SeckillID: 7, Now: now,
	})
	if !errors.Is(err, chain.ErrSeckillNotAvailable) {
		t.Fatalf("err = %v, want chain.ErrSeckillNotAvailable", err)
	}
}

func TestTimeWindowCheck_SeckillIDZero_Reject(t *testing.T) {
	// 防御编程错误：seckill source 但 SeckillID=0 → fast reject。
	check := NewTimeWindowCheck(&stubSeckillRepo{})

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceSeckill, UserID: 1, SeckillID: 0, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrSeckillNotAvailable) {
		t.Fatalf("err = %v, want chain.ErrSeckillNotAvailable", err)
	}
}

func TestTimeWindowCheck_PropagatesDBError(t *testing.T) {
	dbErr := errors.New("db down")
	sec := &stubSeckillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
			return nil, dbErr
		},
	}
	check := NewTimeWindowCheck(sec)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceSeckill, UserID: 1, SeckillID: 7, Now: time.Now(),
	})
	if !errors.Is(err, dbErr) {
		t.Fatalf("err = %v, want wrap dbErr", err)
	}
}

func TestTimeWindowCheck_DefaultsToNowWhenInputNowZero(t *testing.T) {
	now := time.Now()
	sec := &stubSeckillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
			return &mall.MallSeckillActivity{
				ID:               id,
				Status:           mall.StatusActive,
				RedisInitialized: true,
				StartAt:          now.Add(-1 * time.Hour),
				EndAt:            now.Add(1 * time.Hour),
			}, nil
		},
	}
	check := NewTimeWindowCheck(sec)

	// input.Now 是 zero time → 节点内部 fallback 到 time.Now() → 通过
	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceSeckill, UserID: 1, SeckillID: 7,
		// Now 故意留空
	})
	if err != nil {
		t.Fatalf("zero Now should fallback to time.Now(), got err = %v", err)
	}
}

func TestTimeWindowCheck_Name(t *testing.T) {
	check := NewTimeWindowCheck(&stubSeckillRepo{})
	if check.Name() != "time_window" {
		t.Errorf("Name() = %q, want %q", check.Name(), "time_window")
	}
}
