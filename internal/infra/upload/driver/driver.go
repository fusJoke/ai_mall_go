// Package driver 定义上传存储后端的 Driver 接口 + 具体实现。
//
// Driver 接口放在本包（与具体实现同包），避免 upload 包 → driver 包 → upload 包的循环导入；
// upload 包通过 driver.Driver 引用接口。
package driver

import (
	"context"
	"io"
)

// Driver 是上传存储后端的统一接口。
//
// 5 个方法对应上传场景的全部外部能力：
//   - Save: 持久化（接受 io.Reader，限流 + SHA1 计算由 Service 层完成）
//   - Delete: 删除（幂等；文件不存在返回 nil）
//   - Url: 对外可访问地址
//   - Exists: 判断文件是否存在
//   - FullPath: 物理 / 逻辑定位（local 驱动返回文件系统路径，其他驱动返回 key）
type Driver interface {
	Save(ctx context.Context, content io.Reader, storedPath string) error
	Delete(ctx context.Context, storedPath string) error
	Url(storedPath string) string
	Exists(storedPath string) bool
	FullPath(storedPath string) string
}
