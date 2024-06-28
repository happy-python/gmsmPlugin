package gmsmPlugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/piaohao/godis"
	"github.com/tjfoc/gmsm/sm3"
)

// Config the plugin configuration.
type Config struct {
	RedisHost     string `json:"redisHost,omitempty"`
	RedisPassword string `json:"redisPassword,omitempty"`
	RedisPort     int    `json:"redisPort,omitempty"`
	RedisDb       int    `json:"redisDb,omitempty"`
	SMAlgorithm   string `json:"smAlgorithm,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		SMAlgorithm:   "SM3",
		RedisHost:     "localhost",
		RedisPassword: "",
		RedisPort:     6379,
		RedisDb:       0,
	}
}

// MyPlugin plugin.
type MyPlugin struct {
	next        http.Handler
	smAlgorithm string
	redis       *godis.Redis
}

// New created a new MyPlugin plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	// redis
	redis := godis.NewRedis(&godis.Option{
		Host:     config.RedisHost,
		Port:     config.RedisPort,
		Password: config.RedisPassword,
		Db:       config.RedisDb,
	})

	return &MyPlugin{
		smAlgorithm: config.SMAlgorithm,
		redis:       redis,
		next:        next,
	}, nil
}

func (p *MyPlugin) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	p.redis.Set("godis", "1")
	value, _ := p.redis.Get("godis")

	os.Stdout.WriteString("获取redis的值为: " + value + "\n")

	bytes, _ := io.ReadAll(req.Body)

	// 实现自己的逻辑
	if p.smAlgorithm == "SM3" {
		hasher := sm3.New()
		hasher.Write(bytes)
		hash := hasher.Sum(nil)

		// 将字节切片转换为十六进制字符串表示
		hashHex := fmt.Sprintf("%x", hash)
		// 打印输出

		os.Stdout.WriteString("加密后的值为: " + hashHex + "\n")

		m, _ := json.Marshal(map[string]interface{}{"result": hashHex, "code": 0, "message": "ok"})

		rw.Write(m)
	} else {
		// 原样输出
		rw.Write(bytes)
	}
	// a.next.ServeHTTP(rw, req)
}
