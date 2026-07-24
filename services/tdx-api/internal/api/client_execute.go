package api

import "KlineChartQuantGo/services/tdx-api/internal/client"

func mainCall[T any](operation func(client.MainQuerier) (T, error)) (T, error) {
	return client.QueryMain(operation)
}

func exCall[T any](operation func(client.ExQuerier) (T, error)) (T, error) {
	return client.QueryEx(operation)
}

func macCall[T any](operation func(client.MACQuerier) (T, error)) (T, error) {
	return client.QueryMAC(operation)
}
