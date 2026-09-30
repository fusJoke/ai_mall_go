package admin

import "testing"

// TestParseRuleIDs 覆盖 design D4 中规定的解析契约。
//
// 用例清单（与 tasks.md 3.4 一致）：
//   - 通配符 "*"
//   - 空字符串
//   - 规整列表 "1,2,3"
//   - 含空格的列表 " 1, 2 , 5 "
//   - 含非数字段 "abc,1"
//   - 全分隔符 ","
func TestParseRuleIDs(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		wantWildcard bool
		wantIDs      []uint
	}{
		{
			name:         "wildcard literal",
			input:        "*",
			wantWildcard: true,
			wantIDs:      nil,
		},
		{
			name:         "wildcard with surrounding whitespace",
			input:        "  *  ",
			wantWildcard: true,
			wantIDs:      nil,
		},
		{
			name:         "empty string",
			input:        "",
			wantWildcard: false,
			wantIDs:      nil,
		},
		{
			name:         "whitespace-only string",
			input:        "   ",
			wantWildcard: false,
			wantIDs:      nil,
		},
		{
			name:         "comma-separated IDs",
			input:        "1,2,3",
			wantWildcard: false,
			wantIDs:      []uint{1, 2, 3},
		},
		{
			name:         "IDs with whitespace",
			input:        " 1, 2 , 5 ",
			wantWildcard: false,
			wantIDs:      []uint{1, 2, 5},
		},
		{
			name:         "non-numeric segment skipped",
			input:        "abc,1",
			wantWildcard: false,
			wantIDs:      []uint{1},
		},
		{
			name:         "all non-numeric yields empty slice",
			input:        "abc,def",
			wantWildcard: false,
			wantIDs:      []uint{},
		},
		{
			name:         "only comma yields empty slice",
			input:        ",",
			wantWildcard: false,
			wantIDs:      []uint{},
		},
		{
			name:         "trailing comma yields no extra",
			input:        "1,2,",
			wantWildcard: false,
			wantIDs:      []uint{1, 2},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotWildcard, gotIDs := ParseRuleIDs(tc.input)

			if gotWildcard != tc.wantWildcard {
				t.Errorf("wildcard = %v, want %v", gotWildcard, tc.wantWildcard)
			}

			// 都为 nil 视为相等
			if gotIDs == nil && tc.wantIDs == nil {
				return
			}
			// 都为空切片视为相等
			if len(gotIDs) == 0 && len(tc.wantIDs) == 0 {
				return
			}
			if !equalIDs(gotIDs, tc.wantIDs) {
				t.Errorf("ids = %v, want %v", gotIDs, tc.wantIDs)
			}
		})
	}
}

// equalIDs 比较两个 []uint 是否相等。nil 与空切片都视为相等。
func equalIDs(a, b []uint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
