import { useState, useEffect, useCallback } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import {
  Network,
  RefreshCw,
  ChevronRight,
  AlertTriangle,
  MapPin,
  Building2,
  Server,
  Archive,
  Plus,
  ChevronLeft,
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

// Helper to format subnet CIDR
function formatSubnetCIDR(subnet) {
  if (!subnet) return '';
  const addr = subnet.network_address || '';
  if (addr.includes('/')) return addr;
  return subnet.network_size ? `${addr}/${subnet.network_size}` : addr;
}

const PAGE_SIZE = 50;

// Pagination component
function Pagination({ currentPage, totalPages, totalCount, pageSize, onPageChange }) {
  const startItem = (currentPage - 1) * pageSize + 1;
  const endItem = Math.min(currentPage * pageSize, totalCount);

  return (
    <div className="flex items-center justify-between px-4 py-3 border-t border-theme">
      <div className="text-sm text-theme-muted">
        Showing <span className="font-medium text-theme-secondary">{startItem.toLocaleString()}</span> to{' '}
        <span className="font-medium text-theme-secondary">{endItem.toLocaleString()}</span> of{' '}
        <span className="font-medium text-theme-secondary">{totalCount.toLocaleString()}</span> results
      </div>
      <div className="flex items-center gap-2">
        <Button
          variant="secondary"
          size="sm"
          onClick={() => onPageChange(currentPage - 1)}
          disabled={currentPage <= 1}
          className="gap-1"
        >
          <ChevronLeft className="w-4 h-4" />
          Previous
        </Button>
        <span className="text-sm text-theme-secondary px-3">
          Page {currentPage} of {totalPages}
        </span>
        <Button
          variant="secondary"
          size="sm"
          onClick={() => onPageChange(currentPage + 1)}
          disabled={currentPage >= totalPages}
          className="gap-1"
        >
          Next
          <ChevronRight className="w-4 h-4" />
        </Button>
      </div>
    </div>
  );
}

export function Subnets() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  // Get params from URL
  const initialPage = parseInt(searchParams.get('page') || '1', 10);
  const initialSearch = searchParams.get('search') || '';
  const initialPop = searchParams.get('pop') || '';
  const initialIncludeArchived = searchParams.get('archived') === 'true';

  const [subnets, setSubnets] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  // Pagination state
  const [currentPage, setCurrentPage] = useState(initialPage);
  const [totalCount, setTotalCount] = useState(0);

  // Filter state
  const [search, setSearch] = useState(initialSearch);
  const [searchInput, setSearchInput] = useState(initialSearch);
  const [popFilter, setPopFilter] = useState(initialPop);
  const [includeArchived, setIncludeArchived] = useState(initialIncludeArchived);

  const [showCreateModal, setShowCreateModal] = useState(false);

  // Debounce search
  useEffect(() => {
    const timer = setTimeout(() => {
      if (searchInput !== search) {
        setSearch(searchInput);
        setCurrentPage(1);
      }
    }, 300);
    return () => clearTimeout(timer);
  }, [searchInput, search]);

  // Update URL params when filters change
  useEffect(() => {
    const params = new URLSearchParams();
    if (currentPage > 1) params.set('page', currentPage.toString());
    if (search) params.set('search', search);
    if (popFilter) params.set('pop', popFilter);
    if (includeArchived) params.set('archived', 'true');
    setSearchParams(params, { replace: true });
  }, [currentPage, search, popFilter, includeArchived, setSearchParams]);

  const fetchData = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);

      const offset = (currentPage - 1) * PAGE_SIZE;

      const res = await endpoints.listSubnetsPaginated({
        limit: PAGE_SIZE,
        offset,
        pop: popFilter,
        search,
        includeArchived,
      });

      setSubnets(res.subnets || []);
      setTotalCount(res.total_count || 0);
    } catch (err) {
      console.error('Failed to fetch subnets:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, [currentPage, search, popFilter, includeArchived]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  // Auto-refresh (less aggressive with pagination)
  useEffect(() => {
    const interval = setInterval(fetchData, 30000);
    return () => clearInterval(interval);
  }, [fetchData]);

  const totalPages = Math.ceil(totalCount / PAGE_SIZE);

  const handlePageChange = (newPage) => {
    setCurrentPage(newPage);
    window.scrollTo(0, 0);
  };

  const handlePopChange = (value) => {
    setPopFilter(value);
    setCurrentPage(1);
  };

  const handleArchivedToggle = () => {
    setIncludeArchived(!includeArchived);
    setCurrentPage(1);
  };

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
        description={`${totalCount.toLocaleString()} subnets`}
        actions={
          <div className="flex gap-3">
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
            title="Total Subnets"
            value={totalCount.toLocaleString()}
            icon={Network}
          />
          <MetricCard
            title="Current Page"
            value={`${currentPage} / ${totalPages || 1}`}
            icon={MapPin}
          />
          <MetricCard
            title="Per Page"
            value={PAGE_SIZE.toString()}
            icon={Building2}
          />
          <MetricCard
            title="Showing"
            value={subnets.length.toString()}
            icon={Archive}
          />
        </div>

        {/* Filters */}
        <Card className="mb-6">
          <div className="flex flex-wrap gap-4 items-center">
            <SearchInput
              value={searchInput}
              onChange={setSearchInput}
              placeholder="Search subnet, subscriber, location..."
              className="w-72"
            />
            <input
              type="text"
              value={popFilter}
              onChange={(e) => handlePopChange(e.target.value)}
              placeholder="Filter by POP"
              className="px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan w-40 text-sm"
            />
            <label className="flex items-center gap-2 text-sm text-theme-secondary cursor-pointer">
              <input
                type="checkbox"
                checked={includeArchived}
                onChange={handleArchivedToggle}
                className="rounded border-theme bg-surface-primary text-pilot-cyan focus:ring-pilot-cyan"
              />
              Include archived
            </label>
            {(search || popFilter || includeArchived) && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  setSearchInput('');
                  setSearch('');
                  setPopFilter('');
                  setIncludeArchived(false);
                  setCurrentPage(1);
                }}
                className="text-theme-muted hover:text-theme-primary"
              >
                Clear filters
              </Button>
            )}
          </div>
        </Card>

        {/* Subnet List */}
        <Card className="overflow-hidden">
          {loading && subnets.length === 0 ? (
            <div className="text-center py-12 text-theme-muted">
              <RefreshCw className="w-8 h-8 mx-auto mb-4 animate-spin" />
              <p>Loading subnets...</p>
            </div>
          ) : subnets.length === 0 ? (
            <div className="text-center py-12 text-theme-muted">
              <Network className="w-12 h-12 mx-auto mb-4 opacity-50" />
              {totalCount === 0 && !search && !popFilter ? (
                <>
                  <p>No subnets configured</p>
                  <p className="text-sm mt-1">Subnets will appear here after syncing with Pilot</p>
                </>
              ) : (
                <p>No subnets match your filters</p>
              )}
            </div>
          ) : (
            <>
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
                  {subnets.map((subnet) => (
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
                          {subnet.service_id && (
                            <div className="text-xs text-theme-muted">Service ID: {subnet.service_id}</div>
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

              {/* Pagination */}
              {totalPages > 1 && (
                <Pagination
                  currentPage={currentPage}
                  totalPages={totalPages}
                  totalCount={totalCount}
                  pageSize={PAGE_SIZE}
                  onPageChange={handlePageChange}
                />
              )}
            </>
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
