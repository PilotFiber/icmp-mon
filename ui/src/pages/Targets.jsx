import { useState, useEffect, useMemo, useCallback } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import {
  Target,
  Plus,
  RefreshCw,
  ChevronRight,
  AlertTriangle,
  X,
  ChevronLeft,
} from 'lucide-react';

import { PageHeader, PageContent } from '../components/Layout';
import { Card } from '../components/Card';
import { MetricCard } from '../components/MetricCard';
import { StatusBadge, StatusDot } from '../components/StatusBadge';
import { Button } from '../components/Button';
import { SearchInput, Select } from '../components/Input';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '../components/Table';
import { Modal, ModalFooter } from '../components/Modal';
import { formatRelativeTime } from '../lib/utils';
import { endpoints } from '../lib/api';

// Target Create Modal
function TargetModal({ isOpen, onClose, tiers, onSave }) {
  const [formData, setFormData] = useState({
    ip: '',
    tier: 'standard',
    tags: {},
  });
  const [tagInput, setTagInput] = useState({ key: '', value: '' });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (isOpen) {
      setFormData({ ip: '', tier: 'standard', tags: {} });
      setError(null);
    }
  }, [isOpen]);

  const handleAddTag = () => {
    if (tagInput.key && tagInput.value) {
      setFormData(prev => ({
        ...prev,
        tags: { ...prev.tags, [tagInput.key]: tagInput.value },
      }));
      setTagInput({ key: '', value: '' });
    }
  };

  const handleRemoveTag = (key) => {
    setFormData(prev => {
      const newTags = { ...prev.tags };
      delete newTags[key];
      return { ...prev, tags: newTags };
    });
  };

  const handleSubmit = async () => {
    setError(null);
    setSaving(true);
    try {
      await endpoints.createTarget(formData);
      onSave();
      onClose();
    } catch (err) {
      setError(err.message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Add Target" size="md">
      <div className="space-y-4">
        {error && (
          <div className="p-3 bg-pilot-red/20 border border-pilot-red/30 rounded-lg text-pilot-red text-sm">
            {error}
          </div>
        )}

        <div>
          <label className="block text-sm font-medium text-theme-secondary mb-1">IP Address</label>
          <input
            type="text"
            value={formData.ip}
            onChange={(e) => setFormData(prev => ({ ...prev, ip: e.target.value }))}
            placeholder="e.g., 192.168.1.1"
            className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
          />
        </div>

        <div>
          <label className="block text-sm font-medium text-theme-secondary mb-1">Tier</label>
          <select
            value={formData.tier}
            onChange={(e) => setFormData(prev => ({ ...prev, tier: e.target.value }))}
            className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
          >
            {tiers.map(tier => (
              <option key={tier.name} value={tier.name}>{tier.display_name || tier.name}</option>
            ))}
          </select>
        </div>

        <div>
          <label className="block text-sm font-medium text-theme-secondary mb-1">Tags</label>
          <div className="flex flex-wrap gap-2 mb-2">
            {Object.entries(formData.tags).map(([key, value]) => (
              <span key={key} className="inline-flex items-center gap-1 px-2 py-1 bg-surface-tertiary rounded text-sm">
                <span className="text-theme-muted">{key}:</span>
                <span className="text-theme-primary">{value}</span>
                <button onClick={() => handleRemoveTag(key)} className="text-theme-muted hover:text-pilot-red ml-1">
                  <X className="w-3 h-3" />
                </button>
              </span>
            ))}
          </div>
          <div className="flex gap-2">
            <input
              type="text"
              value={tagInput.key}
              onChange={(e) => setTagInput(prev => ({ ...prev, key: e.target.value }))}
              placeholder="Key"
              className="flex-1 px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan text-sm"
            />
            <input
              type="text"
              value={tagInput.value}
              onChange={(e) => setTagInput(prev => ({ ...prev, value: e.target.value }))}
              placeholder="Value"
              className="flex-1 px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan text-sm"
            />
            <Button variant="secondary" size="sm" onClick={handleAddTag}>Add</Button>
          </div>
        </div>
      </div>

      <ModalFooter>
        <Button variant="ghost" onClick={onClose} disabled={saving}>Cancel</Button>
        <Button onClick={handleSubmit} disabled={saving || !formData.ip}>
          {saving ? 'Creating...' : 'Create'}
        </Button>
      </ModalFooter>
    </Modal>
  );
}

// Monitoring state options for filtering
const monitoringStates = [
  { value: '', label: 'All States' },
  { value: 'active', label: 'Active' },
  { value: 'degraded', label: 'Degraded' },
  { value: 'down', label: 'Down' },
  { value: 'unresponsive', label: 'Never Responded' },
  { value: 'excluded', label: 'Excluded' },
  { value: 'inactive', label: 'Inactive' },
  { value: 'unknown', label: 'Unknown' },
];

// Map monitoring_state to StatusBadge status prop
const stateToStatus = {
  active: 'healthy',
  degraded: 'degraded',
  down: 'down',
  unresponsive: 'inactive',
  excluded: 'inactive',
  inactive: 'inactive',
  unknown: 'unknown',
};

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

export function Targets() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  // Get params from URL
  const initialPage = parseInt(searchParams.get('page') || '1', 10);
  const initialSearch = searchParams.get('search') || '';
  const initialTier = searchParams.get('tier') || '';
  const initialState = searchParams.get('state') || '';

  const [targets, setTargets] = useState([]);
  const [targetStatuses, setTargetStatuses] = useState({});
  const [tiers, setTiers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  // Pagination state
  const [currentPage, setCurrentPage] = useState(initialPage);
  const [totalCount, setTotalCount] = useState(0);

  // Filter state
  const [search, setSearch] = useState(initialSearch);
  const [searchInput, setSearchInput] = useState(initialSearch);
  const [tierFilter, setTierFilter] = useState(initialTier);
  const [stateFilter, setStateFilter] = useState(initialState);

  const [showTargetModal, setShowTargetModal] = useState(false);

  // Debounce search
  useEffect(() => {
    const timer = setTimeout(() => {
      if (searchInput !== search) {
        setSearch(searchInput);
        setCurrentPage(1); // Reset to first page on search
      }
    }, 300);
    return () => clearTimeout(timer);
  }, [searchInput, search]);

  // Update URL params when filters change
  useEffect(() => {
    const params = new URLSearchParams();
    if (currentPage > 1) params.set('page', currentPage.toString());
    if (search) params.set('search', search);
    if (tierFilter) params.set('tier', tierFilter);
    if (stateFilter) params.set('state', stateFilter);
    setSearchParams(params, { replace: true });
  }, [currentPage, search, tierFilter, stateFilter, setSearchParams]);

  const fetchData = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);

      const offset = (currentPage - 1) * PAGE_SIZE;

      const [targetsRes, tiersRes, statusesRes] = await Promise.all([
        endpoints.listTargetsPaginated({
          limit: PAGE_SIZE,
          offset,
          tier: tierFilter,
          state: stateFilter,
          search,
        }),
        endpoints.listTiers(),
        endpoints.getAllTargetStatuses(),
      ]);

      setTargets(targetsRes.targets || []);
      setTotalCount(targetsRes.total_count || 0);
      setTiers(tiersRes.tiers || []);

      const statusMap = {};
      (statusesRes.statuses || []).forEach(s => {
        statusMap[s.target_id] = s;
      });
      setTargetStatuses(statusMap);
    } catch (err) {
      console.error('Failed to fetch targets:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, [currentPage, search, tierFilter, stateFilter]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  // Auto-refresh (less aggressive with pagination)
  useEffect(() => {
    const interval = setInterval(fetchData, 30000);
    return () => clearInterval(interval);
  }, [fetchData]);

  const totalPages = Math.ceil(totalCount / PAGE_SIZE);

  const tierOptions = useMemo(() => {
    return [
      { value: '', label: 'All Tiers' },
      ...tiers.map(t => ({ value: t.name, label: t.display_name || t.name })),
    ];
  }, [tiers]);

  const targetsWithStatus = useMemo(() => {
    return targets.map(target => ({
      ...target,
      status: targetStatuses[target.id] || null,
    }));
  }, [targets, targetStatuses]);

  const tierColors = {
    infrastructure: 'bg-pilot-red/20 text-pilot-red',
    vip: 'bg-pilot-yellow/20 text-accent',
    standard: 'bg-pilot-cyan/20 text-pilot-cyan',
  };

  const getStateColor = (state) => stateToStatus[state] || 'unknown';

  const getStateLabel = (state) => {
    const found = monitoringStates.find(s => s.value === state);
    return found ? found.label : state || 'Unknown';
  };

  const handlePageChange = (newPage) => {
    setCurrentPage(newPage);
    window.scrollTo(0, 0);
  };

  const handleTierChange = (value) => {
    setTierFilter(value);
    setCurrentPage(1);
  };

  const handleStateChange = (value) => {
    setStateFilter(value);
    setCurrentPage(1);
  };

  if (error) {
    return (
      <>
        <PageHeader title="Targets" />
        <PageContent>
          <Card accent="red">
            <div className="flex items-center gap-3">
              <AlertTriangle className="w-6 h-6 text-pilot-red" />
              <div>
                <h3 className="font-medium text-theme-primary">Failed to load targets</h3>
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
        title="Targets"
        description={`${totalCount.toLocaleString()} monitored targets`}
        actions={
          <div className="flex gap-3">
            <Button variant="secondary" onClick={fetchData} className="gap-2">
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </Button>
            <Button className="gap-2" onClick={() => setShowTargetModal(true)}>
              <Plus className="w-4 h-4" />
              Add Target
            </Button>
          </div>
        }
      />

      <PageContent>
        {/* Summary Cards */}
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
          <MetricCard title="Total Targets" value={totalCount.toLocaleString()} icon={Target} />
          <MetricCard title="Current Page" value={`${currentPage} / ${totalPages || 1}`} />
          <MetricCard title="Per Page" value={PAGE_SIZE.toString()} />
          <MetricCard title="Showing" value={targetsWithStatus.length.toString()} />
        </div>

        {/* Filters */}
        <Card className="mb-6">
          <div className="flex flex-wrap gap-4 items-center">
            <SearchInput
              value={searchInput}
              onChange={setSearchInput}
              placeholder="Search IP, name, or description..."
              className="w-72"
            />
            <Select options={tierOptions} value={tierFilter} onChange={handleTierChange} className="w-40" />
            <Select options={monitoringStates} value={stateFilter} onChange={handleStateChange} className="w-48" />
            {(search || tierFilter || stateFilter) && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  setSearchInput('');
                  setSearch('');
                  setTierFilter('');
                  setStateFilter('');
                  setCurrentPage(1);
                }}
                className="text-theme-muted hover:text-theme-primary"
              >
                Clear filters
              </Button>
            )}
          </div>
        </Card>

        {/* Target List */}
        <Card className="overflow-hidden">
          {loading && targets.length === 0 ? (
            <div className="text-center py-12 text-theme-muted">
              <RefreshCw className="w-8 h-8 mx-auto mb-4 animate-spin" />
              <p>Loading targets...</p>
            </div>
          ) : targetsWithStatus.length === 0 ? (
            <div className="text-center py-12 text-theme-muted">
              <Target className="w-12 h-12 mx-auto mb-4 opacity-50" />
              {totalCount === 0 && !search && !tierFilter && !stateFilter ? (
                <>
                  <p>No targets configured</p>
                  <p className="text-sm mt-1">Add targets to begin monitoring</p>
                </>
              ) : (
                <p>No targets match your filters</p>
              )}
            </div>
          ) : (
            <>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Target</TableHead>
                    <TableHead>Tier</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead className="text-right">Latency</TableHead>
                    <TableHead className="text-right">Loss</TableHead>
                    <TableHead>Agents</TableHead>
                    <TableHead>Last Probe</TableHead>
                    <TableHead></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {targetsWithStatus.map((target) => {
                    const hasExpectedOutcome = target.expected_outcome?.should_succeed === false;
                    const status = target.status;
                    const monitoringState = target.monitoring_state || 'unknown';
                    return (
                      <TableRow
                        key={target.id}
                        onClick={() => navigate(`/targets/${target.id}`)}
                        className="cursor-pointer"
                      >
                        <TableCell>
                          <div className="flex items-center gap-3">
                            <StatusDot status={getStateColor(monitoringState)} pulse={monitoringState === 'down'} />
                            <div>
                              <span className="font-mono text-theme-primary">{target.ip}</span>
                              {target.display_name && (
                                <span className="ml-2 text-sm text-theme-muted">{target.display_name}</span>
                              )}
                            </div>
                            {hasExpectedOutcome && (
                              <span className="text-xs px-1.5 py-0.5 bg-surface-tertiary text-theme-muted rounded">Expected Fail</span>
                            )}
                          </div>
                        </TableCell>
                        <TableCell>
                          <span className={`px-2 py-0.5 rounded text-xs font-medium capitalize ${tierColors[target.tier] || 'bg-gray-500/20 text-theme-muted'}`}>
                            {target.tier}
                          </span>
                        </TableCell>
                        <TableCell>
                          <StatusBadge status={getStateColor(monitoringState)} label={getStateLabel(monitoringState)} size="sm" />
                        </TableCell>
                        <TableCell className="text-right font-mono">
                          {status?.avg_latency_ms != null ? `${status.avg_latency_ms.toFixed(1)}ms` : '—'}
                        </TableCell>
                        <TableCell className="text-right font-mono">
                          <span className={status?.packet_loss_pct > 0 ? 'text-warning' : ''}>
                            {status?.packet_loss_pct != null ? `${status.packet_loss_pct.toFixed(1)}%` : '—'}
                          </span>
                        </TableCell>
                        <TableCell>
                          {status?.total_agents > 0 ? (
                            <span className={status.reachable_agents < status.total_agents ? 'text-warning' : ''}>
                              {status.reachable_agents}/{status.total_agents}
                            </span>
                          ) : '—'}
                        </TableCell>
                        <TableCell className="text-theme-muted text-sm">
                          {status?.last_probe ? formatRelativeTime(status.last_probe) : '—'}
                        </TableCell>
                        <TableCell>
                          <ChevronRight className="w-4 h-4 text-theme-muted" />
                        </TableCell>
                      </TableRow>
                    );
                  })}
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

        {/* Target Create Modal */}
        <TargetModal
          isOpen={showTargetModal}
          onClose={() => setShowTargetModal(false)}
          tiers={tiers}
          onSave={fetchData}
        />
      </PageContent>
    </>
  );
}
