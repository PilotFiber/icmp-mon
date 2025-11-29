package pilot

import (
	"net"
	"strings"
)

// IPPool represents an IP subnet from the Flight Deck API with enriched metadata.
// This is returned by the Client.ListIPPools method.
type IPPool struct {
	// Core subnet fields
	ID             int    `json:"id"`
	NetworkAddress string `json:"network_address"`
	NetworkSize    int    `json:"network_size"`
	Name           *string `json:"name,omitempty"`

	// Computed address fields
	GatewayAddress     *string `json:"gateway_address,omitempty"`      // First usable IP (network + 1)
	FirstUsableAddress *string `json:"first_usable_address,omitempty"` // Gateway + 1
	LastUsableAddress  *string `json:"last_usable_address,omitempty"`  // Broadcast - 1

	// Subnet type (0=NA, 1=WAN, 2=LAN)
	SubnetType     *int    `json:"subnet_type,omitempty"`
	SubnetTypeName *string `json:"subnet_type_name,omitempty"` // "WAN", "LAN", "NA"

	// Network topology
	VLANID        *int    `json:"vlan_id,omitempty"`
	GatewayDevice *string `json:"gateway_device,omitempty"` // CSW hostname
	POPName       *string `json:"pop_name,omitempty"`       // Extracted from CSW hostname (e.g., "jfk00")

	// Service relationship
	ServiceID     *int    `json:"service_id,omitempty"`
	ServiceStatus *string `json:"service_status,omitempty"` // Flight Deck service status (e.g., "active", "cancelled")

	// Subscriber info
	SubscriberID   *int    `json:"subscriber_id,omitempty"`
	SubscriberName *string `json:"subscriber_name,omitempty"`

	// Location info
	LocationID      *int    `json:"location_id,omitempty"`
	LocationAddress *string `json:"location_address,omitempty"`
	City            *string `json:"city,omitempty"`
	Region          *string `json:"region,omitempty"`
}

// ComputeAddresses calculates gateway, first usable, and last usable addresses from the network CIDR.
// For a /29 network like 104.192.216.120/29:
//   - Network address: .120
//   - Gateway (first usable): .121
//   - First customer usable: .122
//   - Last usable: .126
//   - Broadcast: .127
//
// For a /31 network (point-to-point link) like 10.0.0.0/31:
//   - Lower IP (.0): Gateway (ISP side)
//   - Upper IP (.1): Customer IP (to be monitored)
//
// For a /32 network (single host):
//   - The single IP is typically the gateway itself, no customer IPs
func (p *IPPool) ComputeAddresses() {
	_, ipNet, err := net.ParseCIDR(p.NetworkAddress)
	if err != nil {
		return
	}

	// Get network address as uint32
	ip := ipNet.IP.To4()
	if ip == nil {
		return // IPv6 not supported
	}

	// Calculate network size
	ones, bits := ipNet.Mask.Size()
	hostBits := bits - ones
	numHosts := uint32(1) << uint(hostBits)

	// Convert IP to uint32 for arithmetic
	networkAddr := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])

	// Handle /32 - single host, typically gateway itself, no customer IPs
	if ones == 32 {
		gateway := uint32ToIP(networkAddr)
		p.GatewayAddress = &gateway
		// No first/last usable - it's a single IP
		return
	}

	// Handle /31 - point-to-point link (RFC 3021)
	// Lower IP is gateway (ISP side), upper IP is customer
	if ones == 31 {
		gateway := uint32ToIP(networkAddr)       // Lower IP (.0) = gateway
		customer := uint32ToIP(networkAddr + 1)  // Upper IP (.1) = customer
		p.GatewayAddress = &gateway
		p.FirstUsableAddress = &customer
		p.LastUsableAddress = &customer
		return
	}

	// Standard subnets (/30 and larger)
	if numHosts < 4 {
		return
	}

	// Gateway is network + 1
	gatewayAddr := networkAddr + 1
	gateway := uint32ToIP(gatewayAddr)
	p.GatewayAddress = &gateway

	// First usable is gateway + 1
	firstUsableAddr := networkAddr + 2
	firstUsable := uint32ToIP(firstUsableAddr)
	p.FirstUsableAddress = &firstUsable

	// Last usable is broadcast - 1 (broadcast = network + numHosts - 1)
	lastUsableAddr := networkAddr + numHosts - 2
	lastUsable := uint32ToIP(lastUsableAddr)
	p.LastUsableAddress = &lastUsable
}

// ExtractPOPName parses the POP name from a CSW hostname like "csw24.jfk00.pilotfiber.net" -> "jfk00"
func (p *IPPool) ExtractPOPName() {
	if p.GatewayDevice == nil || *p.GatewayDevice == "" {
		return
	}
	parts := strings.Split(*p.GatewayDevice, ".")
	if len(parts) >= 2 {
		// Second part is the POP name (e.g., "jfk00")
		pop := parts[1]
		p.POPName = &pop
	}
}

// uint32ToIP converts a uint32 to a dotted-decimal IPv4 string
func uint32ToIP(n uint32) string {
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n)).String()
}

// CIDR returns the subnet in CIDR notation (e.g., "192.168.1.0/24").
func (p *IPPool) CIDR() string {
	return p.NetworkAddress + "/" + itoa(p.NetworkSize)
}

// TypeString returns a human-readable subnet type.
func (p *IPPool) TypeString() string {
	if p.SubnetTypeName != nil {
		return *p.SubnetTypeName
	}
	return "unknown"
}

func itoa(i int) string {
	// Simple int to string without importing strconv
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
