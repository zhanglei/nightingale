package config

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"github.com/toolkits/pkg/file"
)

type ConfYaml struct {
	Debug   bool                `yaml:"debug"`
	MinStep int                 `yaml:"minStep"`
	Logger  LoggerSection       `yaml:"logger"`
	HTTP    HTTPSection         `yaml:"http"`
	RPC     RPCSection          `yaml:"rpc"`
	Tsdb    TsdbSection         `yaml:"tsdb"`
	Judge   JudgeSection        `yaml:"judge"`
	Index   IndexSection        `yaml:"index"`
	API     map[string][]string `yaml:"api"`
}

type IndexSection struct {
	Path    string `yaml:"path"`
	Timeout int    `yaml:"timeout"`
}

type LoggerSection struct {
	Dir       string `yaml:"dir"`
	Level     string `yaml:"level"`
	KeepHours uint   `yaml:"keepHours"`
}

type HTTPSection struct {
	Enabled bool   `yaml:"enabled"`
	Access  string `yaml:"access"`
}

type RPCSection struct {
	Enabled bool `yaml:"enabled"`
}

type TsdbSection struct {
	Enabled     bool                    `yaml:"enabled"`
	Batch       int                     `yaml:"batch"`
	ConnTimeout int                     `yaml:"connTimeout"`
	CallTimeout int                     `yaml:"callTimeout"`
	WorkerNum   int                     `yaml:"workerNum"`
	MaxConns    int                     `yaml:"maxConns"`
	MaxIdle     int                     `yaml:"maxIdle"`
	Replicas    int                     `yaml:"replicas"`
	Cluster     map[string]string       `yaml:"cluster"`
	ClusterList map[string]*ClusterNode `json:"clusterList"`
}

type JudgeSection struct {
	Enabled     bool `yaml:"enabled"`
	Batch       int  `yaml:"batch"`
	ConnTimeout int  `yaml:"connTimeout"`
	CallTimeout int  `yaml:"callTimeout"`
	WorkerNum   int  `yaml:"workerNum"`
	MaxConns    int  `yaml:"maxConns"`
	MaxIdle     int  `yaml:"maxIdle"`
	Replicas    int  `yaml:"replicas"`
}

var (
	Config *ConfYaml
)

// CLUSTER NODE
type ClusterNode struct {
	Addrs []string `json:"addrs"`
}

func NewClusterNode(addrs []string) *ClusterNode {
	return &ClusterNode{addrs}
}

// map["node"]="host1,host2" --> map["node"]=["host1", "host2"]
func formatClusterItems(cluster map[string]string) map[string]*ClusterNode {
	ret := make(map[string]*ClusterNode)
	for node, clusterStr := range cluster {
		items := strings.Split(clusterStr, ",")
		nitems := make([]string, 0)
		for _, item := range items {
			nitems = append(nitems, strings.TrimSpace(item))
		}
		ret[node] = NewClusterNode(nitems)
	}

	return ret
}

func Parse(conf string) error {
	bs, err := file.ReadBytes(conf)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", conf, err)
	}

	viper.SetConfigType("yaml")
	err = viper.ReadConfig(bytes.NewBuffer(bs))
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", conf, err)
	}

	viper.SetDefault("http.enabled", true)
	viper.SetDefault("index.path", "/api/index/counter/fullmatch")
	viper.SetDefault("index.timeout", 3000)
	viper.SetDefault("minStep", 1)

	viper.SetDefault("tsdb", map[string]interface{}{
		"enabled":     true,
		"batch":       200, //每次拉取文件的个数
		"replicas":    500, //一致性has虚拟节点
		"workerNum":   32,
		"maxConns":    32,   //查询和推送数据的并发个数
		"maxIdle":     32,   //建立的连接池的最大空闲数
		"connTimeout": 1000, //链接超时时间，单位毫秒
		"callTimeout": 3000, //访问超时时间，单位毫秒
	})

	viper.SetDefault("judge", map[string]interface{}{
		"enabled":     true,
		"batch":       200, //每次拉取文件的个数
		"workerNum":   32,
		"maxConns":    32,   //查询和推送数据的并发个数
		"maxIdle":     32,   //建立的连接池的最大空闲数
		"connTimeout": 1000, //链接超时时间，单位毫秒
		"callTimeout": 3000, //访问超时时间，单位毫秒
	})

	err = viper.Unmarshal(&Config)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v\n", conf, err)
	}

	Config.Tsdb.ClusterList = formatClusterItems(Config.Tsdb.Cluster)

	return err
}
