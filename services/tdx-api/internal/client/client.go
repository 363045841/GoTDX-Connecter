package client

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bensema/gotdx"
	"github.com/bensema/gotdx/proto"
)

var (
	defaultManagerOnce sync.Once
	defaultManager     *Manager
)

func DefaultManager() *Manager {
	defaultManagerOnce.Do(func() {
		defaultManager = newManager(map[Domain]domainConfig{
			DomainMain: defaultDomainConfig(DomainMain),
			DomainEx:   defaultDomainConfig(DomainEx),
			DomainMAC:  defaultDomainConfig(DomainMAC),
		})
	})
	return defaultManager
}

func Start() error {
	return DefaultManager().Start()
}

func Close() error {
	return DefaultManager().Close()
}

func CloseContext(ctx context.Context) error {
	return DefaultManager().CloseContext(ctx)
}

type MainQuerier interface {
	GetServerHeartbeat() (*proto.HeartBeatReply, error)
	StockAll(uint8) ([]proto.Security, error)
	StockCount(uint8) (uint16, error)
	StockList(uint8, uint32, uint32) ([]proto.Security, error)
	StockQuotesDetail([]uint8, []string) ([]proto.SecurityQuote, error)
	StockKLine(uint16, uint8, string, uint16, uint16, uint16, uint16) ([]proto.SecurityBar, error)
	StockTickChart(uint8, string, uint16, uint16) ([]proto.MinuteTimeData, error)
	StockHistoryTickChart(uint32, uint8, string) ([]proto.HistoryMinuteTimeData, error)
	StockHistoryFullTransaction(uint32, uint8, string) ([]proto.HistoryTransactionData, error)
	StockFullTransaction(uint8, string) ([]proto.TransactionData, error)
	StockIndexInfo(uint8, string) (*proto.GetIndexInfoReply, error)
	StockTransaction(uint8, string, uint16, uint16) ([]proto.TransactionData, error)
	StockHistoryTransaction(uint32, uint8, string, uint16, uint16) ([]proto.HistoryTransactionData, error)
	GetIndexBars(uint16, uint8, string, uint16, uint16) (*proto.GetIndexBarsReply, error)
}

type ExQuerier interface {
	ExCount() (uint32, error)
	ExList(uint32, uint16) ([]proto.ExListItem, error)
	ExQuote(uint8, string) (*proto.ExQuoteItem, error)
	ExQuotes([]uint8, []string) ([]proto.ExQuoteItem, error)
	ExKLine(uint8, string, uint16, uint32, uint16, uint16) ([]proto.ExKLineItem, error)
	ExTickChart(uint8, string, uint32) ([]proto.ExTickChartData, error)
	ExHistoryTransaction(uint32, uint8, string) ([]proto.ExHistoryTransactionItem, error)
	ExTable() (string, error)
}

type MACQuerier interface {
	MACBoardList(uint16, uint32) ([]proto.MACBoardListItem, error)
	MACBoardMembers(string, uint32) ([]proto.MACBoardMemberItem, error)
	MACBoardMembersQuotes(string, uint32) ([]proto.MACBoardMemberQuoteItem, error)
	MACBoardMembersQuotesDynamic(string, uint32, uint16, uint8, [20]byte) (*proto.MACBoardMembersQuotesDynamicReply, error)
	MACSymbolQuotes([]uint8, []string, [20]byte) (*proto.MACSymbolQuotesReply, error)
	MACQuotes(uint8, string) (*proto.MACQuotesReply, error)
	MACTransactions(uint8, string, uint32, uint32) ([]proto.MACTransactionItem, error)
	MACAuction(uint8, string, uint32, uint32) ([]proto.MACAuctionItem, error)
	MACTickCharts(uint8, string, uint32, uint16) (*proto.MACTickChartsReply, error)
	MACSymbolInfo(uint8, string) (*proto.MACSymbolInfoReply, error)
	MACCapitalFlow(uint8, string) (*proto.MACCapitalFlowReply, error)
	MACMarketMonitor(uint8, uint32, uint32) ([]proto.MACMarketMonitorItem, error)
	MACServerInfo() (*proto.MACServerInfoReply, error)
}

// QueryMain executes one bounded, read-only main-market request with one recovery retry.
func QueryMain[T any](operation func(MainQuerier) (T, error)) (T, error) {
	return execute(DefaultManager(), DomainMain, func(c *gotdx.Client) (T, error) { return operation(c) })
}

func ProbeMain[T any](operation func(MainQuerier) (T, error)) (T, error) {
	result, _, err := executeOnce(DefaultManager(), DomainMain, func(c *gotdx.Client) (T, error) { return operation(c) })
	return result, err
}

// QueryEx executes one bounded, read-only extended-market request with one recovery retry.
func QueryEx[T any](operation func(ExQuerier) (T, error)) (T, error) {
	return execute(DefaultManager(), DomainEx, func(c *gotdx.Client) (T, error) { return operation(c) })
}

// QueryMAC executes one bounded, read-only MAC request with one recovery retry.
func QueryMAC[T any](operation func(MACQuerier) (T, error)) (T, error) {
	return execute(DefaultManager(), DomainMAC, func(c *gotdx.Client) (T, error) { return operation(c) })
}

func defaultDomainConfig(domain Domain) domainConfig {
	return domainConfig{
		build:   func() *gotdx.Client { return buildDomainClient(domain) },
		connect: func(c *gotdx.Client) error { return connectDomainClient(domain, c) },
		close:   func(c *gotdx.Client) error { return c.Disconnect() },
	}
}

func buildDomainClient(domain Domain) *gotdx.Client {
	var label, envKey string
	var defaults []string
	switch domain {
	case DomainMain:
		label, envKey, defaults = "main", "GOTDX_MAIN_HOSTS", gotdx.MainHostAddresses()
	case DomainEx:
		label, envKey, defaults = "ex", "GOTDX_EX_HOSTS", gotdx.ExHostAddresses()
	case DomainMAC:
		label, envKey, defaults = "mac", "GOTDX_MAC_HOSTS", gotdx.MACHostAddresses()
	default:
		return nil
	}
	hosts := resolveHosts(label, envKey, defaults, 2*time.Second)
	if len(hosts) == 0 {
		log.Printf("[gotdx] no reachable %s hosts, using full default list", label)
		hosts = defaults
	}
	if len(hosts) == 0 {
		return nil
	}
	log.Printf("[gotdx] %s hosts (%d): %v", label, len(hosts), hosts)
	opts := []gotdx.Option{gotdx.WithTimeoutSec(6)}
	switch domain {
	case DomainMain:
		opts = append(opts, gotdx.WithTCPAddress(hosts[0]), gotdx.WithTCPAddressPool(hosts[1:]...))
	case DomainEx:
		opts = append(opts, gotdx.WithExTCPAddress(hosts[0]), gotdx.WithExTCPAddressPool(hosts[1:]...))
	case DomainMAC:
		opts = append(opts, gotdx.WithMacTCPAddress(hosts[0]), gotdx.WithMacTCPAddressPool(hosts[1:]...))
	}
	if os.Getenv("GOTDX_AUTO_SELECT") == "1" {
		opts = append(opts, gotdx.WithAutoSelectFastest(true))
	}
	switch domain {
	case DomainEx:
		return gotdx.NewEx(opts...)
	case DomainMAC:
		return gotdx.NewMAC(opts...)
	default:
		return gotdx.New(opts...)
	}
}

func connectDomainClient(domain Domain, c *gotdx.Client) error {
	if c == nil {
		return errors.New("client is nil")
	}
	switch domain {
	case DomainMain:
		_, err := c.Connect()
		return err
	case DomainEx:
		_, err := c.ConnectEx()
		return err
	case DomainMAC:
		return c.ConnectMAC()
	default:
		return fmt.Errorf("unknown gotdx domain %q", domain)
	}
}

func resolveHosts(label, envKey string, all []string, timeout time.Duration) []string {
	if v := os.Getenv(envKey); v != "" {
		addrs := strings.Split(v, ",")
		log.Printf("[gotdx] %s: using env %s=%v", label, envKey, addrs)
		return addrs
	}

	log.Printf("[gotdx] %s: probing %d hosts (timeout=%v)...", label, len(all), timeout)
	results := gotdx.ProbeAddresses(all, timeout)

	reachable := make([]string, 0, len(results))
	for _, r := range results {
		if r.Reachable {
			log.Printf("[gotdx] %s: %-21s reachable  latency=%v", label, r.Address, r.Latency)
			reachable = append(reachable, r.Address)
		} else {
			log.Printf("[gotdx] %s: %-21s unreachable", label, r.Address)
		}
	}
	log.Printf("[gotdx] %s: %d/%d hosts reachable", label, len(reachable), len(all))
	return reachable
}

func Probe(hosts []gotdx.HostInfo, timeout time.Duration) []gotdx.HostProbeResult {
	return gotdx.ProbeHosts(hosts, timeout)
}

func Fastest(hosts []gotdx.HostInfo, timeout time.Duration) (gotdx.HostProbeResult, error) {
	return gotdx.FastestHost(hosts, timeout)
}

func MainHosts() []gotdx.HostInfo {
	return gotdx.MainHosts()
}

func ExHosts() []gotdx.HostInfo {
	return gotdx.ExHosts()
}

func MACHosts() []gotdx.HostInfo {
	return gotdx.MACHosts()
}

func MACExHosts() []gotdx.HostInfo {
	return gotdx.MACExHosts()
}

func BrokerHosts() []gotdx.HostInfo {
	return gotdx.BrokerHosts()
}
