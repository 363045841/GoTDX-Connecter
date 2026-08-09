// 证券目录领域类型：搜索条目、加载器、持久化接口与品种 kind 常量。
package directory

import (
	"time"

	"github.com/bensema/gotdx/proto"
)

// 品种 kind：与 gotdx 品种分类对齐，供 api 包 K 线路由使用。
const (
	KindStock = "stock"
	KindIndex = "index"
	KindEx    = "ex"
)

// Item 证券目录搜索条目。
// Params 含 market|category 与 kind（stock|index|ex），前端原样带回拉 K 线。
type Item struct {
	Symbol      string `json:"symbol"`
	Description string `json:"description"`
	Exchange    string `json:"exchange"`
	Source      string `json:"source"`
	Params      map[string]any `json:"params"`
}

// Loader 证券目录数据源加载接口，隐藏 gotdx 主站/扩展行情访问细节。
type Loader interface {
	StockAll(market uint8) ([]proto.Security, error)
	ExCount() (uint32, error)
	ExList(start uint32, count uint16) ([]proto.ExListItem, error)
}

// Snapshot 证券目录持久化快照。
type Snapshot struct {
	Entries  []Item
	LoadedAt time.Time
}

// Store 证券目录持久化存储接口。
type Store interface {
	Load() (Snapshot, bool, error)
	Replace(snapshot Snapshot) error
}
