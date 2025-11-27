// Package pilot provides a client for the Flight Deck Network Resource API.
package pilot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// Config holds configuration for the Flight Deck API client.
type Config struct {
	BaseURL    string        // Base URL (e.g., "https://pilotfiber.com/api/v1/network/resources")
	AuthToken  string        // Bearer token for authentication
	Timeout    time.Duration // HTTP timeout (default: 30s)
	RateLimit  int           // Requests per minute (default: 60)
	MaxSubnets int           // Maximum subnets to fetch (0 = unlimited, for testing)
}

// Client is a Flight Deck Network Resource API client.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	authToken   string
	rateLimiter *rate.Limiter
	maxSubnets  int
	logger      *slog.Logger
}

// NewClient creates a new Flight Deck API client.
func NewClient(cfg Config, logger *slog.Logger) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	rateLimit := cfg.RateLimit
	if rateLimit == 0 {
		rateLimit = 60 // 60 requests per minute = 1 per second
	}

	return &Client{
		baseURL: cfg.BaseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		authToken:   cfg.AuthToken,
		rateLimiter: rate.NewLimiter(rate.Limit(float64(rateLimit)/60.0), 1),
		maxSubnets:  cfg.MaxSubnets,
		logger:      logger.With("component", "pilot_client"),
	}
}

// APIResponse represents a generic API response.
// The API returns either {"data": [...]} on success or {"error": "..."} on error.
type APIResponse struct {
	Data  []map[string]any `json:"data"`
	Error string           `json:"error,omitempty"`
}

// query makes a GET request to the API.
func (c *Client) query(ctx context.Context, table string, filters map[string]string) ([]map[string]any, error) {
	// Rate limit
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	// Build URL
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}

	q := u.Query()
	q.Set("table", table)
	for k, v := range filters {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	c.logger.Info("API request", "url", u.String(), "table", table)

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.authToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Error != "" {
		return nil, fmt.Errorf("API returned error: %s", apiResp.Error)
	}

	c.logger.Debug("API response", "table", table, "count", len(apiResp.Data))
	return apiResp.Data, nil
}

// ListIPPools fetches subnets from Flight Deck and enriches them with metadata.
func (c *Client) ListIPPools(ctx context.Context) ([]IPPool, error) {
	start := time.Now()

	// Step 1: Fetch subnets (with optional limit for testing)
	subnets, err := c.fetchSubnets(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch subnets: %w", err)
	}

	if len(subnets) == 0 {
		c.logger.Info("no subnets found")
		return nil, nil
	}

	// Step 2: Collect unique VLAN IDs
	vlanIDs := uniqueInts(subnets, "vlan_id")
	c.logger.Debug("unique VLANs", "count", len(vlanIDs))

	// Step 3: Batch fetch VLANs (includes csw_id)
	vlans, err := c.batchFetch(ctx, "vlan", vlanIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch vlans: %w", err)
	}
	vlanMap := indexByID(vlans)

	// Step 4: Collect unique CSW IDs from VLANs
	cswIDs := uniqueInts(vlans, "csw_id")
	c.logger.Debug("unique CSWs", "count", len(cswIDs))

	// Step 5: Batch fetch CSWs (gateway devices)
	var cswMap map[int]map[string]any
	if len(cswIDs) > 0 {
		csws, err := c.batchFetch(ctx, "csw", cswIDs)
		if err != nil {
			c.logger.Warn("failed to fetch CSWs, continuing without gateway info", "error", err)
		} else {
			cswMap = indexByID(csws)
		}
	}

	// Step 6: Get VLAN-to-service mappings
	vlanServices, err := c.fetchVLANServices(ctx, vlanIDs)
	if err != nil {
		c.logger.Warn("failed to fetch vlan_services, continuing without service info", "error", err)
		vlanServices = nil
	}

	// Step 7: Collect unique service IDs
	serviceIDs := uniqueInts(vlanServices, "service_id")
	c.logger.Debug("unique services", "count", len(serviceIDs))

	// Step 8: Batch fetch services
	var serviceMap map[int]map[string]any
	if len(serviceIDs) > 0 {
		services, err := c.batchFetch(ctx, "service", serviceIDs)
		if err != nil {
			c.logger.Warn("failed to fetch services, continuing without subscriber info", "error", err)
		} else {
			serviceMap = indexByID(services)
		}
	}

	// Step 9: Collect subscriber and location IDs
	subscriberIDs := uniqueInts(mapValues(serviceMap), "subscriber_id")
	locationIDs := uniqueInts(mapValues(serviceMap), "location_id")
	c.logger.Debug("unique subscribers/locations", "subscribers", len(subscriberIDs), "locations", len(locationIDs))

	// Step 10: Fetch subscribers
	var subscriberMap map[int]map[string]any
	if len(subscriberIDs) > 0 {
		subscribers, err := c.batchFetch(ctx, "subscriber", subscriberIDs)
		if err != nil {
			c.logger.Warn("failed to fetch subscribers", "error", err)
		} else {
			subscriberMap = indexByID(subscribers)
		}
	}

	// Step 11: Fetch locations
	var locationMap map[int]map[string]any
	if len(locationIDs) > 0 {
		locations, err := c.batchFetch(ctx, "location", locationIDs)
		if err != nil {
			c.logger.Warn("failed to fetch locations", "error", err)
		} else {
			locationMap = indexByID(locations)
		}
	}

	// Step 12: Build enriched IP pools
	pools := c.buildIPPools(subnets, vlanMap, cswMap, vlanServices, serviceMap, subscriberMap, locationMap)

	c.logger.Info("ListIPPools complete",
		"duration", time.Since(start),
		"subnets", len(subnets),
		"enriched_pools", len(pools),
	)

	return pools, nil
}

// fetchSubnets fetches all subnets from the API.
// The Flight Deck API returns all records in a single response (no pagination).
func (c *Client) fetchSubnets(ctx context.Context) ([]map[string]any, error) {
	all, err := c.query(ctx, "subnet_v4", nil)
	if err != nil {
		return nil, err
	}

	// Apply limit if set (for testing)
	if c.maxSubnets > 0 && len(all) > c.maxSubnets {
		all = all[:c.maxSubnets]
		c.logger.Info("subnet limit applied", "limit", c.maxSubnets, "total", len(all))
	}

	c.logger.Info("fetched subnets", "count", len(all))
	return all, nil
}

// batchFetch fetches records by ID in batches of 100.
func (c *Client) batchFetch(ctx context.Context, table string, ids []int) ([]map[string]any, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	const batchSize = 100
	var results []map[string]any

	for i := 0; i < len(ids); i += batchSize {
		end := i + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]

		// Build comma-separated ID list
		idStr := intsToCSV(batch)

		records, err := c.query(ctx, table, map[string]string{
			"id": idStr,
		})
		if err != nil {
			return nil, fmt.Errorf("batch fetch %s: %w", table, err)
		}
		results = append(results, records...)
	}

	return results, nil
}

// fetchVLANServices fetches VLAN-to-service mappings for the given VLAN IDs.
func (c *Client) fetchVLANServices(ctx context.Context, vlanIDs []int) ([]map[string]any, error) {
	if len(vlanIDs) == 0 {
		return nil, nil
	}

	// Fetch vlan_service records in batches by vlan_id
	const batchSize = 100
	var results []map[string]any

	for i := 0; i < len(vlanIDs); i += batchSize {
		end := i + batchSize
		if end > len(vlanIDs) {
			end = len(vlanIDs)
		}
		batch := vlanIDs[i:end]

		idStr := intsToCSV(batch)

		records, err := c.query(ctx, "vlan_service", map[string]string{
			"vlan_id": idStr,
		})
		if err != nil {
			return nil, fmt.Errorf("fetch vlan_service: %w", err)
		}
		results = append(results, records...)
	}

	return results, nil
}

// buildIPPools builds enriched IP pools from the fetched data.
func (c *Client) buildIPPools(
	subnets []map[string]any,
	vlans map[int]map[string]any,
	csws map[int]map[string]any,
	vlanServices []map[string]any,
	services map[int]map[string]any,
	subscribers map[int]map[string]any,
	locations map[int]map[string]any,
) []IPPool {
	// Build VLAN → Service mapping
	vlanToService := make(map[int]int)
	for _, vs := range vlanServices {
		if vlanID, ok := getInt(vs, "vlan_id"); ok {
			if serviceID, ok := getInt(vs, "service_id"); ok {
				vlanToService[vlanID] = serviceID
			}
		}
	}

	var pools []IPPool
	for _, subnet := range subnets {
		pool := IPPool{}

		// Basic subnet info
		if id, ok := getInt(subnet, "id"); ok {
			pool.ID = id
		}
		// Combine network_address and network_size into CIDR notation
		if addr, ok := subnet["network_address"].(string); ok {
			if size, ok := getInt(subnet, "network_size"); ok {
				pool.NetworkAddress = addr + "/" + strconv.Itoa(size)
				pool.NetworkSize = size
			} else {
				pool.NetworkAddress = addr
			}
		}
		if name, ok := subnet["name"].(string); ok {
			pool.Name = &name
		}

		// Subnet type (0=NA, 1=WAN, 2=LAN)
		if subnetType, ok := getInt(subnet, "type"); ok {
			pool.SubnetType = &subnetType
			switch subnetType {
			case 1:
				pool.SubnetTypeName = strPtr("WAN")
			case 2:
				pool.SubnetTypeName = strPtr("LAN")
			default:
				pool.SubnetTypeName = strPtr("NA")
			}
		}

		// Get VLAN and CSW (gateway device)
		// Note: subnet.vlan_id is the database ID, we need to look up the real VLAN tag (vid)
		if vlanDBID, ok := getInt(subnet, "vlan_id"); ok {
			// Get CSW hostname and real VLAN ID from VLAN table
			if vlan, ok := vlans[vlanDBID]; ok {
				// Get the real VLAN tag (vid), not the database ID
				if vid, ok := getInt(vlan, "vid"); ok {
					pool.VLANID = &vid
				}
				if cswID, ok := getInt(vlan, "csw_id"); ok {
					if csw, ok := csws[cswID]; ok {
						if hostname, ok := csw["hostname"].(string); ok {
							pool.GatewayDevice = &hostname
						}
					}
				}
			}

			// Get service via vlan_service (using database ID)
			if serviceID, ok := vlanToService[vlanDBID]; ok {
				pool.ServiceID = &serviceID

				// Get service details
				if svc, ok := services[serviceID]; ok {
					// Get subscriber
					if subID, ok := getInt(svc, "subscriber_id"); ok {
						pool.SubscriberID = &subID
						if sub, ok := subscribers[subID]; ok {
							if name, ok := sub["friendlyname"].(string); ok {
								pool.SubscriberName = &name
							}
						}
					}

					// Get location
					if locID, ok := getInt(svc, "location_id"); ok {
						pool.LocationID = &locID
						if loc, ok := locations[locID]; ok {
							// Build full address: "155 E 44th St, New York, NY 10017"
							streetNum, _ := loc["street_number"].(string)
							route, _ := loc["route"].(string)
							city, _ := loc["city"].(string)
							state, _ := loc["state"].(string)
							zip, _ := loc["zip"].(string)

							var addrParts []string
							// Street part: "155 E 44th St"
							street := streetNum
							if route != "" {
								if street != "" {
									street += " "
								}
								street += route
							}
							if street != "" {
								addrParts = append(addrParts, street)
							}
							// City, State ZIP: "New York, NY 10017"
							cityStateZip := city
							if state != "" {
								if cityStateZip != "" {
									cityStateZip += ", "
								}
								cityStateZip += state
							}
							if zip != "" {
								if cityStateZip != "" {
									cityStateZip += " "
								}
								cityStateZip += zip
							}
							if cityStateZip != "" {
								addrParts = append(addrParts, cityStateZip)
							}

							if len(addrParts) > 0 {
								fullAddr := strings.Join(addrParts, ", ")
								pool.LocationAddress = &fullAddr
							}

							// City field: "New York, NY"
							if city != "" {
								cityState := city
								if state != "" {
									cityState += ", " + state
								}
								pool.City = &cityState
							}

							// Region field: lookup from region_id
							if regionID, ok := getInt(loc, "region_id"); ok {
								if regionName, ok := regionNames[regionID]; ok {
									pool.Region = &regionName
								}
							}
						}
					}
				}
			}
		}

		// Compute derived fields
		pool.ComputeAddresses()
		pool.ExtractPOPName()

		pools = append(pools, pool)
	}

	return pools
}

// Helper functions

func getInt(m map[string]any, key string) (int, bool) {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return int(val), true
		case int:
			return val, true
		case int64:
			return int(val), true
		}
	}
	return 0, false
}

func uniqueInts(records []map[string]any, key string) []int {
	seen := make(map[int]bool)
	var result []int
	for _, r := range records {
		if id, ok := getInt(r, key); ok && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

func intsToCSV(ids []int) string {
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = strconv.Itoa(id)
	}
	return strings.Join(strs, ",")
}

func indexByID(records []map[string]any) map[int]map[string]any {
	result := make(map[int]map[string]any)
	for _, r := range records {
		if id, ok := getInt(r, "id"); ok {
			result[id] = r
		}
	}
	return result
}

func mapValues(m map[int]map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(m))
	for _, v := range m {
		result = append(result, v)
	}
	return result
}

func strPtr(s string) *string {
	return &s
}

// regionNames maps region_id to human-readable region name.
var regionNames = map[int]string{
	1:  "New York City",
	2:  "Philadelphia",
	4:  "Washington DC",
	5:  "Boston",
	6:  "Chicago",
	7:  "Seattle",
	8:  "San Francisco",
	9:  "San Jose",
	10: "Los Angeles",
	11: "Dallas",
	12: "Atlanta",
	13: "Phoenix",
	14: "New Jersey",
}
