// Command migrate 是不依赖 API 服务的 schema 迁移 CLI。
//
// 子命令：
//
//	up [N]        应用剩余 N 个 up；缺省 N=0 表示全部
//	down [N]      回滚最近 N 个 down；缺省 N=1
//	goto V        跳转至版本 V（应用或回滚）
//	version       打印当前 `<version> <dirty>` 状态
//	force V       强制把 version 设为 V、dirty 设为 false（人工恢复）
//	create NAME   在 migrationsDir 下生成 `<N+1>_NAME.up.sql` 与 `<N+1>_NAME.down.sql`
//
// migrationsDir 默认 `./cmd/migrate/migrations`（相对 cwd）。
// 可由 `-path DIR` 标志覆盖，仅用于本地开发；生产部署 cmd/serve 走 ldflags，
// 不在运行期切路径（spec Requirement: DSN 与 migrations 路径来源）。
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/migrate"
)

// defaultMigrationsDir is the conventional location for *.sql migration files.
// Override with `-path DIR`. cmd/serve 走 ldflags 与本变量无关。
const defaultMigrationsDir = "cmd/migrate/migrations"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	// `-path DIR` 可以出现在子命令之前（`migrate -path DIR create NAME`）或
	// 子命令之后（`migrate create NAME -path DIR`）；都接受。
	var pathFlag string
	var i int
	for i = 1; i < len(os.Args); i++ {
		if os.Args[i] == "-path" && i+1 < len(os.Args) {
			pathFlag = os.Args[i+1]
			i++
			continue
		}
		break
	}
	if pathFlag == "" {
		pathFlag = defaultMigrationsDir
	}

	if i >= len(os.Args) {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[i]
	args := []string{}
	if i+1 < len(os.Args) {
		args = os.Args[i+1:]
		// 也允许 `migrate create NAME -path DIR` 的形态，把结尾的 -path DIR 切掉。
		for j := 0; j < len(args); j++ {
			if args[j] == "-path" && j+1 < len(args) {
				pathFlag = args[j+1]
				args = append(args[:j], args[j+2:]...)
				break
			}
		}
	}

	if err := run(cmd, args, pathFlag); err != nil {
		log.Fatalf("migrate %s: %v", cmd, err)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `usage: migrate [-path DIR] <command> [args]

Commands:
  up [N]        apply at most N up migrations (N=0 -> all remaining)
  down [N]      apply at most N down migrations (default N=1)
  goto V        migrate to version V
  version       print current schema_migrations state
  force V       force version to V (clears dirty)
  create NAME   create new <N+1>_NAME.{up,down}.sql pair

Options:
  -path DIR     migrations directory (default %q)
`, defaultMigrationsDir)
}

func run(cmd string, args []string, migrationsDir string) error {
	switch cmd {
	case "up":
		return runUp(args, migrationsDir)
	case "down":
		return runDown(args, migrationsDir)
	case "goto":
		return runGoto(args, migrationsDir)
	case "version":
		return runVersion(args, migrationsDir)
	case "force":
		return runForce(args, migrationsDir)
	case "create":
		return runCreate(args, migrationsDir)
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

// runUp 解析可选正整数 N（缺省 0 = 全部）。
func runUp(args []string, migrationsDir string) error {
	n, err := parseStep(args, 0)
	if err != nil {
		return err
	}
	mig, closer, err := openMig(migrationsDir)
	if err != nil {
		return err
	}
	defer closer()

	if n == 0 {
		if err := mig.Up(); err != nil {
			return err
		}
		v, _, _ := mig.Version()
		fmt.Printf("up to latest: now at version %d\n", v)
		return nil
	}
	if err := mig.Steps(n); err != nil {
		return err
	}
	v, _, _ := mig.Version()
	fmt.Printf("up %d step(s): now at version %d\n", n, v)
	return nil
}

// runDown 解析可选正整数 N（缺省 1）。
func runDown(args []string, migrationsDir string) error {
	n, err := parseStep(args, 1)
	if err != nil {
		return err
	}
	mig, closer, err := openMig(migrationsDir)
	if err != nil {
		return err
	}
	defer closer()

	if err := mig.Steps(-n); err != nil {
		return err
	}
	v, _, _ := mig.Version()
	fmt.Printf("down %d step(s): now at version %d\n", n, v)
	return nil
}

// runGoto 解析必填的版本号 V。
func runGoto(args []string, migrationsDir string) error {
	if len(args) != 1 {
		return errors.New("goto requires exactly one argument: V")
	}
	v, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid version %q: %w", args[0], err)
	}
	mig, closer, err := openMig(migrationsDir)
	if err != nil {
		return err
	}
	defer closer()

	if err := mig.Goto(uint(v)); err != nil {
		return err
	}
	cur, _, _ := mig.Version()
	fmt.Printf("goto %d: now at version %d\n", uint(v), cur)
	return nil
}

// runVersion 打印 `<version> <dirty>`。尚未初始化时按 spec 输出 `no migration`。
func runVersion(args []string, migrationsDir string) error {
	if len(args) != 0 {
		return errors.New("version takes no arguments")
	}
	mig, closer, err := openMig(migrationsDir)
	if err != nil {
		return err
	}
	defer closer()

	v, dirty, err := mig.Version()
	if err != nil {
		// ErrNoMigrationApplied 与 ErrInvalidState 都是"非 v/dirty 状态"
		// 的 sentinel；CLI 统一按 "no migration" 输出，不让 wrapper 内部细节
		// 漏到 operator 面前。
		if errors.Is(err, migrate.ErrNoMigrationApplied) || errors.Is(err, migrate.ErrInvalidState) {
			fmt.Println("no migration")
			return nil
		}
		return err
	}
	dirtyStr := "false"
	if dirty {
		dirtyStr = "true"
	}
	fmt.Printf("%d %s\n", v, dirtyStr)
	return nil
}

// runForce 解析必填的版本号 V。
func runForce(args []string, migrationsDir string) error {
	if len(args) != 1 {
		return errors.New("force requires exactly one argument: V")
	}
	v, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid version %q: %w", args[0], err)
	}
	mig, closer, err := openMig(migrationsDir)
	if err != nil {
		return err
	}
	defer closer()

	if err := mig.Force(v); err != nil {
		return err
	}
	fmt.Printf("forced version to %d\n", v)
	return nil
}

// runCreate 生成新的 `<N+1>_NAME.{up,down}.sql` 占位文件。
//
// NAME 不允许包含下划线（避免和版本号下划线冲突）、不允许包含空字符、不允许长度 0。
// 占位文件含 `-- +migrate Up` / `-- +migrate Down` 注释，方便后续编辑识别边界。
func runCreate(args []string, migrationsDir string) error {
	if len(args) != 1 {
		return errors.New("create requires exactly one argument: NAME")
	}
	name := args[0]
	if name == "" {
		return errors.New("NAME cannot be empty")
	}
	// NAME 只允许字母 / 数字 / '-' / '_'，作为文件名片段是安全的；不允许路径分隔符 / 空格 / 点。
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return fmt.Errorf("NAME must contain only [a-zA-Z0-9_-]; invalid rune %q", r)
		}
	}

	max, err := nextVersion(migrationsDir)
	if err != nil {
		return err
	}
	versionPrefix := fmt.Sprintf("%06d", max)

	upPath := filepath.Join(migrationsDir, versionPrefix+"_"+name+".up.sql")
	downPath := filepath.Join(migrationsDir, versionPrefix+"_"+name+".down.sql")

	upBody := "-- +migrate Up\n-- TODO: write the up migration here.\nSELECT 1;\n"
	downBody := "-- +migrate Down\n-- TODO: write the down migration here.\nSELECT 1;\n"

	if err := os.WriteFile(upPath, []byte(upBody), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", upPath, err)
	}
	if err := os.WriteFile(downPath, []byte(downBody), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", downPath, err)
	}
	fmt.Printf("created %s\ncreated %s\n", upPath, downPath)
	return nil
}

// openMig 读 config.Get().Database + migrationsDir，构造 Migrator 并返回
// 一个调用 Migrator.Close 的 closer；调用方应 defer closer()。
//
// 当前调用链：openMig 内部总是 RunInit.Run()、再 migrate.New；后者已经会 Ping，
// 所以这里不再二次 Ping。
func openMig(migrationsDir string) (*migrate.Migrator, func(), error) {
	if err := config.Init("."); err != nil {
		return nil, nil, fmt.Errorf("init config: %w", err)
	}
	cfg := config.Get().Database
	mig, err := migrate.New(cfg, migrationsDir)
	if err != nil {
		return nil, nil, err
	}
	return mig, func() {
		_ = mig.Close()
	}, nil
}

// parseStep 把命令行参数解析为 N，正整数 / 0 都允许，缺省值为 d。
func parseStep(args []string, d int) (int, error) {
	fs := flag.NewFlagSet("step", flag.ContinueOnError)
	_ = fs // 当前子命令没有额外 flag；保留接口供未来扩展。
	if len(args) == 0 {
		return d, nil
	}
	if len(args) != 1 {
		return 0, fmt.Errorf("expects 0 or 1 argument, got %d", len(args))
	}
	n, err := strconv.Atoi(args[0])
	if err != nil {
		return 0, fmt.Errorf("invalid step %q: %w", args[0], err)
	}
	if n < 0 {
		return 0, fmt.Errorf("step must be non-negative, got %d", n)
	}
	return n, nil
}

// nextVersion 在 migrationsDir 下枚举文件名，解析出最大版本号 +1。
// 空目录 → 1（首个 migration）。
func nextVersion(migrationsDir string) (int, error) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 1, nil
		}
		return 0, fmt.Errorf("read migrations dir: %w", err)
	}
	type versioned struct {
		name    string
		version int
	}
	var pairs []versioned
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if v, ok := parseVersionPrefixLocal(e.Name()); ok {
			pairs = append(pairs, versioned{e.Name(), v})
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].version < pairs[j].version })
	if len(pairs) == 0 {
		return 1, nil
	}
	return pairs[len(pairs)-1].version + 1, nil
}

// parseVersionPrefixLocal is the same as internal/infra/migrate.parseVersionPrefix
// 但放在 cmd/migrate 里避免 export 该函数。保持格式一致：6 位前导零 + 下划线。
func parseVersionPrefixLocal(name string) (int, bool) {
	if len(name) < 8 {
		return 0, false
	}
	v := 0
	for i := 0; i < 6; i++ {
		ch := name[i]
		if ch < '0' || ch > '9' {
			return 0, false
		}
		v = v*10 + int(ch-'0')
	}
	if name[6] != '_' {
		return 0, false
	}
	return v, true
}
