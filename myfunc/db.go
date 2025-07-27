package myfunc

import (
	"context"
	_ "database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	_ "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var GormDB *gorm.DB //定义为全局变量，包级别的变量
var err error
var rdb *redis.Client
var ConnPool *MyConnPool

type Student struct {
	//连接池必要的参数
	//MaxOpenConns 最大活动连接数
	//
	Name  string `json:"name"`
	Tel   int    `json:"tel"`
	Study string `json:"study"`
	Id    int    `json:"id" gorm:"primaryKey"`
}
type WrapedConn struct {
	DB        *gorm.DB
	CreatedAt time.Time
}
type MyConnPool struct {
	MaxOpenConns    int
	MaxIdleConns    int              //最大空闲连接
	ConnMaxLifetime time.Duration    //连接生存时间
	connections     chan *WrapedConn //通道类型，加入存储时间的变量
	activeConns     int
	mu              sync.Mutex //互斥锁用于保护并发访问
	dsn             string
}

func (cp *MyConnPool) createConnection() (*WrapedConn, error) {

	gormDB, err := gorm.Open(mysql.Open(cp.dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetConnMaxLifetime(cp.ConnMaxLifetime)
	return &WrapedConn{
		DB:        gormDB,
		CreatedAt: time.Now(),
	}, nil
	//*gorm对象不能直接设置连接池，Mysql的db对象是吗
}
func NewConnPool(dsn string, maxIdle, maxOpen int, maxLifetime time.Duration) (*MyConnPool, error) {
	if maxIdle <= 0 || maxOpen <= 0 {
		return nil, fmt.Errorf("无效连接")
	}
	if maxOpen < maxIdle {
		return nil, fmt.Errorf("max idle connections must be positive integers")
	}
	cp := &MyConnPool{
		MaxOpenConns:    maxOpen,
		MaxIdleConns:    maxIdle,
		ConnMaxLifetime: maxLifetime,
		connections:     make(chan *WrapedConn, maxOpen),
		dsn:             dsn,
	}

	//填入连接
	for i := 0; i < maxOpen; i++ {
		conn, err := cp.createConnection()
		if err != nil {
			cp.Close()
		}
		cp.connections <- conn
		cp.activeConns++
	}
	return cp, nil
}
func closeConnection(conn *gorm.DB) {
	sqlDB, _ := conn.DB()
	if err := sqlDB.Close(); err != nil {
		fmt.Println(err)
	}
}
func (cp *MyConnPool) Close() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	for len(cp.connections) > 0 {
		select {
		case conn := <-cp.connections:
			closeConnection(conn.DB)
			cp.activeConns--
		default:
			return
		}
	}
}

func (cp *MyConnPool) GetConnection() (*gorm.DB, error) {

	select {
	case wrapped := <-cp.connections:
		if time.Since(wrapped.CreatedAt) > cp.ConnMaxLifetime {
			closeConnection(wrapped.DB)

			return cp.GetConnection()

		}
		cp.mu.Lock()
		//连接被借出
		cp.activeConns++
		cp.mu.Unlock()
		return wrapped.DB, nil
	default:
		if cp.activeConns < cp.MaxOpenConns {
			wrapped, err := cp.createConnection()
			if err != nil {
				return nil, err
			}
			cp.mu.Lock()
			cp.activeConns++
			cp.mu.Unlock()
			return wrapped.DB, nil
		}
		wrapped := <-cp.connections
		if time.Since(wrapped.CreatedAt) > cp.ConnMaxLifetime {
			closeConnection(wrapped.DB)
			return cp.GetConnection()
		}
		cp.mu.Lock()
		cp.activeConns++
		cp.mu.Unlock()
		return wrapped.DB, nil
	}

}

func (cp *MyConnPool) returnConnection(conn *gorm.DB) {
	cp.mu.Lock()
	cp.activeConns--
	cp.mu.Unlock()

	cp.connections <- &WrapedConn{
		DB:        conn,
		CreatedAt: time.Now(),
	}
}

// 监测连接是否成功
func (cp *MyConnPool) validateConnection(conn *WrapedConn) bool {
	if conn == nil || conn.DB == nil {
		return false
	}
	sqlDB, err := conn.DB.DB()
	if err != nil {
		return false
	}
	//检查连接是否有效
	if err := sqlDB.Ping(); err != nil {
		return false
	}
	if time.Since(conn.CreatedAt) > cp.ConnMaxLifetime {
		return false
	}

	return true
}
func (cp *MyConnPool) GetStatus() (active, idle int) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	idle = len(cp.connections)
	active = cp.activeConns
	return active, idle
}
func (Student) TableName() string {
	return "students"
}

func InitDB() error {
	dsn := "root:31415926@tcp(127.0.0.1:3306)/Student_sql"

	ConnPool, err = NewConnPool(dsn, 5, 10, 5*time.Minute)
	if err != nil {
		return err
	}
	GormDB, err = ConnPool.GetConnection()
	if err != nil {
		return err
	}
	defer ConnPool.returnConnection(GormDB)

	if err := GormDB.AutoMigrate(&Student{}); err != nil {
		return err
	}
	fmt.Println("Successfully connected to DB")
	return nil

}

func InitRedis() error {
	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
		PoolSize: 10,
	})

	if err := rdb.Ping(context.Background()); err != nil {

	}
	fmt.Println("Successfully connected to Redis")
	return nil
}
