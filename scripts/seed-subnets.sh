#!/bin/bash
# Seed test subnets for development
#
# Usage: ./scripts/seed-subnets.sh

set -e

API_URL="${API_URL:-http://localhost:8081}"

echo "Seeding subnets to $API_URL..."

# Function to create a subnet
create_subnet() {
  local pilot_id="$1"
  local network="$2"
  local size="$3"
  local gateway="$4"
  local first="$5"
  local last="$6"
  local vlan="$7"
  local service="$8"
  local subscriber_id="$9"
  local subscriber_name="${10}"
  local location_id="${11}"
  local address="${12}"
  local city="${13}"
  local region="${14}"
  local pop="${15}"
  local device="${16}"

  curl -s -X POST "$API_URL/api/v1/subnets" \
    -H "Content-Type: application/json" \
    -d "{
      \"pilot_subnet_id\": $pilot_id,
      \"network_address\": \"$network\",
      \"network_size\": $size,
      \"gateway_address\": \"$gateway\",
      \"first_usable_address\": \"$first\",
      \"last_usable_address\": \"$last\",
      \"vlan_id\": $vlan,
      \"service_id\": $service,
      \"subscriber_id\": $subscriber_id,
      \"subscriber_name\": \"$subscriber_name\",
      \"location_id\": $location_id,
      \"location_address\": \"$address\",
      \"city\": \"$city\",
      \"region\": \"$region\",
      \"pop_name\": \"$pop\",
      \"gateway_device\": \"$device\"
    }" > /dev/null

  echo "  Created: $network - $subscriber_name @ $pop"
}

# NYC POP - Multiple customers
echo ""
echo "NYC1 POP Subnets:"
create_subnet 10001 "10.100.10.0/24" 24 "10.100.10.1" "10.100.10.2" "10.100.10.254" 110 2001 5001 "TechStart LLC" 101 "100 Broadway, New York, NY" "New York" "us-east" "NYC1" "csw-nyc-01"
create_subnet 10002 "10.100.11.0/24" 24 "10.100.11.1" "10.100.11.2" "10.100.11.254" 111 2002 5002 "DataFlow Inc" 102 "200 Wall St, New York, NY" "New York" "us-east" "NYC1" "csw-nyc-01"
create_subnet 10003 "10.100.12.0/28" 28 "10.100.12.1" "10.100.12.2" "10.100.12.14" 112 2003 5003 "CloudBase Corp" 103 "300 Park Ave, New York, NY" "New York" "us-east" "NYC1" "csw-nyc-01"
create_subnet 10004 "10.100.13.0/29" 29 "10.100.13.1" "10.100.13.2" "10.100.13.6" 113 2004 5004 "SmallBiz Solutions" 104 "50 Main St, Newark, NJ" "Newark" "us-east" "NYC1" "csw-nyc-02"

# LAX POP
echo ""
echo "LAX1 POP Subnets:"
create_subnet 10011 "10.200.10.0/24" 24 "10.200.10.1" "10.200.10.2" "10.200.10.254" 210 3001 6001 "West Coast Media" 201 "1000 Hollywood Blvd, Los Angeles, CA" "Los Angeles" "us-west" "LAX1" "csw-lax-01"
create_subnet 10012 "10.200.11.0/24" 24 "10.200.11.1" "10.200.11.2" "10.200.11.254" 211 3002 6002 "SoCal Enterprises" 202 "500 Sunset Blvd, Los Angeles, CA" "Los Angeles" "us-west" "LAX1" "csw-lax-01"
create_subnet 10013 "10.200.12.0/27" 27 "10.200.12.1" "10.200.12.2" "10.200.12.30" 212 3003 6003 "Bay Area Startups" 203 "100 Sand Hill Rd, Palo Alto, CA" "Palo Alto" "us-west" "LAX1" "csw-lax-02"

# Chicago POP
echo ""
echo "CHI1 POP Subnets:"
create_subnet 10021 "10.150.10.0/24" 24 "10.150.10.1" "10.150.10.2" "10.150.10.254" 310 4001 7001 "Midwest Manufacturing" 301 "200 Michigan Ave, Chicago, IL" "Chicago" "us-central" "CHI1" "csw-chi-01"
create_subnet 10022 "10.150.11.0/25" 25 "10.150.11.1" "10.150.11.2" "10.150.11.126" 311 4002 7002 "Great Lakes Logistics" 302 "400 Lake Shore Dr, Chicago, IL" "Chicago" "us-central" "CHI1" "csw-chi-01"

# Dallas POP
echo ""
echo "DFW1 POP Subnets:"
create_subnet 10031 "10.180.10.0/24" 24 "10.180.10.1" "10.180.10.2" "10.180.10.254" 410 5001 8001 "Texas Energy Corp" 401 "300 Main St, Dallas, TX" "Dallas" "us-south" "DFW1" "csw-dfw-01"
create_subnet 10032 "10.180.11.0/26" 26 "10.180.11.1" "10.180.11.2" "10.180.11.62" 411 5002 8002 "Southern Healthcare" 402 "100 Medical Center Dr, Houston, TX" "Houston" "us-south" "DFW1" "csw-dfw-01"

echo ""
echo "Done! Created 11 subnets across 4 POPs."
echo ""
echo "View subnets: curl -s $API_URL/api/v1/subnets | jq ."
