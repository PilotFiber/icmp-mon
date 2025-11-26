import { useState, useEffect, useMemo } from 'react';
import {
  RefreshCw,
  Rocket,
  Pause,
  Play,
  RotateCcw,
  AlertTriangle,
  CheckCircle2,
  Clock,
  Server,
  ChevronRight,
  PieChart,
} from 'lucide-react';

import { PageHeader, PageContent } from '../components/Layout';
import { Card, CardTitle, CardContent } from '../components/Card';
import { MetricCard } from '../components/MetricCard';
import { StatusBadge } from '../components/StatusBadge';
import { Button } from '../components/Button';
import { Select } from '../components/Input';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '../components/Table';
import { Modal } from '../components/Modal';
import { formatRelativeTime } from '../lib/utils';
import { endpoints } from '../lib/api';

const strategyOptions = [
  { value: 'staged', label: 'Staged (10% → 25% → 50% → 100%)' },
  { value: 'canary', label: 'Canary (5% first, then staged)' },
  { value: 'immediate', label: 'Immediate (All at once)' },
  { value: 'manual', label: 'Manual (Operator controlled)' },
];

const rolloutStatusColors = {
  pending: 'text-theme-muted',
  in_progress: 'text-accent',
  paused: 'text-orange-400',
  completed: 'text-status-healthy',
  failed: 'text-pilot-red',
  rolled_back: 'text-orange-400',
};

export function Fleet() {
  const [versions, setVersions] = useState({});
  const [total, setTotal] = useState(0);
  const [rollouts, setRollouts] = useState([]);
  const [releases, setReleases] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [selectedRollout, setSelectedRollout] = useState(null);

  const fetchData = async () => {
    try {
      setLoading(true);
      setError(null);

      const [versionsRes, rolloutsRes, releasesRes] = await Promise.all([
        endpoints.getFleetVersions().catch(() => ({ versions: {}, total: 0 })),
        endpoints.listRollouts().catch(() => ({ rollouts: [] })),
        endpoints.listReleases().catch(() => ({ releases: [] })),
      ]);

      setVersions(versionsRes.versions || {});
      setTotal(versionsRes.total || 0);
      setRollouts(rolloutsRes.rollouts || []);
      setReleases(releasesRes.releases || []);
    } catch (err) {
      console.error('Failed to fetch fleet data:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 15000);
    return () => clearInterval(interval);
  }, []);

  const versionStats = useMemo(() => {
    const entries = Object.entries(versions);
    const sortedEntries = entries.sort((a, b) => b[1] - a[1]);
    return sortedEntries.map(([version, count]) => ({
      version,
      count,
      percent: total > 0 ? Math.round((count / total) * 100) : 0,
    }));
  }, [versions, total]);

  const activeRollouts = useMemo(() => {
    return rollouts.filter(r => ['pending', 'in_progress', 'paused'].includes(r.status));
  }, [rollouts]);

  const handlePauseRollout = async (id) => {
    try {
      await endpoints.pauseRollout(id);
      fetchData();
    } catch (err) {
      console.error('Failed to pause rollout:', err);
    }
  };

  const handleResumeRollout = async (id) => {
    try {
      await endpoints.resumeRollout(id);
      fetchData();
    } catch (err) {
      console.error('Failed to resume rollout:', err);
    }
  };

  const handleRollback = async (id) => {
    if (!confirm('Are you sure you want to rollback this rollout?')) return;
    try {
      await endpoints.rollbackRollout(id, 'User initiated rollback');
      fetchData();
    } catch (err) {
      console.error('Failed to rollback:', err);
    }
  };

  if (error) {
    return (
      <>
        <PageHeader title="Fleet Management" />
        <PageContent>
          <Card accent="red">
            <div className="flex items-center gap-3">
              <AlertTriangle className="w-6 h-6 text-pilot-red" />
              <div>
                <h3 className="font-medium text-theme-primary">Failed to load fleet data</h3>
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
        title="Fleet Management"
        description="Agent versions and rollout management"
        actions={
          <div className="flex gap-3">
            <Button variant="secondary" onClick={fetchData} className="gap-2">
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </Button>
            <Button onClick={() => setShowCreateModal(true)} className="gap-2">
              <Rocket className="w-4 h-4" />
              New Rollout
            </Button>
          </div>
        }
      />

      <PageContent>
        {/* Version Distribution */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-6">
          <Card className="lg:col-span-2">
            <CardTitle icon={PieChart}>Version Distribution</CardTitle>
            <CardContent>
              {versionStats.length === 0 ? (
                <div className="text-center py-8 text-theme-muted">
                  <Server className="w-12 h-12 mx-auto mb-4 opacity-50" />
                  <p>No version data available</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {versionStats.map(({ version, count, percent }) => (
                    <div key={version} className="flex items-center gap-4">
                      <div className="w-24 font-mono text-sm text-theme-primary">{version}</div>
                      <div className="flex-1">
                        <div className="h-6 bg-surface-tertiary rounded-full overflow-hidden">
                          <div
                            className="h-full bg-pilot-cyan transition-all duration-500"
                            style={{ width: `${percent}%` }}
                          />
                        </div>
                      </div>
                      <div className="w-20 text-right text-sm">
                        <span className="text-theme-primary font-medium">{count}</span>
                        <span className="text-theme-muted"> ({percent}%)</span>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardTitle>Fleet Summary</CardTitle>
            <CardContent>
              <div className="space-y-4">
                <MetricCard
                  title="Total Agents"
                  value={total}
                  icon={Server}
                  size="sm"
                />
                <MetricCard
                  title="Active Rollouts"
                  value={activeRollouts.length}
                  icon={Rocket}
                  size="sm"
                  status={activeRollouts.length > 0 ? 'degraded' : 'healthy'}
                />
                <MetricCard
                  title="Versions in Fleet"
                  value={Object.keys(versions).length}
                  icon={PieChart}
                  size="sm"
                />
              </div>
            </CardContent>
          </Card>
        </div>

        {/* Active Rollouts */}
        {activeRollouts.length > 0 && (
          <Card className="mb-6" accent="yellow">
            <CardTitle icon={Rocket}>Active Rollouts</CardTitle>
            <CardContent>
              <div className="space-y-4">
                {activeRollouts.map((rollout) => (
                  <RolloutProgressCard
                    key={rollout.id}
                    rollout={rollout}
                    onPause={() => handlePauseRollout(rollout.id)}
                    onResume={() => handleResumeRollout(rollout.id)}
                    onRollback={() => handleRollback(rollout.id)}
                    onClick={() => setSelectedRollout(rollout)}
                  />
                ))}
              </div>
            </CardContent>
          </Card>
        )}

        {/* Rollout History */}
        <Card>
          <CardTitle>Rollout History</CardTitle>
          <CardContent>
            {rollouts.length === 0 ? (
              <div className="text-center py-8 text-theme-muted">
                <Rocket className="w-12 h-12 mx-auto mb-4 opacity-50" />
                <p>No rollouts yet</p>
                <p className="text-sm mt-1">Create a new rollout to update your agent fleet</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Version</TableHead>
                    <TableHead>Strategy</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Progress</TableHead>
                    <TableHead>Started</TableHead>
                    <TableHead></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rollouts.map((rollout) => (
                    <TableRow
                      key={rollout.id}
                      onClick={() => setSelectedRollout(rollout)}
                      className="cursor-pointer"
                    >
                      <TableCell className="font-mono text-theme-primary">
                        {rollout.version}
                      </TableCell>
                      <TableCell className="text-theme-muted capitalize">
                        {rollout.config?.strategy || 'staged'}
                      </TableCell>
                      <TableCell>
                        <StatusBadge
                          status={getStatusType(rollout.status)}
                          label={rollout.status.replace('_', ' ')}
                          size="sm"
                        />
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <div className="w-24 h-2 bg-surface-tertiary rounded-full overflow-hidden">
                            <div
                              className="h-full bg-pilot-cyan"
                              style={{
                                width: `${rollout.agents_total > 0
                                  ? Math.round((rollout.agents_updated / rollout.agents_total) * 100)
                                  : 0}%`
                              }}
                            />
                          </div>
                          <span className="text-sm text-theme-muted">
                            {rollout.agents_updated}/{rollout.agents_total}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell className="text-theme-muted text-sm">
                        {rollout.started_at ? formatRelativeTime(rollout.started_at) : 'Not started'}
                      </TableCell>
                      <TableCell>
                        <ChevronRight className="w-4 h-4 text-theme-muted" />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </PageContent>

      {/* Create Rollout Modal */}
      <CreateRolloutModal
        isOpen={showCreateModal}
        onClose={() => setShowCreateModal(false)}
        releases={releases}
        onSuccess={() => {
          setShowCreateModal(false);
          fetchData();
        }}
      />

      {/* Rollout Detail Modal */}
      {selectedRollout && (
        <RolloutDetailModal
          rollout={selectedRollout}
          onClose={() => setSelectedRollout(null)}
          onPause={() => handlePauseRollout(selectedRollout.id)}
          onResume={() => handleResumeRollout(selectedRollout.id)}
          onRollback={() => handleRollback(selectedRollout.id)}
        />
      )}
    </>
  );
}

function RolloutProgressCard({ rollout, onPause, onResume, onRollback, onClick }) {
  const progress = rollout.agents_total > 0
    ? Math.round((rollout.agents_updated / rollout.agents_total) * 100)
    : 0;

  return (
    <div
      className="p-4 bg-surface-primary rounded-lg cursor-pointer hover:bg-surface-tertiary/50 transition-colors"
      onClick={onClick}
    >
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-3">
          <span className="font-mono text-theme-primary">{rollout.version}</span>
          <StatusBadge
            status={getStatusType(rollout.status)}
            label={rollout.status.replace('_', ' ')}
            size="sm"
          />
        </div>
        <div className="flex gap-2" onClick={(e) => e.stopPropagation()}>
          {rollout.status === 'in_progress' && (
            <Button variant="ghost" size="sm" onClick={onPause}>
              <Pause className="w-4 h-4" />
            </Button>
          )}
          {rollout.status === 'paused' && (
            <Button variant="ghost" size="sm" onClick={onResume}>
              <Play className="w-4 h-4" />
            </Button>
          )}
          {['in_progress', 'paused'].includes(rollout.status) && (
            <Button variant="ghost" size="sm" onClick={onRollback} className="text-pilot-red">
              <RotateCcw className="w-4 h-4" />
            </Button>
          )}
        </div>
      </div>

      <div className="mb-2">
        <div className="h-3 bg-surface-secondary rounded-full overflow-hidden">
          <div
            className="h-full bg-pilot-cyan transition-all duration-500"
            style={{ width: `${progress}%` }}
          />
        </div>
      </div>

      <div className="flex justify-between text-sm text-theme-muted">
        <span>Wave {rollout.current_wave}/{rollout.total_waves}</span>
        <span>
          {rollout.agents_updated} updated, {rollout.agents_pending} pending
          {rollout.agents_failed > 0 && (
            <span className="text-pilot-red"> ({rollout.agents_failed} failed)</span>
          )}
        </span>
      </div>
    </div>
  );
}

function CreateRolloutModal({ isOpen, onClose, releases, onSuccess }) {
  const [releaseId, setReleaseId] = useState('');
  const [strategy, setStrategy] = useState('staged');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!releaseId) {
      setError('Please select a release');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      await endpoints.createRollout({
        release_id: releaseId,
        strategy,
      });
      onSuccess();
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const releaseOptions = releases.map(r => ({
    value: r.id,
    label: `${r.version} (${r.status})`,
  }));

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Create Rollout">
      <form onSubmit={handleSubmit} className="space-y-4">
        {error && (
          <div className="p-3 bg-pilot-red/20 border border-pilot-red rounded-lg text-sm text-pilot-red">
            {error}
          </div>
        )}

        <div>
          <label className="block text-sm font-medium text-theme-secondary mb-2">
            Release
          </label>
          <Select
            options={[{ value: '', label: 'Select a release' }, ...releaseOptions]}
            value={releaseId}
            onChange={setReleaseId}
            className="w-full"
          />
        </div>

        <div>
          <label className="block text-sm font-medium text-theme-secondary mb-2">
            Strategy
          </label>
          <Select
            options={strategyOptions}
            value={strategy}
            onChange={setStrategy}
            className="w-full"
          />
        </div>

        <div className="flex justify-end gap-3 pt-4">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={loading}>
            {loading ? 'Creating...' : 'Start Rollout'}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function RolloutDetailModal({ rollout, onClose, onPause, onResume, onRollback }) {
  const [progress, setProgress] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchProgress = async () => {
      try {
        const res = await endpoints.getRolloutProgress(rollout.id);
        setProgress(res.agents || []);
      } catch (err) {
        console.error('Failed to fetch progress:', err);
      } finally {
        setLoading(false);
      }
    };

    fetchProgress();
    const interval = setInterval(fetchProgress, 5000);
    return () => clearInterval(interval);
  }, [rollout.id]);

  const progressPercent = rollout.agents_total > 0
    ? Math.round((rollout.agents_updated / rollout.agents_total) * 100)
    : 0;

  return (
    <Modal isOpen={true} onClose={onClose} title={`Rollout: ${rollout.version}`} size="lg">
      <div className="space-y-6">
        {/* Status and Actions */}
        <div className="flex items-center justify-between">
          <StatusBadge
            status={getStatusType(rollout.status)}
            label={rollout.status.replace('_', ' ')}
          />
          <div className="flex gap-2">
            {rollout.status === 'in_progress' && (
              <Button variant="secondary" size="sm" onClick={onPause}>
                <Pause className="w-4 h-4 mr-2" />
                Pause
              </Button>
            )}
            {rollout.status === 'paused' && (
              <Button variant="secondary" size="sm" onClick={onResume}>
                <Play className="w-4 h-4 mr-2" />
                Resume
              </Button>
            )}
            {['in_progress', 'paused'].includes(rollout.status) && (
              <Button variant="secondary" size="sm" onClick={onRollback} className="text-pilot-red">
                <RotateCcw className="w-4 h-4 mr-2" />
                Rollback
              </Button>
            )}
          </div>
        </div>

        {/* Progress Bar */}
        <div>
          <div className="flex justify-between text-sm mb-2">
            <span className="text-theme-muted">Overall Progress</span>
            <span className="text-theme-primary">{progressPercent}%</span>
          </div>
          <div className="h-4 bg-surface-secondary rounded-full overflow-hidden">
            <div
              className="h-full bg-pilot-cyan transition-all duration-500"
              style={{ width: `${progressPercent}%` }}
            />
          </div>
        </div>

        {/* Stats */}
        <div className="grid grid-cols-4 gap-4">
          <div className="text-center p-3 bg-surface-primary rounded-lg">
            <div className="text-2xl font-bold text-theme-primary">{rollout.agents_updated}</div>
            <div className="text-xs text-theme-muted">Updated</div>
          </div>
          <div className="text-center p-3 bg-surface-primary rounded-lg">
            <div className="text-2xl font-bold text-accent">{rollout.agents_updating}</div>
            <div className="text-xs text-theme-muted">Updating</div>
          </div>
          <div className="text-center p-3 bg-surface-primary rounded-lg">
            <div className="text-2xl font-bold text-theme-muted">{rollout.agents_pending}</div>
            <div className="text-xs text-theme-muted">Pending</div>
          </div>
          <div className="text-center p-3 bg-surface-primary rounded-lg">
            <div className="text-2xl font-bold text-pilot-red">{rollout.agents_failed}</div>
            <div className="text-xs text-theme-muted">Failed</div>
          </div>
        </div>

        {/* Agent Progress List */}
        <div>
          <h4 className="text-sm font-medium text-theme-secondary mb-3">Agent Progress</h4>
          <div className="max-h-64 overflow-y-auto">
            {loading ? (
              <div className="text-center py-4 text-theme-muted">Loading...</div>
            ) : progress.length === 0 ? (
              <div className="text-center py-4 text-theme-muted">No agents in this rollout</div>
            ) : (
              <div className="space-y-2">
                {progress.map((agent) => (
                  <div
                    key={agent.agent_id}
                    className="flex items-center justify-between p-3 bg-surface-primary rounded-lg"
                  >
                    <div>
                      <div className="text-sm text-theme-primary">{agent.agent_name}</div>
                      <div className="text-xs text-theme-muted">
                        {agent.from_version} → {agent.to_version}
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      {agent.status === 'updated' && (
                        <CheckCircle2 className="w-4 h-4 text-status-healthy" />
                      )}
                      {agent.status === 'updating' && (
                        <RefreshCw className="w-4 h-4 text-accent animate-spin" />
                      )}
                      {agent.status === 'pending' && (
                        <Clock className="w-4 h-4 text-theme-muted" />
                      )}
                      {agent.status === 'failed' && (
                        <AlertTriangle className="w-4 h-4 text-pilot-red" />
                      )}
                      <span className={`text-sm capitalize ${
                        agent.status === 'updated' ? 'text-status-healthy' :
                        agent.status === 'failed' ? 'text-pilot-red' :
                        agent.status === 'updating' ? 'text-accent' :
                        'text-theme-muted'
                      }`}>
                        {agent.status}
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Error Info */}
        {rollout.last_error && (
          <div className="p-3 bg-pilot-red/20 border border-pilot-red rounded-lg">
            <div className="text-sm font-medium text-pilot-red mb-1">Error</div>
            <div className="text-sm text-theme-secondary">{rollout.last_error}</div>
          </div>
        )}
      </div>
    </Modal>
  );
}

function getStatusType(status) {
  switch (status) {
    case 'completed':
      return 'healthy';
    case 'failed':
    case 'rolled_back':
      return 'down';
    case 'in_progress':
    case 'paused':
      return 'degraded';
    default:
      return 'unknown';
  }
}
