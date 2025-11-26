package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/pilot-net/icmp-mon/pkg/types"
)

// PilotClient defines the interface for the Pilot API client.
// This is a placeholder - implement when Pilot API access is available.
type PilotClient interface {
	// ListIPPools returns all IP pools from Pilot API.
	ListIPPools(ctx context.Context) ([]PilotIPPool, error)

	// GetIPPoolDetails returns detailed information for an IP pool.
	GetIPPoolDetails(ctx context.Context, poolID int) (*PilotIPPoolDetails, error)
}

// PilotIPPool represents an IP pool from the Pilot API.
type PilotIPPool struct {
	ID                 int
	NetworkAddress     string
	NetworkSize        int
	GatewayAddress     string
	FirstUsableAddress string
	LastUsableAddress  string
	VLANID             *int
	ServiceID          *int
	SubscriberID       *int
	SubscriberName     *string
	LocationID         *int
	LocationAddress    *string
	City               *string
	Region             *string
	POPName            *string
	GatewayDevice      *string
}

// PilotIPPoolDetails contains detailed pool info including IP assignments.
type PilotIPPoolDetails struct {
	Pool      PilotIPPool
	AssignedIPs []PilotIPAssignment
}

// PilotIPAssignment represents an IP assignment within a pool.
type PilotIPAssignment struct {
	IPAddress string
	IPType    types.IPType // gateway, infrastructure, customer
	Notes     string
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

	// CreateTargetForSubnet creates a new target for a subnet.
	CreateTargetForSubnet(ctx context.Context, target *types.Target) error
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

func (w *PilotSyncWorker) createSubnet(ctx context.Context, pool *PilotIPPool) error {
	subnet := &types.Subnet{
		PilotSubnetID:      &pool.ID,
		NetworkAddress:     pool.NetworkAddress,
		NetworkSize:        pool.NetworkSize,
		GatewayAddress:     &pool.GatewayAddress,
		FirstUsableAddress: &pool.FirstUsableAddress,
		LastUsableAddress:  &pool.LastUsableAddress,
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
		State:              "active",
	}

	if err := w.store.CreateSubnet(ctx, subnet); err != nil {
		return err
	}

	w.logger.Info("subnet created from Pilot",
		"subnet_id", subnet.ID,
		"pilot_id", pool.ID,
		"network", pool.NetworkAddress,
	)

	return nil
}

func (w *PilotSyncWorker) updateSubnet(ctx context.Context, existing *types.Subnet, pool *PilotIPPool) error {
	// Update fields from Pilot
	existing.NetworkAddress = pool.NetworkAddress
	existing.NetworkSize = pool.NetworkSize
	existing.GatewayAddress = &pool.GatewayAddress
	existing.FirstUsableAddress = &pool.FirstUsableAddress
	existing.LastUsableAddress = &pool.LastUsableAddress
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

	return w.store.UpdateSubnet(ctx, existing)
}

func (w *PilotSyncWorker) subnetNeedsUpdate(existing *types.Subnet, pool *PilotIPPool) bool {
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
	if ptrStringNotEqual(existing.POPName, pool.POPName) {
		return true
	}
	if ptrStringNotEqual(existing.GatewayDevice, pool.GatewayDevice) {
		return true
	}
	return false
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
