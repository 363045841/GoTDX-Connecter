package api

import (
	"log"
	"time"
)

// kline 分页约定（A 股 / 指数 / 扩展行情共用）：
//   - start=0 为最近一页，增大 start 向更早历史走
//   - 页内时间升序，page[0] 为该页最旧
//   - 推进步长必须用本页实际 len；扩展日线常约 700 < 请求 count，不能用 len<count 当结束
//   - 结束：空页，或最旧时间已早于请求 startDate（该页仍保留，由 filter 裁剪）

// indexFallbackPageSize 部分 start+798 会撞 gotdx invalid kline datetime，缩小 count 再试
const indexFallbackPageSize uint16 = 400

// maxKLinePages 分页页数硬上限：防御异常数据源（或注入点）持续返回新页导致内存无限增长。
// 远大于任何真实 K 线历史深度，只兜底终止条件失效的场景。
const maxKLinePages = 20000

// paginateFromRecent 从最近向历史翻页，按实际条数推进 start。
// toleratePartial：某页失败且已有数据时截断返回（指数深页 dateNum 损坏）。
func paginateFromRecent[T any](
	pageSize uint16,
	fetch func(start uint32, count uint16) ([]T, error),
	oldestTime func(item T) (time.Time, bool),
	startDate time.Time,
	toleratePartial bool,
) ([]T, error) {
	out := make([]T, 0, int(pageSize))
	for pageNum, start := 0, uint32(0); ; pageNum++ {
		if pageNum >= maxKLinePages {
			log.Printf("[gotdx] kline pagination aborted after %d pages (safety cap)", pageNum)
			break
		}
		page, err := fetch(start, pageSize)
		if err != nil {
			if toleratePartial && len(out) > 0 {
				log.Printf("[gotdx] kline page stop at start=%d after %d bars: %v", start, len(out), err)
				break
			}
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		out = append(out, page...)
		if t, ok := oldestTime(page[0]); ok && t.Before(startDate) {
			break
		}
		start += uint32(len(page))
	}
	return out, nil
}

// clampUint16Start 主站协议 start 为 uint16；超出则视为无更多历史
func clampUint16Start(start uint32) (uint16, bool) {
	if start > 0xffff {
		return 0, false
	}
	return uint16(start), true
}
