import { useState, useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Network,
  RefreshCw,
  ChevronRight,
  AlertTriangle,
  MapPin,
  Building2,
  Server,
  Archive,
  Eye,
  EyeOff,
  Plus,
} from 'lucide-react';

import { PageHeader, PageContent } from '../components/Layout';
import { Card } from '../components/Card';
import { MetricCard } from '../components/MetricCard';
import { StatusBadge } from '../components/StatusBadge';
import { Button } from '../components/Button';
import { SearchInput, Select } from '../components/Input';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '../components/Table';
import { Modal, ModalFooter } from '../components/Modal';
import { endpoints } from '../lib/api';

// Create Subnet Modal
function CreateSubnetModal({ isOpen, onClose, onSave }) {
  const [formData, setFormData] = useState({
    network_address: '',
    network_size: 24,
    gateway_address: '',
    subscriber_name: '',
    city: '',
    region: '',
    pop_name: '',
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (isOpen) {
      setFormData({
        network_address: '',
        network_size: 24,
        gateway_address: '',
        subscriber_name: '',
        city: '',
        region: '',
        pop_name: '',
      });
      setError(null);
    }
  }, [isOpen]);

  const handleSubmit = async () => {
    setError(null);
    setSaving(true);
    try {
      // Build request with only non-empty optional fields
      // Backend expects network_address in CIDR format (e.g., "192.168.1.0/24")
      const networkSize = parseInt(formData.network_size, 10);
      const req = {
        network_address: `${formData.network_address}/${networkSize}`,
        network_size: networkSize,
      };
      if (formData.gateway_address) req.gateway_address = formData.gateway_address;
      if (formData.subscriber_name) req.subscriber_name = formData.subscriber_name;
      if (formData.city) req.city = formData.city;
      if (formData.region) req.region = formData.region;
      if (formData.pop_name) req.pop_name = formData.pop_name;

      await endpoints.createSubnet(req);
      onSave();
      onClose();
    } catch (err) {
      setError(err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleChange = (field, value) => {
    setFormData(prev => ({ ...prev, [field]: value }));
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Create Subnet" size="md">
      <div className="space-y-4">
        {error && (
          <div className="p-3 bg-pilot-red/20 border border-pilot-red/30 rounded-lg text-pilot-red text-sm">
            {error}
          </div>
        )}

        {/* Network Address (required) */}
        <div>
          <label className="block text-sm font-medium text-theme-secondary mb-1">
            Network Address <span className="text-pilot-red">*</span>
          </label>
          <input
            type="text"
            value={formData.network_address}
            onChange={(e) => handleChange('network_address', e.target.value)}
            placeholder="e.g., 192.168.1.0"
            className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
          />
        </div>

        {/* Network Size (required) */}
        <div>
          <label className="block text-sm font-medium text-theme-secondary mb-1">
            Network Size (CIDR) <span className="text-pilot-red">*</span>
          </label>
          <select
            value={formData.network_size}
            onChange={(e) => handleChange('network_size', e.target.value)}
            className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
          >
            {[...Array(25)].map((_, i) => {
              const size = i + 8;
              const hosts = Math.pow(2, 32 - size) - 2;
              return (
                <option key={size} value={size}>
                  /{size} ({hosts > 0 ? hosts.toLocaleString() : 0} hosts)
                </option>
              );
            })}
          </select>
        </div>

        {/* Gateway Address (optional) */}
        <div>
          <label className="block text-sm font-medium text-theme-secondary mb-1">
            Gateway Address
          </label>
          <input
            type="text"
            value={formData.gateway_address}
            onChange={(e) => handleChange('gateway_address', e.target.value)}
            placeholder="e.g., 192.168.1.1"
            className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
          />
        </div>

        {/* Subscriber Name (optional) */}
        <div>
          <label className="block text-sm font-medium text-theme-secondary mb-1">
            Subscriber Name
          </label>
          <input
            type="text"
            value={formData.subscriber_name}
            onChange={(e) => handleChange('subscriber_name', e.target.value)}
            placeholder="e.g., Acme Corp"
            className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
          />
        </div>

        {/* Location: City, Region, POP */}
        <div className="grid grid-cols-3 gap-3">
          <div>
            <label className="block text-sm font-medium text-theme-secondary mb-1">City</label>
            <input
              type="text"
              value={formData.city}
              onChange={(e) => handleChange('city', e.target.value)}
              placeholder="New York"
              className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan text-sm"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-theme-secondary mb-1">Region</label>
            <input
              type="text"
              value={formData.region}
              onChange={(e) => handleChange('region', e.target.value)}
              placeholder="NY"
              className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan text-sm"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-theme-secondary mb-1">POP</label>
            <input
              type="text"
              value={formData.pop_name}
              onChange={(e) => handleChange('pop_name', e.target.value)}
              placeholder="NYC-1"
              className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan text-sm"
            />
          </div>
        </div>
      </div>

      <ModalFooter>
        <Button variant="ghost" onClick={onClose} disabled={saving}>Cancel</Button>
        <Button onClick={handleSubmit} disabled={saving || !formData.network_address}>
          {saving ? 'Creating...' : 'Create Subnet'}
        </Button>
      </ModalFooter>
    </Modal>
  );
}

// Helper to format subnet CIDR (handles case where network_address may already include prefix)
function formatSubnetCIDR(subnet) {
  if (!subnet) return '';
  const addr = subnet.network_address || '';
  if (addr.includes('/')) return addr;
  return subnet.network_size ? `${addr}/${subnet.network_size}` : addr;
}

export function Subnets() {
  const navigate = useNavigate();
  const [subnets, setSubnets] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [search, setSearch] = useState('');
  const [popFilter, setPopFilter] = useState('');
  const [stateFilter, setStateFilter] = useState('');
  const [showArchived, setShowArchived] = useState(false);
  const [showCreateModal, setShowCreateModal] = useState(false);

  const fetchData = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await endpoints.listSubnets();
      setSubnets(res.subnets || []);
    } catch (err) {
      console.error('Failed to fetch subnets:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  // Get unique POPs for filter
  const pops = useMemo(() => {
    const uniquePops = [...new Set(subnets.map(s => s.pop_name).filter(Boolean))];
    return [
      { value: '', label: 'All POPs' },
      ...uniquePops.map(p => ({ value: p, label: p })),
    ];
  }, [subnets]);

  const stateOptions = [
    { value: '', label: 'All States' },
    { value: 'active', label: 'Active' },
    { value: 'archived', label: 'Archived' },
  ];

  const filteredSubnets = useMemo(() => {
    return subnets.filter(subnet => {
      // Filter archived
      if (!showArchived && subnet.archived_at) return false;
      if (stateFilter === 'active' && subnet.archived_at) return false;
      if (stateFilter === 'archived' && !subnet.archived_at) return false;

      // Search
      if (search) {
        const searchLower = search.toLowerCase();
        const matchesNetwork = subnet.network_address?.toLowerCase().includes(searchLower);
        const matchesSubscriber = subnet.subscriber_name?.toLowerCase().includes(searchLower);
        const matchesPop = subnet.pop_name?.toLowerCase().includes(searchLower);
        const matchesCity = subnet.city?.toLowerCase().includes(searchLower);
        if (!matchesNetwork && !matchesSubscriber && !matchesPop && !matchesCity) {
          return false;
        }
      }

      // POP filter
      if (popFilter && subnet.pop_name !== popFilter) return false;

      return true;
    });
  }, [subnets, search, popFilter, stateFilter, showArchived]);

  const stats = useMemo(() => {
    const active = subnets.filter(s => !s.archived_at);
    const archived = subnets.filter(s => s.archived_at);
    const popCount = new Set(active.map(s => s.pop_name).filter(Boolean)).size;
    const subscriberCount = new Set(active.map(s => s.subscriber_id).filter(Boolean)).size;
    return {
      total: active.length,
      archived: archived.length,
      pops: popCount,
      subscribers: subscriberCount,
    };
  }, [subnets]);

  if (error) {
    return (
      <>
        <PageHeader title="Subnets" />
        <PageContent>
          <Card accent="red">
            <div className="flex items-center gap-3">
              <AlertTriangle className="w-6 h-6 text-pilot-red" />
              <div>
                <h3 className="font-medium text-theme-primary">Failed to load subnets</h3>
                <p className="text-sm text-theme-muted">{error}</p>
              </div>
              <Button variant="secondary" size="sm" onClick={fetchData} className="ml-auto">
                Retry
              </Button>
            </div>
          </Card>
        </PageContent>
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="Subnets"
        description={`${stats.total} active subnets across ${stats.pops} POPs`}
        actions={
          <div className="flex gap-3">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowArchived(!showArchived)}
              className="gap-2"
            >
              {showArchived ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
              {showArchived ? 'Hide Archived' : 'Show Archived'}
            </Button>
            <Button variant="secondary" onClick={fetchData} className="gap-2">
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </Button>
            <Button onClick={() => setShowCreateModal(true)} className="gap-2">
              <Plus className="w-4 h-4" />
              Create Subnet
            </Button>
          </div>
        }
      />

      <PageContent>
        {/* Summary Cards */}
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
          <MetricCard
            title="Active Subnets"
            value={stats.total.toLocaleString()}
            icon={Network}
          />
          <MetricCard
            title="POPs"
            value={stats.pops.toLocaleString()}
            icon={MapPin}
          />
          <MetricCard
            title="Subscribers"
            value={stats.subscribers.toLocaleString()}
            icon={Building2}
          />
          <MetricCard
            title="Archived"
            value={stats.archived.toLocaleString()}
            icon={Archive}
            status={stats.archived > 0 ? 'degraded' : 'healthy'}
          />
        </div>

        {/* Filters */}
        <Card className="mb-6">
          <div className="flex flex-wrap gap-4 items-center">
            <SearchInput
              value={search}
              onChange={setSearch}
              placeholder="Search subnet, subscriber, POP..."
              className="w-72"
            />
            <Select
              options={pops}
              value={popFilter}
              onChange={setPopFilter}
              className="w-40"
            />
            <Select
              options={stateOptions}
              value={stateFilter}
              onChange={setStateFilter}
              className="w-40"
            />
          </div>
        </Card>

        {/* Subnet List */}
        <Card>
          {loading ? (
            <div className="flex items-center justify-center py-12">
              <RefreshCw className="w-6 h-6 animate-spin text-theme-muted" />
            </div>
          ) : filteredSubnets.length === 0 ? (
            <div className="text-center py-12 text-theme-muted">
              <Network className="w-12 h-12 mx-auto mb-4 opacity-50" />
              {subnets.length === 0 ? (
                <>
                  <p>No subnets configured</p>
                  <p className="text-sm mt-1">Subnets will appear here after syncing with Pilot</p>
                </>
              ) : (
                <p>No subnets match your filters</p>
              )}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Network</TableHead>
                  <TableHead>Subscriber</TableHead>
                  <TableHead>POP</TableHead>
                  <TableHead>Gateway</TableHead>
                  <TableHead>Location</TableHead>
                  <TableHead>State</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredSubnets.map((subnet) => (
                  <TableRow
                    key={subnet.id}
                    onClick={() => navigate(`/subnets/${subnet.id}`)}
                    className={`cursor-pointer ${subnet.archived_at ? 'opacity-60' : ''}`}
                  >
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Network className="w-4 h-4 text-pilot-cyan" />
                        <span className="font-mono text-theme-primary">
                          {formatSubnetCIDR(subnet)}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div>
                        <div className="text-theme-primary">{subnet.subscriber_name || '—'}</div>
                        {subnet.subscriber_id && (
                          <div className="text-xs text-theme-muted">ID: {subnet.subscriber_id}</div>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <span className="px-2 py-0.5 bg-pilot-cyan/20 text-pilot-cyan rounded text-xs font-medium">
                        {subnet.pop_name || '—'}
                      </span>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Server className="w-3 h-3 text-theme-muted" />
                        <span className="font-mono text-sm text-theme-secondary">
                          {subnet.gateway_address || '—'}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="text-sm">
                        <div className="text-theme-primary">{subnet.city || '—'}</div>
                        <div className="text-xs text-theme-muted">{subnet.region}</div>
                      </div>
                    </TableCell>
                    <TableCell>
                      {subnet.archived_at ? (
                        <StatusBadge status="down" label="Archived" size="sm" />
                      ) : (
                        <StatusBadge status="healthy" label="Active" size="sm" />
                      )}
                    </TableCell>
                    <TableCell>
                      <ChevronRight className="w-4 h-4 text-theme-muted" />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Card>
      </PageContent>

      {/* Create Subnet Modal */}
      <CreateSubnetModal
        isOpen={showCreateModal}
        onClose={() => setShowCreateModal(false)}
        onSave={fetchData}
      />
    </>
  );
}
