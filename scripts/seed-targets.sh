#!/bin/bash
# Seed script to add multiple targets for testing multi-agent scenarios
# Usage: ./scripts/seed-targets.sh

API_URL="${ICMPMON_API_URL:-http://localhost:8081}"

echo "Seeding targets to $API_URL..."

# DNS Providers (Infrastructure tier - critical)
echo "Adding infrastructure targets..."
curl -s -X POST "$API_URL/api/v1/targets" -H "Content-Type: application/json" -d '{
  "ip": "8.8.8.8",
  "tier": "infrastructure",
  "tags": {"device_type": "dns", "provider": "google", "service": "public-dns"}
}' | jq -r '.id // .error'

curl -s -X POST "$API_URL/api/v1/targets" -H "Content-Type: application/json" -d '{
  "ip": "8.8.4.4",
  "tier": "infrastructure",
  "tags": {"device_type": "dns", "provider": "google", "service": "public-dns"}
}' | jq -r '.id // .error'

curl -s -X POST "$API_URL/api/v1/targets" -H "Content-Type: application/json" -d '{
  "ip": "1.1.1.1",
  "tier": "infrastructure",
  "tags": {"device_type": "dns", "provider": "cloudflare", "service": "public-dns"}
}' | jq -r '.id // .error'

curl -s -X POST "$API_URL/api/v1/targets" -H "Content-Type: application/json" -d '{
  "ip": "1.0.0.1",
  "tier": "infrastructure",
  "tags": {"device_type": "dns", "provider": "cloudflare", "service": "public-dns"}
}' | jq -r '.id // .error'

curl -s -X POST "$API_URL/api/v1/targets" -H "Content-Type: application/json" -d '{
  "ip": "9.9.9.9",
  "tier": "infrastructure",
  "tags": {"device_type": "dns", "provider": "quad9", "service": "public-dns"}
}' | jq -r '.id // .error'

# Major CDN/Cloud endpoints (VIP tier)
echo "Adding VIP targets..."
curl -s -X POST "$API_URL/api/v1/targets" -H "Content-Type: application/json" -d '{
  "ip": "151.101.1.140",
  "tier": "vip",
  "subscriber_id": "sub-reddit",
  "tags": {"subscriber_name": "Reddit", "service_type": "cdn", "pop": "fastly"}
}' | jq -r '.id // .error'

curl -s -X POST "$API_URL/api/v1/targets" -H "Content-Type: application/json" -d '{
  "ip": "104.16.132.229",
  "tier": "vip",
  "subscriber_id": "sub-cloudflare",
  "tags": {"subscriber_name": "Cloudflare", "service_type": "cdn"}
}' | jq -r '.id // .error'

curl -s -X POST "$API_URL/api/v1/targets" -H "Content-Type: application/json" -d '{
  "ip": "52.94.236.248",
  "tier": "vip",
  "subscriber_id": "sub-aws",
  "tags": {"subscriber_name": "AWS", "service_type": "cloud", "region": "us-east-1"}
}' | jq -r '.id // .error'

curl -s -X POST "$API_URL/api/v1/targets" -H "Content-Type: application/json" -d '{
  "ip": "142.250.80.46",
  "tier": "vip",
  "subscriber_id": "sub-google",
  "tags": {"subscriber_name": "Google", "service_type": "cloud"}
}' | jq -r '.id // .error'

# Standard subscriber targets
echo "Adding standard targets..."
curl -s -X POST "$API_URL/api/v1/targets" -H "Content-Type: application/json" -d '{
  "ip": "208.67.222.222",
  "tier": "standard",
  "subscriber_id": "sub-opendns",
  "tags": {"subscriber_name": "OpenDNS", "service_type": "dns"}
}' | jq -r '.id // .error'

curl -s -X POST "$API_URL/api/v1/targets" -H "Content-Type: application/json" -d '{
  "ip": "208.67.220.220",
  "tier": "standard",
  "subscriber_id": "sub-opendns",
  "tags": {"subscriber_name": "OpenDNS", "service_type": "dns"}
}' | jq -r '.id // .error'

curl -s -X POST "$API_URL/api/v1/targets" -H "Content-Type: application/json" -d '{
  "ip": "199.85.126.10",
  "tier": "standard",
  "subscriber_id": "sub-norton",
  "tags": {"subscriber_name": "Norton DNS", "service_type": "dns"}
}' | jq -r '.id // .error'

curl -s -X POST "$API_URL/api/v1/targets" -H "Content-Type: application/json" -d '{
  "ip": "185.228.168.9",
  "tier": "standard",
  "subscriber_id": "sub-cleanbrowsing",
  "tags": {"subscriber_name": "CleanBrowsing", "service_type": "dns"}
}' | jq -r '.id // .error'

curl -s -X POST "$API_URL/api/v1/targets" -H "Content-Type: application/json" -d '{
  "ip": "76.76.19.19",
  "tier": "standard",
  "subscriber_id": "sub-alternate",
  "tags": {"subscriber_name": "Alternate DNS", "service_type": "dns"}
}' | jq -r '.id // .error'

curl -s -X POST "$API_URL/api/v1/targets" -H "Content-Type: application/json" -d '{
  "ip": "94.140.14.14",
  "tier": "standard",
  "subscriber_id": "sub-adguard",
  "tags": {"subscriber_name": "AdGuard DNS", "service_type": "dns"}
}' | jq -r '.id // .error'

echo ""
echo "Done! Checking target count..."
curl -s "$API_URL/api/v1/targets" | jq '.count'
