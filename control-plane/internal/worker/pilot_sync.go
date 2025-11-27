package worker

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/pilot-net/icmp-mon/control-plane/internal/pilot"
	"github.com/pilot-net/icmp-mon/control-plane/internal/store"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

// PilotClient defines the interface for the Pilot API client.
type PilotClient interface {
	// ListIPPools returns all IP pools from Pilot API.
	ListIPPools(ctx context.Context) ([]pilot.IPPool, error)
}

// PilotSyncStore defines the storage interface for the sync worker.
type PilotSyncStore interface {
	// GetSubnetByPilotID retrieves a subnet by its Pilot API ID.
	GetSubnetByPilotID(ctx context.Context, pilotID int) (*types.Subnet, error)

	// CreateSubnet creates a new subnet.
	CreateSubnet(ctx context.Context, subnet *types.Subnet) error

	// UpdateSubnet updates an existing subnet.
	UpdateSubnet(ctx context.Context, subnet *types.Subnet) error

	// ArchiveSubnet archives a subnet that no longer exists in Pilot.
	ArchiveSubnet(ctx context.Context, id, reason string, archiveAutoTargets bool) error

	// ListSubnets returns all active subnets.
	ListSubnets(ctx context.Context) ([]types.Subnet, error)

	// CreateAutoTarget creates a target with auto-seeding parameters.
	CreateAutoTarget(ctx context.Context, params store.AutoTargetParams) error

	// UpdateTargetTagsBySubnet updates tags on all targets in a subnet.
	UpdateTargetTagsBySubnet(ctx context.Context, subnetID string, tags map[string]string) error
}

// PilotSyncConfig holds configuration for the sync worker.
type PilotSyncConfig struct {
	// Interval between sync runs.
	Interval time.Duration

	// FullSyncInterval is how often to do a full sync (check for removed pools).
	FullSyncInterval time.Duration

	// AutoCreateTargets controls whether to auto-create targets for IPs in pools.
	AutoCreateTargets bool
}

// DefaultPilotSyncConfig returns sensible defaults.
func DefaultPilotSyncConfig() PilotSyncConfig {
	return PilotSyncConfig{
		Interval:          15 * time.Minute,
		FullSyncInterval:  24 * time.Hour,
		AutoCreateTargets: true,
	}
}

// PilotSyncWorker synchronizes subnets from Pilot API.
type PilotSyncWorker struct {
	client     PilotClient
	store      PilotSyncStore
	config     PilotSyncConfig
	logger     *slog.Logger
	stopCh     chan struct{}
	lastFullSync time.Time
}

// NewPilotSyncWorker creates a new Pilot sync worker.
func NewPilotSyncWorker(client PilotClient, store PilotSyncStore, config PilotSyncConfig, logger *slog.Logger) *PilotSyncWorker {
	return &PilotSyncWorker{
		client: client,
		store:  store,
		config: config,
		logger: logger.With("component", "pilot_sync"),
		stopCh: make(chan struct{}),
	}
}

// Start begins the sync worker in a goroutine.
func (w *PilotSyncWorker) Start(ctx context.Context) {
	go w.run(ctx)
}

// Stop signals the worker to stop.
func (w *PilotSyncWorker) Stop() {
	close(w.stopCh)
}

func (w *PilotSyncWorker) run(ctx context.Context) {
	w.logger.Info("pilot sync worker started",
		"interval", w.config.Interval,
		"full_sync_interval", w.config.FullSyncInterval,
	)

	// Run immediately on start
	w.runOnce(ctx)

	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("pilot sync worker stopping (context cancelled)")
			return
		case <-w.stopCh:
			w.logger.Info("pilot sync worker stopping (stop signal)")
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *PilotSyncWorker) runOnce(ctx context.Context) {
	start := time.Now()

	// Check if we need a full sync
	isFullSync := time.Since(w.lastFullSync) >= w.config.FullSyncInterval

	pools, err := w.client.ListIPPools(ctx)
	if err != nil {
		w.logger.Error("failed to list IP pools from Pilot", "error", err)
		return
	}

	created, updated, archived := 0, 0, 0

	// Track which Pilot IDs we've seen (for full sync)
	seenPilotIDs := make(map[int]bool)

	for _, pool := range pools {
		seenPilotIDs[pool.ID] = true

		existing, err := w.store.GetSubnetByPilotID(ctx, pool.ID)
		if err != nil {
			w.logger.Error("failed to get subnet by pilot ID", "pilot_id", pool.ID, "error", err)
			continue
		}

		if existing == nil {
			// Create new subnet
			if err := w.createSubnet(ctx, &pool); err != nil {
				w.logger.Error("failed to create subnet", "pilot_id", pool.ID, "error", err)
				continue
			}
			created++
		} else {
			// Update existing subnet if changed
			if w.subnetNeedsUpdate(existing, &pool) {
				if err := w.updateSubnet(ctx, existing, &pool); err != nil {
					w.logger.Error("failed to update subnet", "pilot_id", pool.ID, "error", err)
					continue
				}
				updated++
			}
		}
	}

	// Full sync: archive subnets that no longer exist in Pilot
	if isFullSync {
		existingSubnets, err := w.store.ListSubnets(ctx)
		if err != nil {
			w.logger.Error("failed to list subnets for full sync", "error", err)
		} else {
			for _, subnet := range existingSubnets {
				if subnet.PilotSubnetID != nil && !seenPilotIDs[*subnet.PilotSubnetID] {
					if err := w.store.ArchiveSubnet(ctx, subnet.ID, "removed_from_pilot", true); err != nil {
						w.logger.Error("failed to archive removed subnet",
							"subnet_id", subnet.ID,
							"pilot_id", *subnet.PilotSubnetID,
							"error", err,
						)
						continue
					}
					archived++
					w.logger.Info("subnet archived (removed from Pilot)",
						"subnet_id", subnet.ID,
						"pilot_id", *subnet.PilotSubnetID,
					)
				}
			}
		}
		w.lastFullSync = time.Now()
	}

	w.logger.Info("pilot sync complete",
		"duration", time.Since(start),
		"pools_fetched", len(pools),
		"created", created,
		"updated", updated,
		"archived", archived,
		"full_sync", isFullSync,
	)
}

func (w *PilotSyncWorker) createSubnet(ctx context.Context, pool *pilot.IPPool) error {
	subnet := &types.Subnet{
		ID:                 uuid.New().String(),
		PilotSubnetID:      &pool.ID,
		NetworkAddress:     pool.NetworkAddress,
		NetworkSize:        pool.NetworkSize,
		GatewayAddress:     pool.GatewayAddress,
		FirstUsableAddress: pool.FirstUsableAddress,
		LastUsableAddress:  pool.LastUsableAddress,
		VLANID:             pool.VLANID,
		ServiceID:          pool.ServiceID,
		SubscriberID:       pool.SubscriberID,
		SubscriberName:     pool.SubscriberName,
		LocationID:         pool.LocationID,
		LocationAddress:    pool.LocationAddress,
		City:               pool.City,
		Region:             pool.Region,
		POPName:            pool.POPName,
		GatewayDevice:      pool.GatewayDevice,
		SubnetType:         pool.SubnetType,
		SubnetTypeName:     pool.SubnetTypeName,
		State:              "active",
	}

	if err := w.store.CreateSubnet(ctx, subnet); err != nil {
		return err
	}

	w.logger.Info("subnet created from Pilot",
		"subnet_id", subnet.ID,
		"pilot_id", pool.ID,
		"network", pool.NetworkAddress,
		"type", pool.TypeString(),
		"subscriber", pool.SubscriberName,
	)

	// Auto-seed targets for discovery
	gatewayCreated, customerCount := w.seedSubnetTargets(ctx, subnet)
	w.logger.Info("subnet targets seeded",
		"subnet_id", subnet.ID,
		"network", subnet.NetworkAddress,
		"gateway_created", gatewayCreated,
		"customer_targets", customerCount,
	)

	return nil
}

// seedSubnetTargets creates auto-discovery targets for a subnet.
// Returns whether gateway was created and the count of customer targets created.
func (w *PilotSyncWorker) seedSubnetTargets(ctx context.Context, subnet *types.Subnet) (bool, int) {
	gatewayCreated := false
	customerCount := 0

	// Build tags from subnet metadata
	tags := buildTargetTags(subnet)

	// 1. Create gateway target if gateway_address is specified
	if subnet.GatewayAddress != nil && *subnet.GatewayAddress != "" {
		err := w.store.CreateAutoTarget(ctx, store.AutoTargetParams{
			ID:              uuid.New().String(),
			IP:              *subnet.GatewayAddress,
			SubnetID:        subnet.ID,
			IPType:          types.IPTypeGateway,
			Tier:            "vlan_gateway",
			Ownership:       types.OwnershipAuto,
			Origin:          types.OriginSync,
			MonitoringState: types.StateUnknown,
			DisplayName:     fmt.Sprintf("Gateway for %s", subnet.NetworkAddress),
			Tags:            tags,
		})
		if err != nil {
			w.logger.Warn("failed to create gateway target", "ip", *subnet.GatewayAddress, "error", err)
		} else {
			gatewayCreated = true
		}
	}

	// 2. Calculate and create customer targets for usable IP range
	usableIPs, err := calculateUsableIPs(subnet)
	if err != nil {
		w.logger.Warn("failed to calculate usable IPs", "subnet_id", subnet.ID, "error", err)
		return gatewayCreated, customerCount
	}

	for _, ip := range usableIPs {
		err := w.store.CreateAutoTarget(ctx, store.AutoTargetParams{
			ID:              uuid.New().String(),
			IP:              ip,
			SubnetID:        subnet.ID,
			IPType:          types.IPTypeCustomer,
			Tier:            "standard",
			Ownership:       types.OwnershipAuto,
			Origin:          types.OriginSync,
			MonitoringState: types.StateUnknown,
			Tags:            tags,
		})
		if err != nil {
			w.logger.Debug("failed to create customer target", "ip", ip, "error", err)
		} else {
			customerCount++
		}
	}

	return gatewayCreated, customerCount
}

// buildTargetTags creates tags from subnet metadata for target enrichment.
// All Pilot metadata is stored as tags for point-in-time filtering in metrics queries.
func buildTargetTags(subnet *types.Subnet) map[string]string {
	tags := map[string]string{
		"auto_seeded": "true",
		"pilot_sync":  "true",
		"subnet":      subnet.NetworkAddress,
	}

	// Location metadata
	if subnet.LocationAddress != nil && *subnet.LocationAddress != "" {
		tags["address"] = *subnet.LocationAddress
	}
	if subnet.LocationID != nil {
		tags["location_id"] = fmt.Sprintf("%d", *subnet.LocationID)
	}
	if subnet.City != nil && *subnet.City != "" {
		tags["city"] = *subnet.City
	}
	if subnet.Region != nil && *subnet.Region != "" {
		tags["region"] = *subnet.Region
	}

	// Subscriber metadata
	if subnet.SubscriberName != nil && *subnet.SubscriberName != "" {
		tags["subscriber"] = *subnet.SubscriberName
	}
	if subnet.SubscriberID != nil {
		tags["subscriber_id"] = fmt.Sprintf("%d", *subnet.SubscriberID)
	}

	// Service metadata
	if subnet.ServiceID != nil {
		tags["service_id"] = fmt.Sprintf("%d", *subnet.ServiceID)
	}

	// Network topology
	if subnet.POPName != nil && *subnet.POPName != "" {
		tags["pop"] = *subnet.POPName
	}
	if subnet.GatewayDevice != nil && *subnet.GatewayDevice != "" {
		tags["csw"] = *subnet.GatewayDevice
	}
	if subnet.VLANID != nil {
		tags["vlan_id"] = fmt.Sprintf("%d", *subnet.VLANID)
	}

	return tags
}

// calculateUsableIPs returns all usable customer IPs in a subnet.
// Excludes network address, gateway, and broadcast.
func calculateUsableIPs(subnet *types.Subnet) ([]string, error) {
	_, network, err := net.ParseCIDR(subnet.NetworkAddress)
	if err != nil {
		return nil, fmt.Errorf("parsing CIDR: %w", err)
	}

	var gatewayIP net.IP
	if subnet.GatewayAddress != nil && *subnet.GatewayAddress != "" {
		gatewayIP = net.ParseIP(*subnet.GatewayAddress)
	}

	maskSize, bits := network.Mask.Size()

	// Special cases
	if maskSize == 32 {
		return []string{}, nil
	}
	if maskSize == 31 {
		// Point-to-point link
		var ips []string
		ip := network.IP.To4()
		if ip == nil {
			return nil, fmt.Errorf("only IPv4 supported")
		}
		for i := 0; i < 2; i++ {
			currentIP := make(net.IP, 4)
			copy(currentIP, ip)
			currentIP[3] += byte(i)
			if gatewayIP != nil && currentIP.Equal(gatewayIP) {
				continue
			}
			ips = append(ips, currentIP.String())
		}
		return ips, nil
	}

	// Standard subnet
	hostBits := bits - maskSize
	totalHosts := (1 << hostBits) - 2

	if totalHosts <= 0 {
		return []string{}, nil
	}

	ip := network.IP.To4()
	if ip == nil {
		return nil, fmt.Errorf("only IPv4 supported")
	}
	networkInt := binary.BigEndian.Uint32(ip)

	var ips []string
	for i := uint32(1); i <= uint32(totalHosts); i++ {
		hostInt := networkInt + i
		hostIP := make(net.IP, 4)
		binary.BigEndian.PutUint32(hostIP, hostInt)

		if gatewayIP != nil && hostIP.Equal(gatewayIP) {
			continue
		}

		ips = append(ips, hostIP.String())
	}

	return ips, nil
}

func (w *PilotSyncWorker) updateSubnet(ctx context.Context, existing *types.Subnet, pool *pilot.IPPool) error {
	// Update fields from Pilot
	existing.NetworkAddress = pool.NetworkAddress
	existing.NetworkSize = pool.NetworkSize
	existing.GatewayAddress = pool.GatewayAddress
	existing.FirstUsableAddress = pool.FirstUsableAddress
	existing.LastUsableAddress = pool.LastUsableAddress
	existing.VLANID = pool.VLANID
	existing.ServiceID = pool.ServiceID
	existing.SubscriberID = pool.SubscriberID
	existing.SubscriberName = pool.SubscriberName
	existing.LocationID = pool.LocationID
	existing.LocationAddress = pool.LocationAddress
	existing.City = pool.City
	existing.Region = pool.Region
	existing.POPName = pool.POPName
	existing.GatewayDevice = pool.GatewayDevice
	existing.SubnetType = pool.SubnetType
	existing.SubnetTypeName = pool.SubnetTypeName

	// Update the subnet in database
	if err := w.store.UpdateSubnet(ctx, existing); err != nil {
		return err
	}

	// Sync tags to all targets in this subnet (for point-in-time metadata accuracy)
	tags := buildTargetTags(existing)
	if err := w.store.UpdateTargetTagsBySubnet(ctx, existing.ID, tags); err != nil {
		w.logger.Warn("failed to update target tags on subnet update",
			"subnet_id", existing.ID,
			"network", existing.NetworkAddress,
			"error", err,
		)
		// Don't fail the update if tag sync fails
	}

	return nil
}

func (w *PilotSyncWorker) subnetNeedsUpdate(existing *types.Subnet, pool *pilot.IPPool) bool {
	// Compare key fields
	if existing.NetworkAddress != pool.NetworkAddress {
		return true
	}
	if existing.NetworkSize != pool.NetworkSize {
		return true
	}
	if ptrStringNotEqual(existing.SubscriberName, pool.SubscriberName) {
		return true
	}
	if ptrStringNotEqual(existing.GatewayDevice, pool.GatewayDevice) {
		return true
	}
	if ptrIntNotEqual(existing.SubnetType, pool.SubnetType) {
		return true
	}
	return false
}

// Helper to compare pointer ints
func ptrIntNotEqual(a, b *int) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil || b == nil {
		return true
	}
	return *a != *b
}

// Helper to compare pointer strings
func ptrStringNotEqual(a, b *string) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil || b == nil {
		return true
	}
	return *a != *b
}
