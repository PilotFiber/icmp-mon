package service

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"

	"github.com/google/uuid"
	"github.com/pilot-net/icmp-mon/control-plane/internal/store"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

// =============================================================================
// SUBNET OPERATIONS
// =============================================================================

// CreateSubnetRequest contains parameters for creating a subnet.
type CreateSubnetRequest struct {
	PilotSubnetID      *int
	NetworkAddress     string
	NetworkSize        int
	GatewayAddress     *string
	FirstUsableAddress *string
	LastUsableAddress  *string
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

// CreateSubnet creates a new subnet.
func (s *Service) CreateSubnet(ctx context.Context, req CreateSubnetRequest) (*types.Subnet, error) {
	subnet := &types.Subnet{
		ID:                 uuid.New().String(),
		PilotSubnetID:      req.PilotSubnetID,
		NetworkAddress:     req.NetworkAddress,
		NetworkSize:        req.NetworkSize,
		GatewayAddress:     req.GatewayAddress,
		FirstUsableAddress: req.FirstUsableAddress,
		LastUsableAddress:  req.LastUsableAddress,
		VLANID:             req.VLANID,
		ServiceID:          req.ServiceID,
		SubscriberID:       req.SubscriberID,
		SubscriberName:     req.SubscriberName,
		LocationID:         req.LocationID,
		LocationAddress:    req.LocationAddress,
		City:               req.City,
		Region:             req.Region,
		POPName:            req.POPName,
		GatewayDevice:      req.GatewayDevice,
		State:              "active",
	}

	if err := subnet.Validate(); err != nil {
		return nil, fmt.Errorf("invalid subnet: %w", err)
	}

	if err := s.store.CreateSubnet(ctx, subnet); err != nil {
		return nil, fmt.Errorf("creating subnet: %w", err)
	}

	s.logger.Info("subnet created",
		"id", subnet.ID,
		"network", subnet.NetworkAddress,
		"subscriber", subnet.SubscriberName,
	)

	// Auto-seed targets for discovery
	result, err := s.SeedSubnetTargets(ctx, subnet.ID)
	if err != nil {
		s.logger.Warn("failed to seed subnet targets",
			"subnet_id", subnet.ID,
			"error", err,
		)
		// Don't fail subnet creation if seeding fails
	} else {
		s.logger.Info("subnet targets seeded",
			"subnet_id", subnet.ID,
			"gateway_created", result.GatewayCreated,
			"customer_targets_created", result.CustomerTargetsCreated,
		)
	}

	return subnet, nil
}

// SeedSubnetTargetsResult contains the results of seeding targets for a subnet.
type SeedSubnetTargetsResult struct {
	GatewayCreated         bool
	CustomerTargetsCreated int
	Errors                 []string
}

// SeedSubnetTargets creates auto-discovery targets for a subnet.
// This creates:
// 1. A gateway target (if gateway_address is specified, or auto-computed for /31)
// 2. Customer targets for all usable IPs in the subnet range
//
// All targets start in UNKNOWN state and will be discovered via probing.
func (s *Service) SeedSubnetTargets(ctx context.Context, subnetID string) (*SeedSubnetTargetsResult, error) {
	subnet, err := s.store.GetSubnet(ctx, subnetID)
	if err != nil {
		return nil, fmt.Errorf("getting subnet: %w", err)
	}
	if subnet == nil {
		return nil, fmt.Errorf("subnet not found: %s", subnetID)
	}

	result := &SeedSubnetTargetsResult{}

	// For /31 networks, auto-compute gateway if not set
	// Lower IP is gateway (ISP side), upper IP is customer
	gatewayIP := ""
	if subnet.GatewayAddress != nil && *subnet.GatewayAddress != "" {
		gatewayIP = *subnet.GatewayAddress
	} else if subnet.NetworkSize == 31 {
		// Auto-compute gateway for /31: lower IP (network address)
		_, network, err := net.ParseCIDR(subnet.NetworkAddress)
		if err == nil && network.IP.To4() != nil {
			gatewayIP = network.IP.String()
			s.logger.Info("auto-computed gateway for /31 subnet",
				"subnet_id", subnetID,
				"network", subnet.NetworkAddress,
				"gateway", gatewayIP,
			)
		}
	}

	// 1. Create gateway target if we have a gateway address
	if gatewayIP != "" {
		err := s.store.CreateAutoTarget(ctx, store.AutoTargetParams{
			ID:              uuid.New().String(),
			IP:              gatewayIP,
			SubnetID:        subnetID,
			IPType:          types.IPTypeGateway,
			Tier:            "infrastructure", // Gateways get infrastructure tier
			Ownership:       types.OwnershipAuto,
			Origin:          types.OriginDiscovery,
			MonitoringState: types.StateUnknown,
			DisplayName:     fmt.Sprintf("Gateway for %s", subnet.NetworkAddress),
			Tags:            map[string]string{"auto_seeded": "true", "subnet": subnet.NetworkAddress},
		})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("gateway %s: %v", gatewayIP, err))
		} else {
			result.GatewayCreated = true
			s.logger.Debug("created gateway target",
				"ip", gatewayIP,
				"subnet_id", subnetID,
			)
		}
	}

	// 2. Calculate and create customer targets for usable IP range
	usableIPs, err := calculateUsableIPs(subnet)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("calculating usable IPs: %v", err))
		return result, nil
	}

	for _, ip := range usableIPs {
		err := s.store.CreateAutoTarget(ctx, store.AutoTargetParams{
			ID:              uuid.New().String(),
			IP:              ip,
			SubnetID:        subnetID,
			IPType:          types.IPTypeCustomer,
			Tier:            "standard", // Customer IPs get standard tier
			Ownership:       types.OwnershipAuto,
			Origin:          types.OriginDiscovery,
			MonitoringState: types.StateUnknown,
			Tags:            map[string]string{"auto_seeded": "true", "subnet": subnet.NetworkAddress},
		})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("customer %s: %v", ip, err))
		} else {
			result.CustomerTargetsCreated++
		}
	}

	s.logger.Info("subnet targets seeded",
		"subnet_id", subnetID,
		"network", subnet.NetworkAddress,
		"gateway_created", result.GatewayCreated,
		"customer_targets", result.CustomerTargetsCreated,
		"errors", len(result.Errors),
	)

	return result, nil
}

// calculateUsableIPs returns all usable customer IPs in a subnet.
// For a typical subnet:
// - Network address (first IP) is excluded
// - Gateway is excluded (usually second IP, but we check against GatewayAddress)
// - Broadcast address (last IP) is excluded
// For /31 and /32, special handling applies.
func calculateUsableIPs(subnet *types.Subnet) ([]string, error) {
	// Parse the CIDR
	_, network, err := net.ParseCIDR(subnet.NetworkAddress)
	if err != nil {
		return nil, fmt.Errorf("parsing CIDR: %w", err)
	}

	// Get gateway IP for exclusion
	var gatewayIP net.IP
	if subnet.GatewayAddress != nil && *subnet.GatewayAddress != "" {
		gatewayIP = net.ParseIP(*subnet.GatewayAddress)
	}

	maskSize, bits := network.Mask.Size()

	// Special cases
	if maskSize == 32 {
		// /32 - single host, can't have usable customer IPs (it IS the gateway usually)
		return []string{}, nil
	}
	if maskSize == 31 {
		// /31 - point-to-point link (RFC 3021)
		// Lower IP (.0) is gateway (ISP side)
		// Upper IP (.1) is customer (to be monitored)
		ip := network.IP.To4()
		if ip == nil {
			return nil, fmt.Errorf("only IPv4 supported")
		}

		// If no gateway is set, auto-compute: lower IP is gateway, upper IP is customer
		if gatewayIP == nil {
			// Set gateway to lower IP (network address for /31)
			gatewayIP = make(net.IP, 4)
			copy(gatewayIP, ip)
		}

		// Return only the upper IP (customer)
		customerIP := make(net.IP, 4)
		copy(customerIP, ip)
		customerIP[3] += 1

		// If the upper IP happens to be the gateway (unusual), return the lower
		if gatewayIP.Equal(customerIP) {
			return []string{ip.String()}, nil
		}

		return []string{customerIP.String()}, nil
	}

	// Standard subnet: exclude network, broadcast, and gateway
	// Calculate total hosts
	hostBits := bits - maskSize
	totalHosts := (1 << hostBits) - 2 // Exclude network and broadcast

	if totalHosts <= 0 {
		return []string{}, nil
	}

	// Convert network IP to uint32 for easy iteration
	ip := network.IP.To4()
	if ip == nil {
		return nil, fmt.Errorf("only IPv4 supported")
	}
	networkInt := binary.BigEndian.Uint32(ip)

	var ips []string
	// Start from first usable (network + 1) to last usable (broadcast - 1)
	for i := uint32(1); i <= uint32(totalHosts); i++ {
		hostInt := networkInt + i
		hostIP := make(net.IP, 4)
		binary.BigEndian.PutUint32(hostIP, hostInt)

		// Exclude gateway
		if gatewayIP != nil && hostIP.Equal(gatewayIP) {
			continue
		}

		ips = append(ips, hostIP.String())
	}

	return ips, nil
}

// GetSubnet retrieves a subnet by ID.
func (s *Service) GetSubnet(ctx context.Context, id string) (*types.Subnet, error) {
	return s.store.GetSubnet(ctx, id)
}

// GetSubnetByPilotID retrieves a subnet by its Pilot API ID.
func (s *Service) GetSubnetByPilotID(ctx context.Context, pilotID int) (*types.Subnet, error) {
	return s.store.GetSubnetByPilotID(ctx, pilotID)
}

// ListSubnets returns all active subnets.
func (s *Service) ListSubnets(ctx context.Context) ([]types.Subnet, error) {
	return s.store.ListSubnets(ctx)
}

// ListSubnetsIncludeArchived returns all subnets including archived ones.
func (s *Service) ListSubnetsIncludeArchived(ctx context.Context) ([]types.Subnet, error) {
	return s.store.ListSubnetsIncludeArchived(ctx)
}

// ListSubnetsPaginated returns subnets with pagination and filtering.
func (s *Service) ListSubnetsPaginated(ctx context.Context, params store.SubnetListParams) (*store.SubnetListResult, error) {
	return s.store.ListSubnetsPaginated(ctx, params)
}

// ListSubnetsBySubscriber returns subnets for a specific subscriber.
func (s *Service) ListSubnetsBySubscriber(ctx context.Context, subscriberID int) ([]types.Subnet, error) {
	return s.store.ListSubnetsBySubscriber(ctx, subscriberID)
}

// ListSubnetsByPOP returns subnets at a specific POP.
func (s *Service) ListSubnetsByPOP(ctx context.Context, popName string) ([]types.Subnet, error) {
	return s.store.ListSubnetsByPOP(ctx, popName)
}

// UpdateSubnetRequest contains parameters for updating a subnet.
type UpdateSubnetRequest struct {
	ID                 string
	PilotSubnetID      *int
	NetworkAddress     string
	NetworkSize        int
	GatewayAddress     *string
	FirstUsableAddress *string
	LastUsableAddress  *string
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

// UpdateSubnet updates a subnet's metadata.
func (s *Service) UpdateSubnet(ctx context.Context, req UpdateSubnetRequest) (*types.Subnet, error) {
	existing, err := s.store.GetSubnet(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("subnet not found: %s", req.ID)
	}

	// Update fields
	existing.PilotSubnetID = req.PilotSubnetID
	existing.NetworkAddress = req.NetworkAddress
	existing.NetworkSize = req.NetworkSize
	existing.GatewayAddress = req.GatewayAddress
	existing.FirstUsableAddress = req.FirstUsableAddress
	existing.LastUsableAddress = req.LastUsableAddress
	existing.VLANID = req.VLANID
	existing.ServiceID = req.ServiceID
	existing.SubscriberID = req.SubscriberID
	existing.SubscriberName = req.SubscriberName
	existing.LocationID = req.LocationID
	existing.LocationAddress = req.LocationAddress
	existing.City = req.City
	existing.Region = req.Region
	existing.POPName = req.POPName
	existing.GatewayDevice = req.GatewayDevice

	if err := existing.Validate(); err != nil {
		return nil, fmt.Errorf("invalid subnet: %w", err)
	}

	if err := s.store.UpdateSubnet(ctx, existing); err != nil {
		return nil, fmt.Errorf("updating subnet: %w", err)
	}

	s.logger.Info("subnet updated", "id", req.ID)
	return existing, nil
}

// ArchiveSubnet archives a subnet and handles its targets.
func (s *Service) ArchiveSubnet(ctx context.Context, id string, reason string) error {
	existing, err := s.store.GetSubnet(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("subnet not found: %s", id)
	}
	if existing.State == "archived" {
		return fmt.Errorf("subnet already archived: %s", id)
	}

	// Archive subnet and auto-owned targets
	if err := s.store.ArchiveSubnet(ctx, id, reason, true); err != nil {
		return fmt.Errorf("archiving subnet: %w", err)
	}

	s.logger.Info("subnet archived",
		"id", id,
		"network", existing.NetworkAddress,
		"reason", reason,
	)
	return nil
}

// GetSubnetTargetCounts returns target counts by monitoring state for a subnet.
func (s *Service) GetSubnetTargetCounts(ctx context.Context, subnetID string) (map[string]int, error) {
	return s.store.GetSubnetTargetCounts(ctx, subnetID)
}

// SubnetHasActiveCoverage checks if a subnet has any active customer targets.
func (s *Service) SubnetHasActiveCoverage(ctx context.Context, subnetID string) (bool, error) {
	return s.store.SubnetHasActiveCoverage(ctx, subnetID)
}

// =============================================================================
// TARGETS BY SUBNET
// =============================================================================

// ListTargetsBySubnet returns all active targets in a subnet.
func (s *Service) ListTargetsBySubnet(ctx context.Context, subnetID string) ([]types.Target, error) {
	return s.store.ListTargetsBySubnet(ctx, subnetID)
}

// ListTargetsNeedingReview returns targets that need human review.
func (s *Service) ListTargetsNeedingReview(ctx context.Context) ([]types.TargetEnriched, error) {
	return s.store.ListTargetsNeedingReview(ctx)
}

// GetTargetTagKeys returns distinct tag keys from all active targets.
func (s *Service) GetTargetTagKeys(ctx context.Context) ([]string, error) {
	return s.store.GetDistinctTargetTagKeys(ctx)
}

// =============================================================================
// TARGET STATE TRANSITIONS
// =============================================================================

// TransitionTargetState changes a target's monitoring state.
func (s *Service) TransitionTargetState(ctx context.Context, targetID string, newState types.MonitoringState, reason, triggeredBy string) error {
	if err := s.store.TransitionTargetState(ctx, targetID, newState, reason, triggeredBy); err != nil {
		return fmt.Errorf("transitioning target state: %w", err)
	}

	s.logger.Info("target state transitioned",
		"target_id", targetID,
		"new_state", newState,
		"reason", reason,
		"triggered_by", triggeredBy,
	)
	return nil
}

// GetTargetStateHistory returns recent state transitions for a target.
func (s *Service) GetTargetStateHistory(ctx context.Context, targetID string, limit int) ([]types.TargetStateTransition, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.GetTargetStateHistory(ctx, targetID, limit)
}

// AcknowledgeTargetRequest contains parameters for acknowledging a target.
type AcknowledgeTargetRequest struct {
	TargetID     string
	MarkInactive bool   // If true, transition to INACTIVE state
	Notes        string // Optional notes about the acknowledgment
	AcknowledgedBy string
}

// AcknowledgeTarget acknowledges a target in the review queue.
func (s *Service) AcknowledgeTarget(ctx context.Context, req AcknowledgeTargetRequest) error {
	triggeredBy := req.AcknowledgedBy
	if triggeredBy == "" {
		triggeredBy = "user"
	}

	if err := s.store.AcknowledgeTarget(ctx, req.TargetID, req.MarkInactive, triggeredBy); err != nil {
		return fmt.Errorf("acknowledging target: %w", err)
	}

	action := "cleared from review"
	if req.MarkInactive {
		action = "marked inactive"
	}
	s.logger.Info("target acknowledged",
		"target_id", req.TargetID,
		"action", action,
		"by", triggeredBy,
	)
	return nil
}

// =============================================================================
// ACTIVITY LOG
// =============================================================================

// ActivityFilter defines filters for activity log queries.
type ActivityFilter struct {
	TargetID string
	SubnetID string
	AgentID  string
	IP       string
	Category string
	Severity string
	Limit    int
}

// ListActivity returns activity log entries with optional filtering.
func (s *Service) ListActivity(ctx context.Context, filter ActivityFilter) ([]types.ActivityLogEntry, error) {
	return s.store.ListActivity(ctx, store.ActivityFilter{
		TargetID: filter.TargetID,
		SubnetID: filter.SubnetID,
		AgentID:  filter.AgentID,
		IP:       filter.IP,
		Category: filter.Category,
		Severity: filter.Severity,
		Limit:    filter.Limit,
	})
}

// GetTargetActivity returns recent activity for a specific target.
func (s *Service) GetTargetActivity(ctx context.Context, targetID string, limit int) ([]types.ActivityLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.GetRecentActivityForTarget(ctx, targetID, limit)
}

// GetSubnetActivity returns recent activity for a subnet and its targets.
func (s *Service) GetSubnetActivity(ctx context.Context, subnetID string, limit int) ([]types.ActivityLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.GetRecentActivityForSubnet(ctx, subnetID, limit)
}

// =============================================================================
// TARGET UPDATE/DELETE
// =============================================================================

// UpdateTargetRequest contains parameters for updating a target.
type UpdateTargetRequest struct {
	ID              string
	Tier            string
	Tags            map[string]string
	DisplayName     string
	Notes           string
	ExpectedOutcome *types.ExpectedOutcome
}

// UpdateTarget updates a target's metadata.
func (s *Service) UpdateTarget(ctx context.Context, req UpdateTargetRequest) (*types.Target, error) {
	existing, err := s.store.GetTarget(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("target not found: %s", req.ID)
	}

	// Update allowed fields
	if req.Tier != "" {
		existing.Tier = req.Tier
	}
	if req.Tags != nil {
		existing.Tags = req.Tags
	}
	if req.DisplayName != "" {
		existing.DisplayName = req.DisplayName
	}
	existing.Notes = req.Notes
	existing.ExpectedOutcome = req.ExpectedOutcome

	if err := s.store.UpdateTarget(ctx, existing); err != nil {
		return nil, fmt.Errorf("updating target: %w", err)
	}

	s.logger.Info("target updated", "id", req.ID)
	return existing, nil
}

// DeleteTarget archives a target (soft delete).
func (s *Service) DeleteTarget(ctx context.Context, id string, reason string) error {
	existing, err := s.store.GetTarget(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("target not found: %s", id)
	}
	if existing.ArchivedAt != nil {
		return fmt.Errorf("target already archived: %s", id)
	}

	if reason == "" {
		reason = "deleted via API"
	}

	if err := s.store.ArchiveTarget(ctx, id, reason); err != nil {
		return fmt.Errorf("archiving target: %w", err)
	}

	s.logger.Info("target deleted (archived)",
		"id", id,
		"ip", existing.IP,
		"reason", reason,
	)
	return nil
}
