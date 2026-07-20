package api

import (
	"time"

	"KlineChartQuantGo/services/tdx-api/internal/client"
	"github.com/bensema/gotdx"
	"github.com/gin-gonic/gin"
)

type hostProbeRequest struct {
	Type    string `json:"type"`
	Timeout int    `json:"timeout"`
}

type hostListResponse struct {
	Main   []string `json:"main"`
	Ex     []string `json:"ex"`
	MAC    []string `json:"mac"`
	MACEx  []string `json:"mac_ex"`
	Broker []string `json:"broker"`
}

func handleHostProbe(c *gin.Context) {
	var req hostProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	timeout := time.Duration(req.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	var hosts []gotdx.HostInfo
	switch req.Type {
	case "ex":
		hosts = client.ExHosts()
	case "mac":
		hosts = client.MACHosts()
	case "mac_ex":
		hosts = client.MACExHosts()
	case "broker":
		hosts = client.BrokerHosts()
	default:
		hosts = client.MainHosts()
	}

	results := client.Probe(hosts, timeout)
	c.JSON(200, results)
}

func handleHostList(c *gin.Context) {
	resp := hostListResponse{
		Main:   hostAddresses(client.MainHosts()),
		Ex:     hostAddresses(client.ExHosts()),
		MAC:    hostAddresses(client.MACHosts()),
		MACEx:  hostAddresses(client.MACExHosts()),
		Broker: hostAddresses(client.BrokerHosts()),
	}
	c.JSON(200, resp)
}

func hostAddresses(hosts []gotdx.HostInfo) []string {
	addrs := make([]string, len(hosts))
	for i, h := range hosts {
		addrs[i] = h.Address()
	}
	return addrs
}
