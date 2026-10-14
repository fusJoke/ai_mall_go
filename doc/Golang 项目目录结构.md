# Golang WEB 项目目录结构

```bash
.
├── .claude                     # Claude Code 相关
│
├── cmd/
│   ├── main.go                 # 命令行统一入口
│   │  
│   ├── migrate/                # 数据库迁移命令
│   │   ├── commands.go
│   │   ├── migrate.go
│   │   ├── prefix.go
│   │   │
│   │   └── migrations          # 迁移文件
│   │
│   └── serve                   # 启动 API 服务命令
│
├── internal/                   # 私有应用代码（仅限本项目内部使用）
│   ├── infra/                  # 基础设施层
│   │   ├── database/           # 初始化数据库连接等
│   │   ├── captcha/            # 验证码相关
│   │   ├── config/             # 配置系统初始化、配置加载
│   │   ├── token/              # 令牌相关
│   │   │   ├── driver          # 令牌存储驱动，文件名代表对应驱动的实现
│   │   │   │   ├── database.go
│   │   │   │   └── redis.go
│   │   │   └─── token.go
│   │   └── upload/             # 文件上传相关，也是多驱动设计
│   │
│   ├── kit/                    # 业务层通用封装、成套辅助工具
│   ├── dto/                    # 数据传输对象
│   ├── middleware/             # 中间件
│   ├── handler/                # 处理器，解析请求（JSON→struct）> 调用 Service > 序列化响应（struct→JSON），可做参数格式合法性检查，如参数非空（只处理参数、返回值）
│   ├── model/                  # 数据模型（只定义表结构，表名函数）
│   ├── repository/             # 数据访问层（只处理数据库操作，把数据从 DB 搬到内存，或反过来）
│   ├── router/                 # 路由定义及路由自动发现实现
│   └── service/                # 业务服务层，参数业务合法性检查，如用户名不存在、余额是否足够，逻辑编排、事务控制（只处理业务逻辑）
│
├── pkg/                        # 公共库代码（可被其他项目引用）
│   ├── random/                 # 随机数生成模块
│   ├── filesystem/             # 文件系统模块
│   └── ...                     # 其他可复用模块
│
├── config/                     # 配置文件模板（如 YAML/JSON）
├── .env.yaml.example           # 环境配置文件示例
├── .gitignore                  # Git 忽略规则
├── AGENTS.md                   # AGENTS 的工作指导文档
├── go.mod                      # 依赖管理
├── go.sum                      # 依赖校验
├── LICENSE                     # 许可证文件
└── README.md                   # 项目说明
```

## 结构特点
1. 采用多入口设计（即 cmd 下面一个目录一个入口）
2. 分层架构的调用关系（固定流向，永远不变）：`Handler > Service > Repository > Model`



