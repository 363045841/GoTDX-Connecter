// 领域层对 gotdx 客户端的统一查询入口，隐藏 Main/Ex 域分发细节。
package domain

import "KlineChartQuantGo/services/tdx-api/internal/client"

func mainCall[T any](operation func(client.MainQuerier) (T, error)) (T, error) {
	return client.QueryMain(operation)
}

func exCall[T any](operation func(client.ExQuerier) (T, error)) (T, error) {
	return client.QueryEx(operation)
}
