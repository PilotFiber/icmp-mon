import { useState, useEffect, useMemo } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import {
  Network,
  RefreshCw,
  AlertTriangle,
  Server,
  Target,
  Archive,
  History,
  Clock,
  ArrowRight,
  ArrowLeft,
  MapPin,
  Building2,
} from 'lucide-react';

import { PageHeader, PageContent } from '../components/Layout';
import { Card } from '../components/Card';
import { StatusBadge, StatusDot } from '../components/StatusBadge';
import { Button } from '../components/Button';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '../components/Table';
import { formatRelativeTime } from '../lib/utils';
import { endpoints } from '../lib/api';

// Monitoring state color mapping
const stateColors = {
  active: 'healthy',
  degraded: 'degraded',    // Responding but with packet loss/latency issues (alertable)
  down: 'down',            // Was responding, now not responding (alertable)
  excluded: 'inactive',    // Permanently excluded from monitoring
  unresponsive: 'inactive', // Never responded to ICMP (not alertable, gray)
  unknown: 'unknown',
  inactive: 'unknown',
};

const stateLabels = {
  active: 'Active',
  degraded: 'Degraded',            // Packet loss or latency issues (alertable)
  down: 'Down',                    // Alertable outage - was responding, now not
  excluded: 'Excluded',
  unresponsive: 'Never Responded', // Not alertable - never established a baseline
  unknown: 'Unknown',
  inactive: 'Inactive',
};

// Activity log event type badges
const eventTypeStyles = {
  created: { bg: 'bg-status-healthy/20', text: 'text-status-healthy' },
  updated: { bg: 'bg-pilot-cyan/20', text: 'text-pilot-cyan' },
  archived: { bg: 'bg-pilot-red/20', text: 'text-pilot-red' },
  state_change: { bg: 'bg-accent/20', text: 'text-accent' },
  transitioned: { bg: 'bg-accent/20', text: 'text-accent' },
};

const severityColors = {
  info: 'text-theme-muted',
  warning: 'text-accent',
  error: 'text-pilot-red',
};

// Helper to format subnet CIDR (handles case where network_address may already include prefix)
function formatSubnetCIDR(subnet) {
  if (!subnet) return '';
  const addr = subnet.network_address || '';
  // If address already contains a slash, use it as-is
  if (addr.includes('/')) return addr;
  // Otherwise append the network size
  return subnet.network_size ? `${addr}/${subnet.network_size}` : addr;
}

function ActivityLog({ activity, loading }) {
  if (loading) {
    return (
      <div className="flex items-center justify-center py-8">
        <RefreshCw className="w-5 h-5 animate-spin text-theme-muted" />
      </div>
    );
  }

  if (!activity || activity.length === 0) {
    return (
      <div className="text-center py-8 text-theme-muted">
        <History className="w-8 h-8 mx-auto mb-2 opacity-50" />
        <p className="text-sm">No activity recorded yet</p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {activity.map(entry => {
        const style = eventTypeStyles[entry.event_type] || { bg: 'bg-surface-tertiary', text: 'text-theme-muted' };
        return (
          <div
            key={entry.id}
            className="flex items-start gap-3 p-3 bg-surface-primary rounded-lg"
          >
            <div className="flex-shrink-0 mt-0.5">
              <Clock className={`w-4 h-4 ${severityColors[entry.severity] || 'text-theme-muted'}`} />
            </div>
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2 flex-wrap">
                <span className={`px-2 py-0.5 rounded text-xs font-medium ${style.bg} ${style.text}`}>
                  {entry.event_type?.replace(/_/g, ' ')}
                </span>
                <span className="text-xs text-theme-muted">
                  {formatRelativeTime(entry.created_at)}
                </span>
                {entry.triggered_by && (
                  <span className="text-xs text-theme-muted">
                    by {entry.triggered_by}
                  </span>
                )}
              </div>
              <div className="mt-1 text-sm text-theme-secondary">
                {entry.ip && (
                  <span className="font-mono mr-2">{entry.ip}</span>
                )}
                {entry.details && typeof entry.details === 'object' && (
                  <span>
                    {entry.details.from_state && entry.details.to_state && (
                      <span className="inline-flex items-center gap-1">
                        <span className="text-theme-muted">{entry.details.from_state}</span>
                        <ArrowRight className="w-3 h-3" />
                        <span>{entry.details.to_state}</span>
                      </span>
                    )}
                    {entry.details.reason && (
                      <span className="text-theme-muted ml-2">({entry.details.reason})</span>
                    )}
                  </span>
                )}
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function TargetsList({ targets, loading }) {
  if (loading) {
    return (
      <div className="flex items-center justify-center py-8">
        <RefreshCw className="w-5 h-5 animate-spin text-theme-muted" />
      </div>
    );
  }

  if (!targets || targets.length === 0) {
    return (
      <div className="text-center py-8 text-theme-muted">
        <Target className="w-8 h-8 mx-auto mb-2 opacity-50" />
        <p className="text-sm">No targets in this subnet</p>
      </div>
    );
  }

  // Sort targets: active first, then down, then others
  const stateOrder = ['active', 'degraded', 'down', 'unknown', 'unresponsive', 'excluded', 'inactive'];
  const sortedTargets = [...targets].sort((a, b) => {
    const aState = a.monitoring_state || 'unknown';
    const bState = b.monitoring_state || 'unknown';
    const aOrder = stateOrder.indexOf(aState);
    const bOrder = stateOrder.indexOf(bState);
    if (aOrder !== bOrder) return aOrder - bOrder;
    // Secondary sort by IP
    return a.ip.localeCompare(b.ip, undefined, { numeric: true });
  });

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>IP Address</TableHead>
          <TableHead>State</TableHead>
          <TableHead>Type</TableHead>
          <TableHead>Ownership</TableHead>
          <TableHead>Last Seen</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {sortedTargets.map(target => {
          const state = target.monitoring_state || 'unknown';
          return (
            <TableRow key={target.id}>
              <TableCell>
                <Link to={`/targets/${target.id}`} className="flex items-center gap-2 hover:text-pilot-cyan">
                  <StatusDot status={stateColors[state]} pulse={state === 'down'} />
                  <span className="font-mono text-theme-primary hover:text-pilot-cyan">{target.ip}</span>
                </Link>
              </TableCell>
              <TableCell>
                <StatusBadge status={stateColors[state]} label={stateLabels[state]} size="sm" />
              </TableCell>
              <TableCell className="text-theme-secondary text-sm">
                {target.ip_type || 'customer'}
              </TableCell>
              <TableCell className="text-theme-secondary text-sm">
                {target.ownership || 'auto'}
              </TableCell>
              <TableCell className="text-theme-muted text-sm">
                {target.last_response_at ? formatRelativeTime(target.last_response_at) : '—'}
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}

export function SubnetDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [subnet, setSubnet] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [activeTab, setActiveTab] = useState('targets');
  const [targets, setTargets] = useState([]);
  const [targetsLoading, setTargetsLoading] = useState(false);
  const [activity, setActivity] = useState([]);
  const [activityLoading, setActivityLoading] = useState(false);

  const fetchSubnet = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await endpoints.getSubnet(id);
      setSubnet(data);
    } catch (err) {
      console.error('Failed to fetch subnet:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const fetchTargets = async () => {
    setTargetsLoading(true);
    try {
      const res = await endpoints.getSubnetTargets(id);
      setTargets(res.targets || []);
    } catch (err) {
      console.error('Failed to fetch targets:', err);
      setTargets([]);
    } finally {
      setTargetsLoading(false);
    }
  };

  const fetchActivity = async () => {
    setActivityLoading(true);
    try {
      const res = await endpoints.getSubnetActivity(id, 100);
      setActivity(res.activity || []);
    } catch (err) {
      console.error('Failed to fetch activity:', err);
      setActivity([]);
    } finally {
      setActivityLoading(false);
    }
  };

  useEffect(() => {
    fetchSubnet();
    fetchTargets();
    fetchActivity();
  }, [id]);

  // Calculate target stats
  const targetStats = useMemo(() => {
    if (!targets.length) return null;
    const counts = targets.reduce((acc, t) => {
      const state = t.monitoring_state || 'unknown';
      acc[state] = (acc[state] || 0) + 1;
      acc.total++;
      return acc;
    }, { total: 0 });
    return counts;
  }, [targets]);

  // Find gateway target from targets list
  const gatewayTarget = useMemo(() => {
    if (!subnet?.gateway_address || !targets.length) return null;
    return targets.find(t => t.ip === subnet.gateway_address);
  }, [subnet?.gateway_address, targets]);

  if (loading) {
    return (
      <>
        <PageHeader
          title="Loading..."
          breadcrumbs={[
            { label: 'Subnets', href: '/subnets' },
            { label: '...' },
          ]}
        />
        <PageContent>
          <div className="flex items-center justify-center py-12">
            <RefreshCw className="w-6 h-6 animate-spin text-theme-muted" />
          </div>
        </PageContent>
      </>
    );
  }

  if (error || !subnet) {
    return (
      <>
        <PageHeader
          title="Subnet Not Found"
          breadcrumbs={[
            { label: 'Subnets', href: '/subnets' },
            { label: 'Error' },
          ]}
        />
        <PageContent>
          <Card accent="red">
            <div className="flex items-center gap-3">
              <AlertTriangle className="w-6 h-6 text-pilot-red" />
              <div>
                <h3 className="font-medium text-theme-primary">Failed to load subnet</h3>
                <p className="text-sm text-theme-muted">{error || 'Subnet not found'}</p>
              </div>
              <Button variant="secondary" size="sm" onClick={() => navigate('/subnets')} className="ml-auto">
                Back to Subnets
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
        title={formatSubnetCIDR(subnet)}
        breadcrumbs={[
          { label: 'Subnets', href: '/subnets' },
          { label: formatSubnetCIDR(subnet) },
        ]}
        actions={
          <div className="flex gap-3">
            <Button variant="secondary" onClick={() => { fetchSubnet(); fetchTargets(); }} className="gap-2">
              <RefreshCw className="w-4 h-4" />
              Refresh
            </Button>
          </div>
        }
      />

      <PageContent>
        {/* Status Banner */}
        {subnet.archived_at && (
          <Card accent="red" className="mb-6">
            <div className="flex items-center gap-3">
              <Archive className="w-5 h-5 text-pilot-red" />
              <div>
                <span className="font-medium text-pilot-red">This subnet is archived</span>
                <span className="text-theme-muted ml-2">
                  {formatRelativeTime(subnet.archived_at)}
                  {subnet.archive_reason && ` - ${subnet.archive_reason}`}
                </span>
              </div>
            </div>
          </Card>
        )}

        {/* Subnet Info */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-6">
          {/* Main Info Card */}
          <Card className="lg:col-span-2">
            <div className="flex items-start justify-between mb-4">
              <div>
                <div className="flex items-center gap-3 mb-1">
                  <Network className="w-6 h-6 text-pilot-cyan" />
                  <h2 className="text-xl font-mono font-semibold text-theme-primary">
                    {formatSubnetCIDR(subnet)}
                  </h2>
                  {subnet.archived_at ? (
                    <StatusBadge status="down" label="Archived" />
                  ) : (
                    <StatusBadge status="healthy" label="Active" />
                  )}
                </div>
                <p className="text-theme-muted flex items-center gap-4">
                  <span className="flex items-center gap-1">
                    <Building2 className="w-4 h-4" />
                    {subnet.subscriber_name || 'No subscriber'}
                  </span>
                  <span className="flex items-center gap-1">
                    <MapPin className="w-4 h-4" />
                    {subnet.pop_name || 'Unknown POP'}
                  </span>
                </p>
              </div>
            </div>

            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div className="bg-surface-primary rounded-lg p-3">
                <div className="text-xs text-theme-muted mb-1">Gateway</div>
                <div className="flex items-center gap-2">
                  {gatewayTarget && (
                    <StatusDot status={stateColors[gatewayTarget.monitoring_state || 'unknown']} />
                  )}
                  {gatewayTarget ? (
                    <Link to={`/targets/${gatewayTarget.id}`} className="font-mono text-theme-primary hover:text-pilot-cyan">
                      {subnet.gateway_address}
                    </Link>
                  ) : (
                    <span className="font-mono text-theme-primary">{subnet.gateway_address || '—'}</span>
                  )}
                </div>
                <div className="text-xs text-theme-muted mt-1">{subnet.gateway_device || '—'}</div>
              </div>
              <div className="bg-surface-primary rounded-lg p-3">
                <div className="text-xs text-theme-muted mb-1">IP Range</div>
                <div className="font-mono text-sm text-theme-primary">
                  {subnet.first_usable_address || '—'}
                </div>
                <div className="font-mono text-sm text-theme-primary">
                  {subnet.last_usable_address || '—'}
                </div>
              </div>
              <div className="bg-surface-primary rounded-lg p-3">
                <div className="text-xs text-theme-muted mb-1">Location</div>
                <div className="text-theme-primary">{subnet.city || '—'}</div>
                <div className="text-xs text-theme-muted">{subnet.region || '—'}</div>
              </div>
              <div className="bg-surface-primary rounded-lg p-3">
                <div className="text-xs text-theme-muted mb-1">IDs</div>
                <div className="text-xs space-y-1">
                  {subnet.pilot_subnet_id && (
                    <div><span className="text-theme-muted">Pilot:</span> <span className="text-theme-primary">{subnet.pilot_subnet_id}</span></div>
                  )}
                  {subnet.vlan_id && (
                    <div><span className="text-theme-muted">VLAN:</span> <span className="text-theme-primary">{subnet.vlan_id}</span></div>
                  )}
                  {subnet.service_id && (
                    <div><span className="text-theme-muted">Service:</span> <span className="text-theme-primary">{subnet.service_id}</span></div>
                  )}
                  {!subnet.pilot_subnet_id && !subnet.vlan_id && !subnet.service_id && (
                    <div className="text-theme-muted">—</div>
                  )}
                </div>
              </div>
            </div>
          </Card>

          {/* Target Stats Card */}
          <Card>
            <h3 className="text-sm font-medium text-theme-muted mb-4 flex items-center gap-2">
              <Target className="w-4 h-4" />
              Target Summary
            </h3>
            {targetStats ? (
              <div className="space-y-3">
                <div className="flex items-center justify-between text-lg">
                  <span className="text-theme-muted">Total</span>
                  <span className="font-semibold text-theme-primary">{targetStats.total}</span>
                </div>
                <div className="border-t border-theme pt-3 space-y-2">
                  {Object.entries(targetStats).filter(([k]) => k !== 'total').map(([state, count]) => (
                    <div key={state} className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        <StatusDot status={stateColors[state]} />
                        <span className="text-sm text-theme-secondary">{stateLabels[state]}</span>
                      </div>
                      <span className="text-sm font-medium text-theme-primary">{count}</span>
                    </div>
                  ))}
                </div>
              </div>
            ) : (
              <div className="text-center py-4 text-theme-muted">
                <p className="text-sm">No targets</p>
              </div>
            )}
          </Card>
        </div>

        {/* Tabs */}
        <Card>
          <div className="flex gap-4 border-b border-theme -mx-6 px-6">
            <button
              onClick={() => setActiveTab('targets')}
              className={`flex items-center gap-2 px-4 py-3 text-sm font-medium transition-colors border-b-2 -mb-px ${
                activeTab === 'targets'
                  ? 'border-pilot-cyan text-pilot-cyan'
                  : 'border-transparent text-theme-muted hover:text-theme-primary'
              }`}
            >
              <Target className="w-4 h-4" />
              Targets ({targets.length})
            </button>
            <button
              onClick={() => setActiveTab('activity')}
              className={`flex items-center gap-2 px-4 py-3 text-sm font-medium transition-colors border-b-2 -mb-px ${
                activeTab === 'activity'
                  ? 'border-pilot-cyan text-pilot-cyan'
                  : 'border-transparent text-theme-muted hover:text-theme-primary'
              }`}
            >
              <History className="w-4 h-4" />
              Activity ({activity.length})
            </button>
          </div>

          <div className="pt-6">
            {activeTab === 'targets' && (
              <TargetsList targets={targets} loading={targetsLoading} />
            )}
            {activeTab === 'activity' && (
              <ActivityLog activity={activity} loading={activityLoading} />
            )}
          </div>
        </Card>
      </PageContent>
    </>
  );
}
