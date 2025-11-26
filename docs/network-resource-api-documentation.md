# Network Resource API Documentation

## Table of Contents

1. [Overview](#overview)  
2. [Authentication](#authentication)  
3. [API Endpoints](#api-endpoints)  
4. [Resource Types](#resource-types)  
5. [Query Operations](#query-operations)  
6. [Create Operations](#create-operations)  
7. [Update Operations](#update-operations)  
8. [Delete Operations](#delete-operations)  
9. [Advanced Filtering](#advanced-filtering)  
10. [Business Logic & Validation](#business-logic--validation)  
11. [Error Handling](#error-handling)  
12. [Audit Logging](#audit-logging)  
13. [Common Use Cases](#common-use-cases)

---

## Overview

The Network Resource API is a unified, table-driven REST API that provides CRUD operations for network infrastructure management. The API supports 24+ resource types including subnets, VLANs, network devices, BGP sessions, ONT/OLT equipment, and more.

**Base URL:** `/api/v1/network/resources`

**Key Features:**

- Unified endpoint for all network resources  
- Advanced query filtering with operators  
- Role-based access control  
- Comprehensive audit logging  
- Transaction-wrapped operations  
- Binary IP address storage for efficiency  
- Network conflict detection

**Architecture:**

- **Routes:** `pilot-main-laravel/app/Pilot/Http/routes_api.php:306-318`  
- **Controller:** `pilot-main-laravel/app/Pilot/Http/Controllers/Api/ApiController.php`  
- **Model Resolver:** `pilot-main-laravel/app/Pilot/Utils/ApiModel/ModelResolver.php`  
- **Models:** `pilot-main-laravel/app/Pilot/Models/`

---

## Authentication

All API endpoints require bearer token authentication.

**Header:**

```
Authorization: Bearer YOUR_API_TOKEN
```

**Token Configuration:**

- Environment variable: `NETWORK_API_AUTH_KEY`  
- Authenticated as: `FlightEngineer` operator  
- Role permissions: Defined in `app/config/acl.php`

**Example Request:**

```shell
curl -X GET "https://your-domain.com/api/v1/network/resources?table=subnet_v4" \
  -H "Authorization: Bearer YOUR_API_TOKEN"
```

---

## API Endpoints

### Fetch Resources (GET)

**Endpoint:** `GET /api/v1/network/resources`

Retrieve resources with optional filtering.

**Required Parameters:**

- `table` \- Resource type to fetch

**Optional Parameters:**

- Field filters (e.g., `vlan_id=123`)  
- Operator suffixes (e.g., `id_gt=100`)  
- Multiple values (e.g., `id=1,2,3`)  
- Pagination parameters

---

### Create Resources (POST)

**Endpoint:** `POST /api/v1/network/resources`

Create new resources.

**Required Parameters:**

- `table` \- Resource type  
- Resource-specific required fields (see ACL config)

**Optional Parameters:**

- `task_id` \- For audit tracking  
- `staff` \- Staff identifier

---

### Update Resources (PATCH)

**Endpoint:** `PATCH /api/v1/network/resources`

Update existing resources.

**Required Parameters:**

- `table` \- Resource type  
- Filter to identify resources (e.g., `id=123`)  
- Fields to update

**Optional Parameters:**

- `task_id` \- For audit tracking  
- `staff` \- Staff identifier

---

### Delete Resources (DELETE)

**Endpoint:** `DELETE /api/v1/network/resources`

Delete resources.

**Required Parameters:**

- `table` \- Resource type  
- Filter to identify resources (e.g., `id=123`)

**Optional Parameters:**

- `task_id` \- For audit tracking  
- `staff` \- Staff identifier

---

## Resource Types

### Network Infrastructure

#### SubnetV4 (`subnet_v4`)

IPv4 subnet definitions.

**Key Fields:**

- `id` \- Primary key  
- `vlan_id` \- Foreign key to VLAN  
- `ont_id` \- Foreign key to ONT (optional)  
- `type` \- Subnet type (0=NA, 1=WAN, 2=LAN)  
- `kea_subnet_id` \- KEA DHCP subnet ID  
- `name` \- Subnet name  
- `ipblock_id` \- Parent IP block  
- `network_address` \- Network address (e.g., "192.168.1.0")  
- `network_size` \- Prefix length (24-32)  
- `next_hop` \- Gateway address  
- `eero_network_id` \- Eero integration (optional)  
- `eero_network_name` \- Eero network name (optional)  
- `eero_network_password` \- Eero password (optional)

**Business Rules:**

- Network address must be valid for prefix length  
- Must fit within parent IPBlock  
- Cannot conflict with existing subnets  
- Automatically calculates first/last addresses

**Relationships:**

- Belongs to VLAN  
- Belongs to IPBlock  
- May belong to ONT  
- May belong to Eero Network

---

#### SubnetV6 (`subnet_v6`)

IPv6 subnet definitions.

**Key Fields:**

- Similar to SubnetV4 but for IPv6  
- `network_size` \- Prefix length for IPv6

---

#### IPBlockV4 (`ipblock_v4`)

IPv4 address block allocation.

**Key Fields:**

- `id` \- Primary key  
- `region_id` \- Foreign key to region  
- `dhcp` \- DHCP enabled flag  
- `network_address` \- Block address  
- `network_size` \- Prefix length (16-25)  
- `first_address` \- Binary first address  
- `last_address` \- Binary last address  
- `pct_utilization` \- Utilization percentage

**Business Rules:**

- Validates network address format  
- Checks for conflicts with other blocks  
- Prevents deletion if subnets exist  
- Size must be 16-25

**Relationships:**

- Has many SubnetV4  
- Belongs to Region

---

#### IPBlockV6 (`ipblock_v6`)

IPv6 address block allocation.

**Key Fields:**

- Similar to IPBlockV4 but for IPv6

---

#### VLAN (`vlan`)

Virtual LAN configurations.

**Key Fields:**

- `id` \- Primary key  
- `vid` \- VLAN ID (0-4095)  
- `csw_id` \- Foreign key to CSW  
- `type` \- VLAN type (0=NA, 1=ACCESS, 2=MANAGEMENT)  
- `ports` \- Pipe-delimited port list  
- `name` \- VLAN name  
- `description` \- Description

**Business Rules:**

- VID must be between 0-4095  
- Must belong to a CSW

**Relationships:**

- Has many SubnetV4  
- Has many SubnetV6  
- Has many VLANService  
- Belongs to CSW

---

#### CSW (`csw`)

Core switch inventory.

**Key Fields:**

- `id` \- Primary key  
- `hostname` \- Switch hostname  
- `pop_id` \- Foreign key to POP  
- Additional switch configuration fields

**Relationships:**

- Has many VLANs  
- Belongs to POP

---

#### POP (`pop`)

Point of presence locations.

**Key Fields:**

- `id` \- Primary key  
- `name` \- POP name  
- Location and configuration fields

**Relationships:**

- Has many CSW  
- Has many NetworkDevice

---

#### NetworkDevice (`network_device`)

Network device inventory (switches, routers, etc.).

**Key Fields:**

- `id` \- Primary key  
- `hostname` \- Device hostname  
- `pop_id` \- Foreign key to POP  
- `type` \- Device type (0=NA, 1=SWITCH)

**Relationships:**

- Has many NetworkDeviceService  
- Belongs to POP

---

#### NetworkDeviceService (`network_device_service`)

Maps network devices to services.

**Key Fields:**

- `id` \- Primary key  
- `network_device_id` \- Foreign key  
- `service_id` \- Foreign key

---

#### VLANService (`vlan_service`)

Maps VLANs to services.

**Key Fields:**

- `id` \- Primary key  
- `vlan_id` \- Foreign key  
- `service_id` \- Foreign key

---

### BGP Resources

#### BGPSession (`bgp_session`)

BGP peering session configurations.

**Key Fields:**

- `id` \- Primary key  
- `service_id` \- Foreign key to service  
- `vlan_id` \- Foreign key to VLAN  
- `csw_id` \- Foreign key to CSW  
- `interface_name` \- Interface name  
- `local_asn` \- Local AS number  
- `remote_asn` \- Remote AS number  
- `ip_version` \- IP version (4 or 6\)  
- `ingress_filter_id` \- Ingress route filter  
- `egress_filter_id` \- Egress route filter  
- `local_network_address` \- Local BGP peer address  
- `remote_network_address` \- Remote BGP peer address  
- `subnet_v4_id` \- Associated IPv4 subnet  
- `subnet_v6_id` \- Associated IPv6 subnet  
- `md5_key` \- BGP MD5 authentication key

**Business Rules:**

- Ingress and egress filters must be different  
- IP addresses must be within subnet  
- VLAN must belong to CSW  
- Subnet must belong to VLAN  
- Filters must belong to subscriber

**Relationships:**

- Belongs to Service  
- Belongs to VLAN  
- Belongs to CSW  
- Belongs to SubnetV4 or SubnetV6  
- Has ingress BGPSessionFilter  
- Has egress BGPSessionFilter

---

#### BGPSessionFilter (`bgp_session_filter`)

BGP route filters.

**Key Fields:**

- `id` \- Primary key  
- `subscriber_id` \- Foreign key  
- `name` \- Filter name  
- Filter configuration fields

---

#### BGPSessionManualPrefix (`bgp_session_manual_prefix`)

Manual BGP prefix configurations.

**Key Fields:**

- `id` \- Primary key  
- `bgp_session_filter_id` \- Foreign key  
- Prefix configuration fields

---

#### BGPSessionIRRConfig (`bgp_session_irr_config`)

Internet Routing Registry configurations for BGP.

**Key Fields:**

- `id` \- Primary key  
- `bgp_session_filter_id` \- Foreign key  
- IRR configuration fields

---

### ONT/OLT Resources

#### OLT (`olt`)

Optical Line Terminal equipment.

**Key Fields:**

- `id` \- Primary key  
- OLT configuration and management fields

---

#### ONTModel (`ont_model`)

ONT device model definitions.

**Key Fields:**

- `id` \- Primary key  
- `manufacturer` \- Device manufacturer  
- `model` \- Model identifier  
- Capabilities and specifications

---

#### InventoryONT (`inventory_ont`)

ONT inventory tracking.

**Key Fields:**

- `id` \- Primary key  
- `serial_number` \- ONT serial number  
- `ont_model_id` \- Foreign key to ONTModel  
- Inventory tracking fields

---

#### ObservedONTState (`observed_ont_state`)

Current state observations for ONTs.

**Key Fields:**

- `id` \- Primary key  
- State tracking fields  
- Timestamp fields

---

#### ObservedONTStateDetail (`observed_ont_state_detail`)

Detailed ONT state information.

**Key Fields:**

- `id` \- Primary key  
- `observed_ont_state_id` \- Foreign key  
- Detailed state fields

---

#### ObservedONTPortState (`observed_ont_port_state`)

ONT port state tracking.

**Key Fields:**

- `id` \- Primary key  
- Port state and status fields

---

### Eero Mesh Network Resources

#### EeroNetwork (`eero_network`)

Eero mesh network configurations.

**Key Fields:**

- `id` \- Primary key  
- Network configuration fields  
- Integration fields

---

#### EeroDevice (`eero_device`)

Individual Eero device inventory.

**Key Fields:**

- `id` \- Primary key  
- `eero_network_id` \- Foreign key  
- Device identification fields

---

### CPE Resources

#### CPE (`cpe`)

Customer premises equipment.

**Key Fields:**

- `id` \- Primary key  
- `cpe_model_id` \- Foreign key  
- `service_id` \- Foreign key  
- Device identification and management fields

---

#### CPEModel (`cpe_model`)

CPE model definitions.

**Key Fields:**

- `id` \- Primary key  
- `manufacturer` \- Manufacturer  
- `model` \- Model identifier  
- Specifications

---

### Network Task Resources

#### NetworkTask (`network_task`)

Network provisioning and maintenance tasks.

**Key Fields:**

- `id` \- Primary key  
- Task definition fields  
- Status tracking

---

#### NetworkTaskStep (`network_task_step`)

Individual steps within network tasks.

**Key Fields:**

- `id` \- Primary key  
- `network_task_id` \- Foreign key  
- Step definition and status

---

### Read-Only Resources

#### Service (`service`)

Service records (read-only).

**Permissions:** View only

---

#### Subscriber (`subscriber`)

Subscriber records (read-only).

**Permissions:** View only

---

#### Location (`location`)

Location records (read-only).

**Permissions:** View only

---

#### ServicePlan (`service_plan`)

Service plan definitions (read-only).

**Permissions:** View only

---

## Query Operations

### Basic Query

Fetch all resources of a type:

```shell
GET /api/v1/network/resources?table=subnet_v4
```

**Response:**

```json
{
  "success": true,
  "data": [
    {
      "id": 1,
      "vlan_id": 100,
      "network_address": "192.168.1.0",
      "network_size": 24,
      "name": "Customer LAN",
      "type": 2,
      "created_at": "2024-01-15 10:30:00",
      "updated_at": "2024-01-15 10:30:00"
    }
  ]
}
```

---

### Filter by ID

Fetch a specific resource:

```shell
GET /api/v1/network/resources?table=subnet_v4&id=1
```

---

### Filter by Foreign Key

Fetch all subnets for a VLAN:

```shell
GET /api/v1/network/resources?table=subnet_v4&vlan_id=100
```

---

### Multiple Values

Fetch multiple resources by ID:

```shell
GET /api/v1/network/resources?table=subnet_v4&id=1,2,3
```

This returns subnets with id 1, 2, or 3\.

---

## Advanced Filtering

### Comparison Operators

The API supports advanced filtering using operator suffixes:

| Operator | Suffix | Example | Description |
| :---- | :---- | :---- | :---- |
| \= | (none) | `id=5` | Equal to |
| \!= | `_not` | `id_not=5` | Not equal to |
| \> | `_gt` | `id_gt=5` | Greater than |
| \< | `_lt` | `id_lt=10` | Less than |
| \>= | `_gte` | `id_gte=5` | Greater than or equal |
| \<= | `_lte` | `id_lte=10` | Less than or equal |

### Examples

#### Greater Than

Fetch subnets with id \> 100:

```shell
GET /api/v1/network/resources?table=subnet_v4&id_gt=100
```

#### Range Query

Fetch subnets with id between 50 and 150:

```shell
GET /api/v1/network/resources?table=subnet_v4&id_gte=50&id_lte=150
```

#### Exclude Values

Fetch subnets not belonging to VLAN 100:

```shell
GET /api/v1/network/resources?table=subnet_v4&vlan_id_not=100
```

#### Complex Filters

Fetch WAN subnets (type=1) created after a specific date:

```shell
GET /api/v1/network/resources?table=subnet_v4&type=1&id_gt=1000
```

---

## Create Operations

### Create Subnet

**Request:**

```shell
POST /api/v1/network/resources
Content-Type: application/json
Authorization: Bearer YOUR_API_TOKEN

{
  "table": "subnet_v4",
  "vlan_id": 100,
  "network_address": "192.168.100.0",
  "network_size": 24,
  "type": 2,
  "name": "New Customer LAN",
  "ipblock_id": 5,
  "task_id": "task-123",
  "staff": "admin@example.com"
}
```

**Response:**

```json
{
  "success": true,
  "data": {
    "id": 456,
    "vlan_id": 100,
    "network_address": "192.168.100.0",
    "network_size": 24,
    "type": 2,
    "name": "New Customer LAN",
    "ipblock_id": 5,
    "first_address": 3232261120,
    "last_address": 3232261375,
    "created_at": "2024-01-20 14:30:00",
    "updated_at": "2024-01-20 14:30:00"
  }
}
```

**Notes:**

- `first_address` and `last_address` are automatically calculated  
- Network validation ensures no conflicts  
- Audit log entry is automatically created  
- All required fields must be provided (check ACL config)

---

### Create VLAN

**Request:**

```shell
POST /api/v1/network/resources
Content-Type: application/json
Authorization: Bearer YOUR_API_TOKEN

{
  "table": "vlan",
  "vid": 200,
  "csw_id": 10,
  "type": 1,
  "name": "Customer VLAN 200",
  "description": "Production customer VLAN",
  "ports": "1|2|3|4",
  "task_id": "task-124"
}
```

**Response:**

```json
{
  "success": true,
  "data": {
    "id": 789,
    "vid": 200,
    "csw_id": 10,
    "type": 1,
    "name": "Customer VLAN 200",
    "description": "Production customer VLAN",
    "ports": "1|2|3|4",
    "created_at": "2024-01-20 14:35:00",
    "updated_at": "2024-01-20 14:35:00"
  }
}
```

---

### Create BGP Session

**Request:**

```shell
POST /api/v1/network/resources
Content-Type: application/json
Authorization: Bearer YOUR_API_TOKEN

{
  "table": "bgp_session",
  "service_id": 5000,
  "vlan_id": 100,
  "csw_id": 10,
  "subnet_v4_id": 456,
  "interface_name": "ge-0/0/1",
  "local_asn": 65000,
  "remote_asn": 65001,
  "ip_version": 4,
  "local_network_address": "192.168.100.1",
  "remote_network_address": "192.168.100.2",
  "ingress_filter_id": 20,
  "egress_filter_id": 21,
  "md5_key": "secret-key-here",
  "task_id": "task-125"
}
```

**Business Rules Validated:**

- Ingress and egress filters are different  
- IP addresses are within subnet\_v4\_id range  
- VLAN belongs to CSW  
- Subnet belongs to VLAN  
- Filters belong to service's subscriber

---

### Create Network Device

**Request:**

```shell
POST /api/v1/network/resources
Content-Type: application/json
Authorization: Bearer YOUR_API_TOKEN

{
  "table": "network_device",
  "hostname": "switch01.pop5.example.com",
  "pop_id": 5,
  "type": 1,
  "task_id": "task-126"
}
```

---

## Update Operations

### Update Subnet

**Request:**

```shell
PATCH /api/v1/network/resources
Content-Type: application/json
Authorization: Bearer YOUR_API_TOKEN

{
  "table": "subnet_v4",
  "id": 456,
  "name": "Updated Customer LAN",
  "type": 2,
  "task_id": "task-127",
  "staff": "admin@example.com"
}
```

**Response:**

```json
{
  "success": true,
  "message": "Resource updated successfully",
  "updated_count": 1
}
```

**Notes:**

- Only provided fields are updated  
- Cannot update: `id`, `created_at`, `updated_at`  
- Audit log captures before/after values  
- Validation rules still apply

---

### Update Multiple Resources

Update all subnets in a VLAN:

**Request:**

```shell
PATCH /api/v1/network/resources
Content-Type: application/json
Authorization: Bearer YOUR_API_TOKEN

{
  "table": "subnet_v4",
  "vlan_id": 100,
  "type": 2,
  "task_id": "task-128"
}
```

This updates the `type` field for all subnets where `vlan_id=100`.

---

### Update VLAN Ports

**Request:**

```shell
PATCH /api/v1/network/resources
Content-Type: application/json
Authorization: Bearer YOUR_API_TOKEN

{
  "table": "vlan",
  "id": 789,
  "ports": "1|2|3|4|5|6",
  "task_id": "task-129"
}
```

---

### Update BGP Session MD5 Key

**Request:**

```shell
PATCH /api/v1/network/resources
Content-Type: application/json
Authorization: Bearer YOUR_API_TOKEN

{
  "table": "bgp_session",
  "id": 50,
  "md5_key": "new-secret-key",
  "task_id": "task-130"
}
```

---

## Delete Operations

### Delete Subnet

**Request:**

```shell
DELETE /api/v1/network/resources
Content-Type: application/json
Authorization: Bearer YOUR_API_TOKEN

{
  "table": "subnet_v4",
  "id": 456,
  "task_id": "task-131",
  "staff": "admin@example.com"
}
```

**Response:**

```json
{
  "success": true,
  "message": "Resource deleted successfully",
  "deleted_count": 1
}
```

**Notes:**

- Audit log entry is created  
- Foreign key constraints may prevent deletion  
- Some resources check for dependencies before deletion

---

### Delete Multiple Resources

Delete all subnets for a VLAN:

**Request:**

```shell
DELETE /api/v1/network/resources
Content-Type: application/json
Authorization: Bearer YOUR_API_TOKEN

{
  "table": "subnet_v4",
  "vlan_id": 100,
  "task_id": "task-132"
}
```

**Warning:** This deletes ALL subnets matching the filter. Use with caution.

---

### Delete VLAN

**Request:**

```shell
DELETE /api/v1/network/resources
Content-Type: application/json
Authorization: Bearer YOUR_API_TOKEN

{
  "table": "vlan",
  "id": 789,
  "task_id": "task-133"
}
```

**Note:** May fail if subnets or other resources reference this VLAN.

---

## Business Logic & Validation

### Subnet Validation

**SubnetV4 Business Rules:**

1. **Network Address Validation**  
     
   - Must be a valid IPv4 address  
   - Must be the network address for the given prefix  
   - Example: For `/24`, `192.168.1.0` is valid, `192.168.1.5` is not

   

2. **IPBlock Containment**  
     
   - Subnet must fit entirely within parent `ipblock_id`  
   - Validates using binary address comparisons

   

3. **Conflict Detection**  
     
   - No overlapping subnets allowed  
   - Checks for existing subnets that would conflict

   

4. **Automatic Calculations**  
     
   - `first_address` \- Binary representation of first usable IP  
   - `last_address` \- Binary representation of last usable IP  
   - Gateway calculation based on `next_hop` or default

   

5. **KEA DHCP Integration**  
     
   - `kea_subnet_id` links to DHCP server configuration  
   - Ensures DHCP reservations are properly scoped

   

6. **Eero Integration**  
     
   - Optional Eero mesh network fields  
   - Validates Eero network exists if provided

**Size Limits:**

- `network_size` must be 24-32 for SubnetV4  
- Represents CIDR prefix length

---

### VLAN Validation

**VLAN Business Rules:**

1. **VID Range**  
     
   - Must be 0-4095 (valid VLAN ID range)

   

2. **CSW Relationship**  
     
   - Must belong to an existing CSW  
   - VLAN cannot exist without CSW

   

3. **Port Format**  
     
   - Pipe-delimited list: `1|2|3|4`  
   - Represents switch ports assigned to VLAN

---

### IPBlock Validation

**IPBlock Business Rules:**

1. **Size Restrictions**  
     
   - IPv4: Size must be 16-25  
   - Larger blocks for regional allocation

   

2. **Conflict Detection**  
     
   - No overlapping IPBlocks allowed  
   - Binary address range checking

   

3. **Deletion Protection**  
     
   - Cannot delete if subnets exist within block  
   - Prevents orphaning subnets

   

4. **Utilization Tracking**  
     
   - `pct_utilization` calculated based on allocated subnets  
   - Helps with capacity planning

---

### BGP Session Validation

**BGP Session Business Rules:**

1. **Filter Validation**  
     
   - Ingress and egress filters must be different  
   - Filters must belong to service's subscriber

   

2. **IP Address Validation**  
     
   - Local and remote addresses must be within subnet  
   - IP version must match subnet version

   

3. **Relationship Validation**  
     
   - VLAN must belong to CSW  
   - Subnet must belong to VLAN  
   - Service must exist

   

4. **ASN Validation**  
     
   - Local and remote ASN must be valid AS numbers

---

### General Validation Rules

**Field Validation:**

1. **Prohibited Fields**  
     
   - Cannot set: `id`, `created_at`, `updated_at`  
   - These are system-managed

   

2. **Required Fields**  
     
   - Defined per table in ACL config  
   - Enforced on creation

   

3. **Whitelist-Based**  
     
   - Only whitelisted fields can be set  
   - Prevents setting internal/protected fields

   

4. **Empty Value Rejection**  
     
   - Empty strings and null values rejected for most fields  
   - Use explicit NULL handling where needed

**Transaction Safety:**

- All create/update/delete wrapped in database transactions  
- Rollback on any validation failure  
- Ensures data consistency

---

## Error Handling

### Authentication Errors

**Invalid Token:**

```json
{
  "success": false,
  "error": "Authentication failed",
  "message": "Invalid or missing bearer token"
}
```

**HTTP Status:** 401 Unauthorized

---

### Permission Errors

**Insufficient Permissions:**

```json
{
  "success": false,
  "error": "Permission denied",
  "message": "FlightEngineer role does not have 'create' permission for table 'service'"
}
```

**HTTP Status:** 403 Forbidden

---

### Validation Errors

**Missing Required Field:**

```json
{
  "success": false,
  "error": "Validation failed",
  "message": "Required field 'vlan_id' is missing",
  "field": "vlan_id"
}
```

**Invalid Field Value:**

```json
{
  "success": false,
  "error": "Validation failed",
  "message": "Invalid value for field 'type': must be 0, 1, or 2",
  "field": "type"
}
```

**HTTP Status:** 400 Bad Request

---

### Business Logic Errors

**Conflict Detected:**

```json
{
  "success": false,
  "error": "Conflict detected",
  "message": "Subnet 192.168.1.0/24 conflicts with existing subnet 192.168.0.0/16",
  "conflicting_resource": {
    "id": 123,
    "network_address": "192.168.0.0",
    "network_size": 16
  }
}
```

**Foreign Key Violation:**

```json
{
  "success": false,
  "error": "Foreign key constraint violation",
  "message": "VLAN with id 999 does not exist",
  "field": "vlan_id"
}
```

**HTTP Status:** 409 Conflict

---

### Resource Not Found

**No Resources Match Filter:**

```json
{
  "success": false,
  "error": "Not found",
  "message": "No resources found matching the specified filters"
}
```

**HTTP Status:** 404 Not Found

---

### Server Errors

**Database Error:**

```json
{
  "success": false,
  "error": "Internal server error",
  "message": "Database connection failed"
}
```

**HTTP Status:** 500 Internal Server Error

---

## Audit Logging

All create, update, and delete operations are automatically logged to the `audit_log_api` table.

### Audit Log Schema

**Fields:**

- `id` \- Audit log entry ID  
- `system` \- Always "FlightEngineer" for API operations  
- `table` \- Resource table name  
- `entity_id` \- ID of affected resource  
- `staff` \- Staff identifier (from request)  
- `task_id` \- Task ID (from request)  
- `action` \- Operation type: "create", "update", "delete"  
- `field` \- Modified field name  
- `old_value` \- Previous value (NULL for create)  
- `new_value` \- New value (NULL for delete)  
- `created_at` \- Timestamp

### Audit Log Entry Examples

**Create Operation:**

```
system: FlightEngineer
table: subnet_v4
entity_id: 456
action: create
field: network_address
old_value: NULL
new_value: 192.168.100.0
task_id: task-123
staff: admin@example.com
created_at: 2024-01-20 14:30:00
```

**Update Operation:**

```
system: FlightEngineer
table: subnet_v4
entity_id: 456
action: update
field: name
old_value: Old Customer LAN
new_value: Updated Customer LAN
task_id: task-127
staff: admin@example.com
created_at: 2024-01-20 15:00:00
```

**Delete Operation:**

```
system: FlightEngineer
table: subnet_v4
entity_id: 456
action: delete
field: network_address
old_value: 192.168.100.0
new_value: NULL
task_id: task-131
staff: admin@example.com
created_at: 2024-01-20 16:00:00
```

### Audit Features

1. **Complete History**  
     
   - Every field change tracked separately  
   - Before/after values recorded

   

2. **Binary Field Handling**  
     
   - Binary fields (IP addresses) converted to hex for logging  
   - Human-readable in application layer

   

3. **Task Tracking**  
     
   - `task_id` links operations to provisioning tasks  
   - Enables correlation of related changes

   

4. **Staff Attribution**  
     
   - `staff` field identifies responsible party  
   - Can be email, username, or system identifier

   

5. **Transaction Safety**  
     
   - Audit logs committed with data changes  
   - Rollback includes audit entries

### Querying Audit Logs

To view audit history for a resource:

```sql
SELECT * FROM audit_log_api
WHERE table = 'subnet_v4' AND entity_id = 456
ORDER BY created_at DESC;
```

To view all changes by a staff member:

```sql
SELECT * FROM audit_log_api
WHERE staff = 'admin@example.com'
ORDER BY created_at DESC;
```

To view all changes for a task:

```sql
SELECT * FROM audit_log_api
WHERE task_id = 'task-123'
ORDER BY created_at ASC;
```

---

## Common Use Cases

### Use Case 1: Provision New Customer Service

**Scenario:** Set up network infrastructure for a new fiber customer.

**Steps:**

1. **Create VLAN for Customer**

```shell
POST /api/v1/network/resources
{
  "table": "vlan",
  "vid": 250,
  "csw_id": 10,
  "type": 1,
  "name": "Customer 12345 VLAN",
  "ports": "5|6",
  "task_id": "provision-12345"
}
```

2. **Create WAN Subnet**

```shell
POST /api/v1/network/resources
{
  "table": "subnet_v4",
  "vlan_id": <created_vlan_id>,
  "network_address": "192.168.250.0",
  "network_size": 30,
  "type": 1,
  "name": "Customer 12345 WAN",
  "ipblock_id": 5,
  "task_id": "provision-12345"
}
```

3. **Create LAN Subnet**

```shell
POST /api/v1/network/resources
{
  "table": "subnet_v4",
  "vlan_id": <created_vlan_id>,
  "network_address": "10.100.250.0",
  "network_size": 24,
  "type": 2,
  "name": "Customer 12345 LAN",
  "ipblock_id": 8,
  "eero_network_name": "Customer12345",
  "eero_network_password": "SecurePassword123",
  "task_id": "provision-12345"
}
```

4. **Link to Service**

```shell
POST /api/v1/network/resources
{
  "table": "vlan_service",
  "vlan_id": <created_vlan_id>,
  "service_id": 12345,
  "task_id": "provision-12345"
}
```

**Result:** Complete network stack ready for customer service activation.

---

### Use Case 2: Configure BGP for Enterprise Customer

**Scenario:** Set up BGP peering for enterprise customer with custom routing.

**Steps:**

1. **Query Existing Resources**

```shell
GET /api/v1/network/resources?table=subnet_v4&service_id=5000
```

2. **Create Route Filters**

```shell
POST /api/v1/network/resources
{
  "table": "bgp_session_filter",
  "subscriber_id": 100,
  "name": "Customer 5000 Ingress Filter",
  "task_id": "bgp-config-5000"
}

POST /api/v1/network/resources
{
  "table": "bgp_session_filter",
  "subscriber_id": 100,
  "name": "Customer 5000 Egress Filter",
  "task_id": "bgp-config-5000"
}
```

3. **Create BGP Session**

```shell
POST /api/v1/network/resources
{
  "table": "bgp_session",
  "service_id": 5000,
  "vlan_id": 100,
  "csw_id": 10,
  "subnet_v4_id": 456,
  "interface_name": "ge-0/0/5",
  "local_asn": 65000,
  "remote_asn": 65100,
  "ip_version": 4,
  "local_network_address": "192.168.100.1",
  "remote_network_address": "192.168.100.2",
  "ingress_filter_id": <ingress_filter_id>,
  "egress_filter_id": <egress_filter_id>,
  "md5_key": "BGPSecret2024",
  "task_id": "bgp-config-5000"
}
```

4. **Add Manual Prefixes**

```shell
POST /api/v1/network/resources
{
  "table": "bgp_session_manual_prefix",
  "bgp_session_filter_id": <filter_id>,
  "prefix": "203.0.113.0/24",
  "task_id": "bgp-config-5000"
}
```

**Result:** BGP session configured with filtering and custom routing.

---

### Use Case 3: Migrate Customer to New VLAN

**Scenario:** Move customer service from old VLAN to new VLAN.

**Steps:**

1. **Create New VLAN**

```shell
POST /api/v1/network/resources
{
  "table": "vlan",
  "vid": 300,
  "csw_id": 10,
  "type": 1,
  "name": "Migration VLAN 300",
  "ports": "7|8",
  "task_id": "migrate-12345"
}
```

2. **Update Subnets to New VLAN**

```shell
PATCH /api/v1/network/resources
{
  "table": "subnet_v4",
  "vlan_id": 250,
  "vlan_id": 300,
  "task_id": "migrate-12345"
}
```

Note: This updates all subnets from old VLAN (250) to new VLAN (300).

3. **Update VLAN Service Mapping**

```shell
PATCH /api/v1/network/resources
{
  "table": "vlan_service",
  "vlan_id": 250,
  "vlan_id": 300,
  "task_id": "migrate-12345"
}
```

4. **Verify Migration**

```shell
GET /api/v1/network/resources?table=subnet_v4&vlan_id=300
GET /api/v1/network/resources?table=vlan_service&vlan_id=300
```

5. **Delete Old VLAN**

```shell
DELETE /api/v1/network/resources
{
  "table": "vlan",
  "id": <old_vlan_id>,
  "task_id": "migrate-12345"
}
```

**Result:** Customer seamlessly migrated to new VLAN with full audit trail.

---

### Use Case 4: Bulk Subnet Provisioning

**Scenario:** Provision multiple subnets for new POP location.

**Steps:**

1. **Query Available IP Space**

```shell
GET /api/v1/network/resources?table=ipblock_v4&region_id=5&pct_utilization_lt=80
```

2. **Create Multiple Subnets in Sequence**

```shell
# WAN Subnet 1
POST /api/v1/network/resources
{
  "table": "subnet_v4",
  "vlan_id": 100,
  "network_address": "192.168.1.0",
  "network_size": 30,
  "type": 1,
  "name": "WAN Subnet 1",
  "ipblock_id": 5,
  "task_id": "bulk-provision-pop5"
}

# WAN Subnet 2
POST /api/v1/network/resources
{
  "table": "subnet_v4",
  "vlan_id": 100,
  "network_address": "192.168.1.4",
  "network_size": 30,
  "type": 1,
  "name": "WAN Subnet 2",
  "ipblock_id": 5,
  "task_id": "bulk-provision-pop5"
}

# Continue for remaining subnets...
```

3. **Verify No Conflicts**

```shell
GET /api/v1/network/resources?table=subnet_v4&ipblock_id=5
```

4. **Check Utilization**

```shell
GET /api/v1/network/resources?table=ipblock_v4&id=5
```

**Result:** Multiple subnets provisioned efficiently with conflict checking.

---

### Use Case 5: Audit Trail Review

**Scenario:** Investigate who made changes to a critical BGP session.

**Steps:**

1. **Query BGP Session Details**

```shell
GET /api/v1/network/resources?table=bgp_session&id=50
```

2. **Query Audit Logs** (via database or separate audit API)

```sql
SELECT * FROM audit_log_api
WHERE table = 'bgp_session' AND entity_id = 50
ORDER BY created_at DESC;
```

3. **Review Changes**

```
created_at: 2024-01-20 10:00:00
field: md5_key
old_value: OldSecret
new_value: NewSecret
staff: admin@example.com
task_id: bgp-update-50

created_at: 2024-01-19 14:30:00
field: remote_asn
old_value: 65100
new_value: 65200
staff: engineer@example.com
task_id: bgp-migration-50
```

4. **Correlate with Task**

```shell
GET /api/v1/network/resources?table=network_task&id=bgp-update-50
```

**Result:** Complete audit trail showing who changed what and when.

---

### Use Case 6: Network Device Inventory Management

**Scenario:** Add new switches to inventory and configure VLANs.

**Steps:**

1. **Add Network Device**

```shell
POST /api/v1/network/resources
{
  "table": "network_device",
  "hostname": "switch-pop5-01.example.com",
  "pop_id": 5,
  "type": 1,
  "task_id": "inventory-add-2024"
}
```

2. **Create Core Switch Record**

```shell
POST /api/v1/network/resources
{
  "table": "csw",
  "hostname": "csw-pop5-01.example.com",
  "pop_id": 5,
  "task_id": "inventory-add-2024"
}
```

3. **Create VLANs on Switch**

```shell
POST /api/v1/network/resources
{
  "table": "vlan",
  "vid": 10,
  "csw_id": <created_csw_id>,
  "type": 2,
  "name": "Management VLAN",
  "ports": "1",
  "task_id": "inventory-add-2024"
}
```

4. **Link Devices to Services**

```shell
POST /api/v1/network/resources
{
  "table": "network_device_service",
  "network_device_id": <device_id>,
  "service_id": 9999,
  "task_id": "inventory-add-2024"
}
```

**Result:** Complete device inventory with VLAN configuration.

---

### Use Case 7: Query Complex Relationships

**Scenario:** Find all BGP sessions for a specific CSW.

**Steps:**

1. **Get CSW Details**

```shell
GET /api/v1/network/resources?table=csw&id=10
```

2. **Get VLANs on CSW**

```shell
GET /api/v1/network/resources?table=vlan&csw_id=10
```

3. **Get BGP Sessions for CSW**

```shell
GET /api/v1/network/resources?table=bgp_session&csw_id=10
```

4. **Get Subnets for Each VLAN**

```shell
GET /api/v1/network/resources?table=subnet_v4&vlan_id=<vlan_id_from_step_2>
```

5. **Get Services Linked to VLANs**

```shell
GET /api/v1/network/resources?table=vlan_service&vlan_id=<vlan_id_from_step_2>
```

**Result:** Complete view of all resources associated with a core switch.

---

### Use Case 8: ONT Provisioning and State Tracking

**Scenario:** Provision ONT for new fiber installation and track state.

**Steps:**

1. **Check ONT Inventory**

```shell
GET /api/v1/network/resources?table=inventory_ont&ont_model_id=5
```

2. **Create Subnet for ONT**

```shell
POST /api/v1/network/resources
{
  "table": "subnet_v4",
  "vlan_id": 100,
  "ont_id": <ont_id>,
  "network_address": "10.200.50.0",
  "network_size": 29,
  "type": 2,
  "name": "ONT Customer LAN",
  "ipblock_id": 8,
  "task_id": "ont-provision-789"
}
```

3. **Query ONT State**

```shell
GET /api/v1/network/resources?table=observed_ont_state&ont_id=<ont_id>
```

4. **Query ONT Port Status**

```shell
GET /api/v1/network/resources?table=observed_ont_port_state&ont_id=<ont_id>
```

**Result:** ONT provisioned with network configuration and state monitoring.

---

## Performance Tips

### 1\. Use Specific Filters

Instead of fetching all records and filtering client-side:

```shell
# Good
GET /api/v1/network/resources?table=subnet_v4&vlan_id=100

# Bad
GET /api/v1/network/resources?table=subnet_v4
# Then filter on client
```

### 2\. Use Range Queries Efficiently

For large datasets, use pagination with range queries:

```shell
# Page 1: IDs 1-1000
GET /api/v1/network/resources?table=subnet_v4&id_gte=1&id_lte=1000

# Page 2: IDs 1001-2000
GET /api/v1/network/resources?table=subnet_v4&id_gte=1001&id_lte=2000
```

### 3\. Batch Creates in Transactions

When creating multiple related resources, use the same `task_id` for correlation:

```shell
# All use same task_id for atomic tracking
POST ... {"task_id": "batch-123"}
POST ... {"task_id": "batch-123"}
POST ... {"task_id": "batch-123"}
```

### 4\. Index Usage

The following fields are typically indexed:

- `id` (primary key)  
- Foreign keys (`vlan_id`, `csw_id`, `service_id`, etc.)  
- `network_address` and binary address fields  
- `created_at`, `updated_at`

Structure queries to use these indexes.

### 5\. Minimize Field Updates

Only send fields that actually need updating:

```shell
# Good - only update name
PATCH /api/v1/network/resources
{
  "table": "subnet_v4",
  "id": 456,
  "name": "New Name"
}

# Wasteful - sending unchanged fields
PATCH /api/v1/network/resources
{
  "table": "subnet_v4",
  "id": 456,
  "name": "New Name",
  "type": 2,  # unchanged
  "network_size": 24  # unchanged
}
```

---

## Security Best Practices

### 1\. Token Management

- Store API tokens securely (environment variables, secrets manager)  
- Rotate tokens periodically  
- Use different tokens for different services/environments  
- Never commit tokens to version control

### 2\. Input Validation

- The API validates all input, but don't rely on it exclusively  
- Validate on client side too for better UX  
- Sanitize user input before sending to API

### 3\. Audit Logging

- Always provide `staff` and `task_id` for traceability  
- Use meaningful task IDs that link to your provisioning system  
- Review audit logs regularly for unexpected changes

### 4\. Least Privilege

- Use service accounts with minimal permissions  
- Don't use production tokens in development  
- Consider separate tokens per application/service

### 5\. Error Information Leakage

- Don't expose detailed error messages to end users  
- Log full errors server-side  
- Return generic messages to clients

---

## Troubleshooting

### Common Issues

#### Issue: "Permission denied"

**Cause:** FlightEngineer role doesn't have required permission for the operation. **Solution:** Check ACL configuration in `app/config/acl.php`. Ensure the table has the correct permission for the action (view/create/edit/delete).

#### Issue: "Subnet conflicts with existing subnet"

**Cause:** The network range overlaps with an existing subnet. **Solution:**

1. Query existing subnets in the IPBlock  
2. Choose a non-conflicting network address  
3. Consider using a smaller subnet size

#### Issue: "VLAN does not belong to CSW"

**Cause:** Trying to create a BGP session with mismatched VLAN/CSW relationship. **Solution:**

1. Query the VLAN to find its `csw_id`  
2. Use that `csw_id` in the BGP session creation

#### Issue: "IP address not in subnet"

**Cause:** BGP peer address is outside the subnet range. **Solution:**

1. Query the subnet details to see its range  
2. Ensure local\_network\_address and remote\_network\_address are within the subnet

#### Issue: "Cannot delete IPBlock \- subnets exist"

**Cause:** Attempting to delete an IPBlock that still has subnets allocated. **Solution:**

1. Query all subnets in the IPBlock  
2. Delete or migrate subnets first  
3. Then delete the IPBlock

---

## API Limits and Constraints

### Rate Limiting

- Check with system administrator for specific rate limits  
- Recommended: Max 100 requests per minute per token

### Payload Size

- Maximum request body: 1MB  
- For bulk operations, batch in smaller chunks

### Response Size

- Large queries may be paginated  
- Use filters to reduce response size

### Transaction Timeout

- Long-running operations timeout after 30 seconds  
- Break large batch operations into smaller transactions

---

## Migration and Maintenance

### Adding New Resource Types

To extend the API with new resource types:

1. **Create Eloquent Model**  
     
   - Place in `app/Pilot/Models/`  
   - Define table, fields, relationships  
   - Implement validation logic

   

2. **Register in ModelResolver**  
     
   - Add mapping in `app/Pilot/Utils/ApiModel/ModelResolver.php`  
   - Format: `'table_name' => ModelClass::class`

   

3. **Configure ACL**  
     
   - Add permissions in `app/config/acl.php`  
   - Define allowed roles and fields

   

4. **Test CRUD Operations**  
     
   - Create, read, update, delete  
   - Test validation rules  
   - Verify audit logging

### Schema Migrations

When database schema changes:

1. **Update Model**  
     
   - Add new fields to model class  
   - Update validation rules  
   - Update relationships

   

2. **Update ACL**  
     
   - Add new fields to whitelist  
   - Mark required fields

   

3. **Document Changes**  
     
   - Update this documentation  
   - Notify API consumers  
   - Version API if breaking changes

---

## API Version History

### v1 (Current)

- Initial release with 24+ resource types  
- CRUD operations for network infrastructure  
- Advanced filtering with operators  
- Audit logging  
- Role-based access control

### Future Considerations

- GraphQL endpoint for complex queries  
- Webhook support for event notifications  
- Batch operations endpoint  
- JSON schema validation  
- OpenAPI/Swagger documentation

---

## Support and Contact

For issues, questions, or feature requests:

1. **Check Documentation**  
     
   - Review this document  
   - Check source code comments  
   - Review ACL configuration

   

2. **Review Audit Logs**  
     
   - May reveal cause of issues  
   - Shows history of changes

   

3. **Contact Development Team**  
     
   - Provide: Request details, error messages, task\_id  
   - Include relevant audit log entries

   

4. **Code Locations**  
     
   - Routes: `pilot-main-laravel/app/Pilot/Http/routes_api.php`  
   - Controller: `pilot-main-laravel/app/Pilot/Http/Controllers/Api/ApiController.php`  
   - Models: `pilot-main-laravel/app/Pilot/Models/`

---

## Appendix A: Complete Field Reference

### SubnetV4 Fields

```
id                      - Integer, Primary key, Auto-generated
vlan_id                 - Integer, Required, Foreign key
ont_id                  - Integer, Optional, Foreign key
type                    - Integer, Required (0=NA, 1=WAN, 2=LAN)
kea_subnet_id           - Integer, Optional
name                    - String, Required
ipblock_id              - Integer, Required, Foreign key
network_address         - String, Required (e.g. "192.168.1.0")
network_size            - Integer, Required (24-32)
first_address           - Integer, Auto-calculated, Binary IP
last_address            - Integer, Auto-calculated, Binary IP
next_hop                - Integer, Optional, Binary IP
eero_network_id         - Integer, Optional, Foreign key
eero_network_name       - String, Optional
eero_network_password   - String, Optional
eero_network_transferred- Boolean, Optional
created_at              - Timestamp, Auto-generated
updated_at              - Timestamp, Auto-maintained
```

### VLAN Fields

```
id                  - Integer, Primary key, Auto-generated
vid                 - Integer, Required (0-4095)
csw_id              - Integer, Required, Foreign key
type                - Integer, Required (0=NA, 1=ACCESS, 2=MANAGEMENT)
ports               - String, Optional (pipe-delimited)
name                - String, Required
description         - String, Optional
created_at          - Timestamp, Auto-generated
updated_at          - Timestamp, Auto-maintained
```

### BGPSession Fields

```
id                      - Integer, Primary key, Auto-generated
service_id              - Integer, Required, Foreign key
vlan_id                 - Integer, Required, Foreign key
csw_id                  - Integer, Required, Foreign key
interface_name          - String, Required
local_asn               - Integer, Required
remote_asn              - Integer, Required
ip_version              - Integer, Required (4 or 6)
ingress_filter_id       - Integer, Optional, Foreign key
egress_filter_id        - Integer, Optional, Foreign key
local_network_address   - String, Required
remote_network_address  - String, Required
subnet_v4_id            - Integer, Optional, Foreign key
subnet_v6_id            - Integer, Optional, Foreign key
md5_key                 - String, Optional
created_at              - Timestamp, Auto-generated
updated_at              - Timestamp, Auto-maintained
```

---

## Appendix B: HTTP Status Code Reference

| Status Code | Meaning | Common Causes |
| :---- | :---- | :---- |
| 200 | OK | Successful GET/PATCH/DELETE |
| 201 | Created | Successful POST |
| 400 | Bad Request | Validation error, missing required field |
| 401 | Unauthorized | Invalid or missing token |
| 403 | Forbidden | Insufficient permissions |
| 404 | Not Found | Resource doesn't exist |
| 409 | Conflict | Business logic violation, network conflict |
| 500 | Internal Server Error | Database error, unexpected exception |

---

## Appendix C: Operator Reference

| Operator | Suffix | SQL Equivalent | Example |
| :---- | :---- | :---- | :---- |
| Equal | (none) | `=` | `id=5` |
| Not Equal | `_not` | `!=` | `id_not=5` |
| Greater Than | `_gt` | `>` | `id_gt=5` |
| Less Than | `_lt` | `<` | `id_lt=10` |
| Greater or Equal | `_gte` | `>=` | `id_gte=5` |
| Less or Equal | `_lte` | `<=` | `id_lte=10` |
| In List | (comma) | `IN (...)` | `id=1,2,3` |

---

## Appendix D: Complete cURL Examples

### Authentication

```shell
export API_TOKEN="your-api-token-here"
export API_URL="https://your-domain.com/api/v1/network/resources"
```

### Fetch Resources

```shell
curl -X GET "${API_URL}?table=subnet_v4&vlan_id=100" \
  -H "Authorization: Bearer ${API_TOKEN}" \
  -H "Content-Type: application/json"
```

### Create Resource

```shell
curl -X POST "${API_URL}" \
  -H "Authorization: Bearer ${API_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "table": "subnet_v4",
    "vlan_id": 100,
    "network_address": "192.168.100.0",
    "network_size": 24,
    "type": 2,
    "name": "Customer LAN",
    "ipblock_id": 5,
    "task_id": "task-123",
    "staff": "admin@example.com"
  }'
```

### Update Resource

```shell
curl -X PATCH "${API_URL}" \
  -H "Authorization: Bearer ${API_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "table": "subnet_v4",
    "id": 456,
    "name": "Updated Name",
    "task_id": "task-124",
    "staff": "admin@example.com"
  }'
```

### Delete Resource

```shell
curl -X DELETE "${API_URL}" \
  -H "Authorization: Bearer ${API_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "table": "subnet_v4",
    "id": 456,
    "task_id": "task-125",
    "staff": "admin@example.com"
  }'
```

---

**Document Version:** 1.0 **Last Updated:** 2025-11-25 **API Version:** v1  
