package query

import (
	"github.com/didi/nightingale/src/toolkits/address"
)

var (
	TransferConnPools *ConnPools = &ConnPools{}

	connTimeout int32
	callTimeout int32

	Config SeriesQuerySection
)

type SeriesQuerySection struct {
	MaxConn          int    `json:"maxConn"`     //
	MaxIdle          int    `json:"maxIdle"`     //
	ConnTimeout      int    `json:"connTimeout"` // 连接超时
	CallTimeout      int    `json:"callTimeout"` // 请求超时
	IndexPath        string `json:"indexPath"`
	IndexCallTimeout int    `json:"indexCallTimeout"` // 请求超时
}

func Init(cfg SeriesQuerySection) {
	Config = cfg
	TransferConnPools = CreateConnPools(Config.MaxConn, Config.MaxIdle,
		Config.ConnTimeout, Config.CallTimeout, address.GetRPCAddresses("transfer"))
}
