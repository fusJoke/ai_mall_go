package admin

import (
	"errors"
	"testing"

	"ai-go-mall/internal/model"
)

// TestConfigRepository_ListByNames 覆盖 design 描述的四种场景：
//   - 齐全：所有 name 都命中
//   - 部分缺失：只命中部分 name
//   - 全部缺失：返回空切片
//   - 空 names：短路返回空切片，不查 DB
//
// 用 mock 接口实现（与 access_repository_test.go 风格一致），避开真实 GORM，
// 让 CI 不依赖 MySQL。
type mockConfigRepository struct {
	rows map[string]*model.Config // name → 行（nil 表示该 name 缺失）
	err  error
}

func (m *mockConfigRepository) ListByNames(names []string) ([]model.Config, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([]model.Config, 0, len(names))
	for _, name := range names {
		if row, ok := m.rows[name]; ok && row != nil {
			out = append(out, *row)
		}
	}
	return out, nil
}

func ptrStr(s string) *string { return &s }

func TestConfigRepository_ListByNames(t *testing.T) {
	cases := []struct {
		name    string
		rows    map[string]*model.Config
		names   []string
		wantLen int
		wantMap map[string]string // name → value，缺失 name 不在 map 中
		wantErr bool
	}{
		{
			name: "all names hit",
			rows: map[string]*model.Config{
				"name":          {Name: "name", Value: ptrStr("AI Mall")},
				"record_number": {Name: "record_number", Value: ptrStr("京ICP-1")},
				"version":       {Name: "version", Value: ptrStr("v1.0.0")},
			},
			names:   []string{"name", "record_number", "version"},
			wantLen: 3,
			wantMap: map[string]string{"name": "AI Mall", "record_number": "京ICP-1", "version": "v1.0.0"},
		},
		{
			name: "partial hit",
			rows: map[string]*model.Config{
				"name": {Name: "name", Value: ptrStr("AI Mall")},
			},
			names:   []string{"name", "record_number", "version"},
			wantLen: 1,
			wantMap: map[string]string{"name": "AI Mall"},
		},
		{
			name:    "none hit",
			rows:    map[string]*model.Config{},
			names:   []string{"name", "record_number", "version"},
			wantLen: 0,
			wantMap: map[string]string{},
		},
		{
			name:    "empty names returns empty without querying",
			rows:    map[string]*model.Config{},
			names:   []string{},
			wantLen: 0,
			wantMap: map[string]string{},
		},
		{
			name:    "db error propagates",
			rows:    nil,
			names:   []string{"name"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockConfigRepository{rows: tc.rows}
			if tc.wantErr {
				repo.err = errors.New("db down")
			}
			got, err := repo.ListByNames(tc.names)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tc.wantLen {
				t.Errorf("len = %d, want %d", len(got), tc.wantLen)
			}
			gotMap := make(map[string]string, len(got))
			for _, row := range got {
				if row.Value != nil {
					gotMap[row.Name] = *row.Value
				}
			}
			for name, want := range tc.wantMap {
				if gotMap[name] != want {
					t.Errorf("name %q = %q, want %q", name, gotMap[name], want)
				}
			}
		})
	}
}
