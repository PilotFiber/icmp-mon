import { useState, useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Target,
  Plus,
  RefreshCw,
  Tag,
  ChevronRight,
  AlertTriangle,
  X,
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

// States to hide by default (not alertable or user-disabled)
const hiddenByDefault = ['unresponsive', 'excluded', 'inactive'];

function TagList({ tags }) {
  const entries = Object.entries(tags || {}).filter(([k]) => k !== 'expectedOutcome');
  if (entries.length === 0) return <span className="text-theme-muted text-xs">No tags</span>;

  return (
    <div className="flex flex-wrap gap-1">
      {entries.slice(0, 3).map(([key, value]) => (
        <span key={key} className="inline-flex items-center px-2 py-0.5 rounded text-xs bg-surface-tertiary text-theme-secondary">
          <span className="text-theme-muted">{key}:</span>
          <span className="ml-1">{value}</span>
        </span>
      ))}
      {entries.length > 3 && <span className="text-xs text-theme-muted">+{entries.length - 3} more</span>}
    </div>
  );
}

export function Targets() {
  const navigate = useNavigate();
  const [targets, setTargets] = useState([]);
  const [targetStatuses, setTargetStatuses] = useState({});
  const [tiers, setTiers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [search, setSearch] = useState('');
  const [tierFilter, setTierFilter] = useState('');
  const [stateFilter, setStateFilter] = useState(''); // Empty means "show alertable states"
  const [showHidden, setShowHidden] = useState(false); // Toggle to show unresponsive/excluded/inactive
  const [showTargetModal, setShowTargetModal] = useState(false);

  const fetchData = async () => {
    try {
      setLoading(true);
      setError(null);
      const [targetsRes, tiersRes, statusesRes] = await Promise.all([
        endpoints.listTargets(),
        endpoints.listTiers(),
        endpoints.getAllTargetStatuses(),
      ]);
      setTargets(targetsRes.targets || []);
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
  };

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 10000);
    return () => clearInterval(interval);
  }, []);

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

  const filteredTargets = useMemo(() => {
    return targetsWithStatus.filter((target) => {
      const targetState = target.monitoring_state || 'unknown';

      // By default, hide unresponsive/excluded/inactive unless showHidden is true or specific filter selected
      if (!showHidden && !stateFilter && hiddenByDefault.includes(targetState)) {
        return false;
      }

      if (search) {
        const searchLower = search.toLowerCase();
        const ip = target.ip || '';
        const matchesIP = ip.includes(search);
        const matchesTags = Object.values(target.tags || {}).some(
          (v) => String(v).toLowerCase().includes(searchLower)
        );
        const matchesSubscriber = target.subscriber_id?.toLowerCase().includes(searchLower);
        if (!matchesIP && !matchesTags && !matchesSubscriber) return false;
      }
      if (tierFilter && target.tier !== tierFilter) return false;
      if (stateFilter && targetState !== stateFilter) return false;
      return true;
    });
  }, [targetsWithStatus, search, tierFilter, stateFilter, showHidden]);

  const stats = useMemo(() => {
    const total = targetsWithStatus.length;
    const active = targetsWithStatus.filter(t => t.monitoring_state === 'active').length;
    const degraded = targetsWithStatus.filter(t => t.monitoring_state === 'degraded').length;
    const down = targetsWithStatus.filter(t => t.monitoring_state === 'down').length;
    const unresponsive = targetsWithStatus.filter(t => t.monitoring_state === 'unresponsive').length;
    const excluded = targetsWithStatus.filter(t => t.monitoring_state === 'excluded').length;
    const inactive = targetsWithStatus.filter(t => t.monitoring_state === 'inactive').length;
    const unknown = targetsWithStatus.filter(t => !t.monitoring_state || t.monitoring_state === 'unknown').length;
    return { total, active, degraded, down, unresponsive, excluded, inactive, unknown };
  }, [targetsWithStatus]);

  const tierColors = {
    infrastructure: 'bg-pilot-red/20 text-pilot-red',
    vip: 'bg-pilot-yellow/20 text-accent',
    standard: 'bg-pilot-cyan/20 text-pilot-cyan',
  };

  // Get the StatusBadge color from monitoring_state
  const getStateColor = (state) => stateToStatus[state] || 'unknown';

  // Get human-readable label for monitoring state
  const getStateLabel = (state) => {
    const found = monitoringStates.find(s => s.value === state);
    return found ? found.label : state || 'Unknown';
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
        description={`${stats.total} monitored targets`}
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
        <div className="grid grid-cols-1 md:grid-cols-5 gap-4 mb-6">
          <MetricCard title="Total Targets" value={stats.total.toLocaleString()} icon={Target} />
          <MetricCard title="Active" value={stats.active.toLocaleString()} status="healthy" />
          <MetricCard title="Degraded" value={stats.degraded.toLocaleString()} status={stats.degraded > 0 ? 'degraded' : 'healthy'} />
          <MetricCard title="Down" value={stats.down.toLocaleString()} status={stats.down > 0 ? 'down' : 'healthy'} />
          <MetricCard title="Hidden" value={(stats.unresponsive + stats.excluded + stats.inactive).toLocaleString()} />
        </div>

        {/* Filters */}
        <Card className="mb-6">
          <div className="flex flex-wrap gap-4 items-center">
            <SearchInput value={search} onChange={setSearch} placeholder="Search IP or tags..." className="w-72" />
            <Select options={tierOptions} value={tierFilter} onChange={setTierFilter} className="w-40" />
            <Select options={monitoringStates} value={stateFilter} onChange={setStateFilter} className="w-48" />
            <label className="flex items-center gap-2 text-sm text-theme-secondary cursor-pointer">
              <input
                type="checkbox"
                checked={showHidden}
                onChange={(e) => setShowHidden(e.target.checked)}
                className="rounded border-theme bg-surface-primary text-pilot-cyan focus:ring-pilot-cyan"
              />
              Show hidden ({stats.unresponsive + stats.excluded + stats.inactive})
            </label>
          </div>
        </Card>

        {/* Target List */}
        <Card>
          {filteredTargets.length === 0 ? (
            <div className="text-center py-12 text-theme-muted">
              <Target className="w-12 h-12 mx-auto mb-4 opacity-50" />
              {targets.length === 0 ? (
                <>
                  <p>No targets configured</p>
                  <p className="text-sm mt-1">Add targets to begin monitoring</p>
                </>
              ) : (
                <p>No targets match your filters</p>
              )}
            </div>
          ) : (
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
                {filteredTargets.map((target) => {
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
                          <span className="font-mono text-theme-primary">{target.ip}</span>
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
