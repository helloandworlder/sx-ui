package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/helloandworlder/sx-ui/v2/logger"
)

// PublicIpInfo represents a single detected public IP with metadata.
type PublicIpInfo struct {
	Ip        string `json:"ip"`
	Country   string `json:"country,omitempty"`
	City      string `json:"city,omitempty"`
	Isp       string `json:"isp,omitempty"`
	LastSeen  string `json:"lastSeen"`
	Interface string `json:"interface,omitempty"`
}

// IpScannerService detects public IPs on residential nodes.
type IpScannerService struct {
	NodeMetaService NodeMetaService
}

// GetPublicIps returns the cached public IPs from NodeMeta.
func (s *IpScannerService) GetPublicIps() ([]PublicIpInfo, error) {
	raw, err := s.NodeMetaService.Get("public_ips")
	if err != nil || raw == "" {
		return []PublicIpInfo{}, nil
	}
	var ips []PublicIpInfo
	if err := json.Unmarshal([]byte(raw), &ips); err != nil {
		return nil, err
	}
	return ips, nil
}

// ScanPublicIps probes all local non-loopback interfaces to detect public IPs,
// then persists the result in NodeMeta.
func (s *IpScannerService) ScanPublicIps() error {
	logger.Info("Starting public IP scan...")

	// First try getting the main public IP via external service
	var results []PublicIpInfo

	mainIp, err := s.detectMainPublicIp()
	if err == nil && mainIp != "" {
		results = append(results, PublicIpInfo{
			Ip:       mainIp,
			LastSeen: time.Now().UTC().Format(time.RFC3339),
		})
	}

	// Also scan local interfaces for additional IPs
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		seen := make(map[string]bool)
		if mainIp != "" {
			seen[mainIp] = true
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.IsLoopback() || ipNet.IP.IsPrivate() || ipNet.IP.IsLinkLocalUnicast() {
				continue
			}
			ip := ipNet.IP.String()
			if !seen[ip] {
				seen[ip] = true
				results = append(results, PublicIpInfo{
					Ip:        ip,
					Interface: ipNet.IP.String(),
					LastSeen:  time.Now().UTC().Format(time.RFC3339),
				})
			}
		}
	}

	data, _ := json.Marshal(results)
	if err := s.NodeMetaService.Set("public_ips", string(data)); err != nil {
		return fmt.Errorf("failed to save public IPs: %w", err)
	}

	logger.Infof("Public IP scan complete: %d IPs found", len(results))
	return nil
}

// detectMainPublicIp calls an external API to get the node's main public IP.
func (s *IpScannerService) detectMainPublicIp() (string, error) {
	// Try multiple services for reliability
	services := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	}

	client := &http.Client{Timeout: 10 * time.Second}

	for _, svc := range services {
		resp, err := client.Get(svc)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		ip := strings.TrimSpace(string(body))
		if net.ParseIP(ip) != nil {
			return ip, nil
		}
	}
	return "", fmt.Errorf("all IP detection services failed")
}
