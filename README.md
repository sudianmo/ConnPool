# MyConnPool

手写 MySQL 连接池的学习与复现项目，仅供学习参考。

## 项目简介

基于 Go 语言从零实现一个简易的数据库连接池 `MyConnPool`，并通过学生信息 CRUD 的 REST API 来演示连接池的使用。

核心特性：

- 最大活跃连接数与最大空闲连接数控制
- 连接生命周期管理（超时淘汰）
- 基于 channel 的连接存取，配合 mutex 保护并发安全
- 连接池状态查询（活跃数 / 空闲数）

## 技术栈

- **Go 1.24**
- **Gin** — HTTP 框架
- **GORM** — ORM（基于 MySQL）
- **go-redis** — Redis 缓存

## 项目结构

```
.
├── main.go           # 入口，初始化连接池并启动 HTTP 服务
├── myfunc/
│   ├── db.go         # 连接池定义、MySQL/Redis 初始化
│   └── student.go    # 学生 CRUD API（含 Redis 缓存）
```

## 理解要点

连接池本身不自动关闭——它的设计目的是在整个应用生命周期内复用连接，避免频繁创建/销毁连接的开销。`MyConnPool` 的关键设计：

1. `chan *WrapedConn` 作为连接队列，`GetConnection` 时取，归还时 `returnConnection` 塞回
2. 取连接时检查是否超时，超时则关闭重建
3. 连接不足时若未达上限则动态创建，否则阻塞等待归还

## 免责声明

本项目仅为学习连接池原理的练手作品，不适用于生产环境。
