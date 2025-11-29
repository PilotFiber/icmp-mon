import { useState, useEffect, useMemo, useCallback } from 'react';
import { Link } from 'react-router-dom';
import {
  Bell,
  Filter,
  CheckCircle,
  Clock,
  AlertTriangle,
  XCircle,
  ChevronRight,
  RefreshCw,
  Loader2,
  MapPin,
  Server,
  Building2,
  Network,
} from 'lucide-react';

import { PageHeader, PageContent } from '../components/Layout';
import { Card, CardTitle, CardContent } from '../components/Card';
import { MetricCard } from '../components/MetricCard';
import { Button } from '../components/Button';
import { SearchInput, Select } from '../components/Input';
import { AlertCard } from '../components/Alert';
import { formatRelativeTime } from '../lib/utils';

const API_BASE = '/api/v1';

const severities = [
  { value: '', label: 'All Severities' },
  { value: 'critical', label: 'Critical' },
  { value: 'warning', label: 'Warning' },
  { value: 'info', label: 'Info' },
];

const alertStatuses = [
  { value: '', label: 'All Statuses' },
  { value: 'active', label: 'Active' },
  { value: 'acknowledged', label: 'Acknowledged' },
  { value: 'resolved', label: 'Resolved' },
];

export function Alerts() {
  const [search, setSearch] = useState('');
  const [severityFilter, setSeverityFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [alerts, setAlerts] = useState([]);
  const [stats, setStats] = useState({ active: 0, acknowledged: 0, critical: 0, total: 0 });
  const [correlations, setCorrelations] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [acknowledging, setAcknowledging] = useState(null);

  const fetchAlerts = useCallback(async () => {
    try {
      setError(null);
      const params = new URLSearchParams();
      if (severityFilter) params.set('severity', severityFilter);
      if (statusFilter) params.set('status', statusFilter);
      params.set('limit', '100');

      const url = `${API_BASE}/alerts${params.toString() ? '?' + params.toString() : ''}`;
      const response = await fetch(url);
      if (!response.ok) {
        throw new Error(`Failed to fetch alerts: ${response.statusText}`);
      }
      const data = await response.json();
      setAlerts(data.alerts || []);
    } catch (err) {
      console.error('Error fetching alerts:', err);
      setError(err.message);
    }
  }, [severityFilter, statusFilter]);

  const fetchStats = useCallback(async () => {
    try {
      const response = await fetch(`${API_BASE}/alerts/stats`);
      if (!response.ok) {
        throw new Error(`Failed to fetch stats: ${response.statusText}`);
      }
      const data = await response.json();
      setStats({
        active: data.active_count || 0,
        acknowledged: data.acknowledged_count || 0,
        critical: data.critical_count || 0,
        total: data.total_this_week_count || 0,
      });
    } catch (err) {
      console.error('Error fetching stats:', err);
    }
  }, []);

  const fetchCorrelations = useCallback(async () => {
    try {
      const response = await fetch(`${API_BASE}/alerts/correlations`);
      if (!response.ok) {
        throw new Error(`Failed to fetch correlations: ${response.statusText}`);
      }
      const data = await response.json();
      setCorrelations(data);
    } catch (err) {
      console.error('Error fetching correlations:', err);
    }
  }, []);

  useEffect(() => {
    const loadData = async () => {
      setLoading(true);
      await Promise.all([fetchAlerts(), fetchStats(), fetchCorrelations()]);
      setLoading(false);
    };
    loadData();
  }, [fetchAlerts, fetchStats, fetchCorrelations]);

  // Auto-refresh every 15 seconds
  useEffect(() => {
    const interval = setInterval(() => {
      fetchAlerts();
      fetchStats();
      fetchCorrelations();
    }, 15000);
    return () => clearInterval(interval);
  }, [fetchAlerts, fetchStats, fetchCorrelations]);

  const filteredAlerts = useMemo(() => {
    if (!search) return alerts;

    const searchLower = search.toLowerCase();
    return alerts.filter((alert) => {
      const matchesTitle = alert.title?.toLowerCase().includes(searchLower);
      const matchesTarget = alert.target_id?.includes(search) || alert.target_name?.toLowerCase().includes(searchLower);
      const matchesDescription = alert.description?.toLowerCase().includes(searchLower);
      const matchesAgent = alert.agent_name?.toLowerCase().includes(searchLower);
      const matchesSubscriber = alert.subscriber_name?.toLowerCase().includes(searchLower);
      const matchesPop = alert.pop_name?.toLowerCase().includes(searchLower);
      const matchesGateway = alert.gateway_device?.toLowerCase().includes(searchLower);
      const matchesLocation = alert.location_address?.toLowerCase().includes(searchLower);
      const matchesCity = alert.city?.toLowerCase().includes(searchLower);
      const matchesRegion = alert.region?.toLowerCase().includes(searchLower);
      return matchesTitle || matchesTarget || matchesDescription || matchesAgent ||
             matchesSubscriber || matchesPop || matchesGateway || matchesLocation ||
             matchesCity || matchesRegion;
    });
  }, [alerts, search]);

  const handleAcknowledge = async (alertId) => {
    setAcknowledging(alertId);
    try {
      const response = await fetch(`${API_BASE}/alerts/${alertId}/acknowledge`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ acknowledged_by: 'ui_user' }),
      });
      if (!response.ok) {
        throw new Error('Failed to acknowledge alert');
      }
      // Refresh alerts after acknowledge
      await fetchAlerts();
      await fetchStats();
    } catch (err) {
      console.error('Error acknowledging alert:', err);
      setError(err.message);
    } finally {
      setAcknowledging(null);
    }
  };

  const handleResolve = async (alertId) => {
    try {
      const response = await fetch(`${API_BASE}/alerts/${alertId}/resolve`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ reason: 'Manually resolved via UI' }),
      });
      if (!response.ok) {
        throw new Error('Failed to resolve alert');
      }
      await fetchAlerts();
      await fetchStats();
    } catch (err) {
      console.error('Error resolving alert:', err);
      setError(err.message);
    }
  };

  const handleRefresh = () => {
    setLoading(true);
    Promise.all([fetchAlerts(), fetchStats(), fetchCorrelations()]).finally(() => setLoading(false));
  };

  // Map API severity to UI severity
  const mapSeverity = (severity) => {
    const mapping = {
      critical: 'critical',
      major: 'critical',
      minor: 'warning',
      warning: 'warning',
      info: 'info',
    };
    return mapping[severity] || 'info';
  };

  // Map API status to display values
  const isAcknowledged = (alert) => alert.status === 'acknowledged';
  const isResolved = (alert) => alert.status === 'resolved';

  return (
    <>
      <PageHeader
        title="Alerts"
        description="Active and historical alert notifications"
        actions={
          <div className="flex gap-2">
            <Button variant="ghost" size="sm" onClick={handleRefresh} disabled={loading}>
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
            </Button>
            <Button variant="secondary" className="gap-2">
              <Bell className="w-4 h-4" />
              Configure Rules
            </Button>
          </div>
        }
      />

      <PageContent>
        {error && (
          <div className="mb-4 p-4 bg-pilot-red/20 border border-pilot-red rounded-lg text-pilot-red">
            {error}
            <button onClick={() => setError(null)} className="ml-4 underline">Dismiss</button>
          </div>
        )}

        {/* Stats */}
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
          <MetricCard
            title="Active Alerts"
            value={stats.active}
            icon={AlertTriangle}
            status={stats.active > 0 ? 'degraded' : 'healthy'}
          />
          <MetricCard
            title="Critical"
            value={stats.critical}
            status={stats.critical > 0 ? 'down' : 'healthy'}
          />
          <MetricCard
            title="Acknowledged"
            value={stats.acknowledged}
            icon={CheckCircle}
          />
          <MetricCard
            title="Total"
            value={stats.total}
            icon={Clock}
          />
        </div>

        {/* Correlation Summary - Heat Map */}
        {correlations && correlations.total_active_alerts > 0 && (
          <Card className="mb-6">
            <div className="flex items-center gap-2 mb-3">
              <Network className="w-5 h-5 text-pilot-cyan" />
              <h3 className="font-medium text-theme-primary">Alert Patterns</h3>
              <span className="text-xs text-theme-muted">
                {correlations.total_active_alerts} active alerts
              </span>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {/* POP Correlations */}
              {correlations.by_pop?.length > 0 && (
                <div className="space-y-2">
                  <div className="text-xs text-theme-muted uppercase tracking-wide">By POP</div>
                  {correlations.by_pop.map((item) => (
                    <div
                      key={item.value}
                      className={`flex items-center justify-between px-3 py-2 rounded-lg cursor-pointer hover:bg-neutral-700/30 ${
                        item.severity === 'critical' ? 'bg-pilot-red/10 border border-pilot-red/30' :
                        item.severity === 'warning' ? 'bg-warning/10 border border-warning/30' :
                        'bg-neutral-700/20'
                      }`}
                      onClick={() => setSearch(item.value)}
                    >
                      <span className="flex items-center gap-2">
                        <Network className="w-3 h-3 text-pilot-cyan" />
                        <span className="text-sm text-theme-primary">{item.value}</span>
                      </span>
                      <span className={`text-sm font-medium ${
                        item.severity === 'critical' ? 'text-pilot-red' :
                        item.severity === 'warning' ? 'text-warning' :
                        'text-theme-secondary'
                      }`}>
                        {item.alert_count} alerts
                      </span>
                    </div>
                  ))}
                </div>
              )}

              {/* Gateway Correlations */}
              {correlations.by_gateway?.length > 0 && (
                <div className="space-y-2">
                  <div className="text-xs text-theme-muted uppercase tracking-wide">By Gateway</div>
                  {correlations.by_gateway.map((item) => (
                    <div
                      key={item.value}
                      className={`flex items-center justify-between px-3 py-2 rounded-lg cursor-pointer hover:bg-neutral-700/30 ${
                        item.severity === 'critical' ? 'bg-pilot-red/10 border border-pilot-red/30' :
                        item.severity === 'warning' ? 'bg-warning/10 border border-warning/30' :
                        'bg-neutral-700/20'
                      }`}
                      onClick={() => setSearch(item.value)}
                    >
                      <span className="flex items-center gap-2">
                        <Server className="w-3 h-3 text-theme-secondary" />
                        <span className="text-sm text-theme-primary truncate max-w-[200px]">{item.value}</span>
                      </span>
                      <span className={`text-sm font-medium ${
                        item.severity === 'critical' ? 'text-pilot-red' :
                        item.severity === 'warning' ? 'text-warning' :
                        'text-theme-secondary'
                      }`}>
                        {item.alert_count}
                      </span>
                    </div>
                  ))}
                </div>
              )}

              {/* Subscriber Correlations */}
              {correlations.by_subscriber?.length > 0 && (
                <div className="space-y-2">
                  <div className="text-xs text-theme-muted uppercase tracking-wide">By Subscriber</div>
                  {correlations.by_subscriber.map((item) => (
                    <div
                      key={item.value}
                      className={`flex items-center justify-between px-3 py-2 rounded-lg cursor-pointer hover:bg-neutral-700/30 ${
                        item.severity === 'critical' ? 'bg-pilot-red/10 border border-pilot-red/30' :
                        item.severity === 'warning' ? 'bg-warning/10 border border-warning/30' :
                        'bg-neutral-700/20'
                      }`}
                      onClick={() => setSearch(item.value)}
                    >
                      <span className="flex items-center gap-2">
                        <Building2 className="w-3 h-3 text-pilot-purple" />
                        <span className="text-sm text-theme-primary truncate max-w-[200px]">{item.value}</span>
                      </span>
                      <span className={`text-sm font-medium ${
                        item.severity === 'critical' ? 'text-pilot-red' :
                        item.severity === 'warning' ? 'text-warning' :
                        'text-theme-secondary'
                      }`}>
                        {item.alert_count}
                      </span>
                    </div>
                  ))}
                </div>
              )}

              {/* Location Correlations */}
              {correlations.by_location?.length > 0 && (
                <div className="space-y-2">
                  <div className="text-xs text-theme-muted uppercase tracking-wide">By Location</div>
                  {correlations.by_location.map((item) => (
                    <div
                      key={item.value}
                      className={`flex items-center justify-between px-3 py-2 rounded-lg cursor-pointer hover:bg-neutral-700/30 ${
                        item.severity === 'critical' ? 'bg-pilot-red/10 border border-pilot-red/30' :
                        item.severity === 'warning' ? 'bg-warning/10 border border-warning/30' :
                        'bg-neutral-700/20'
                      }`}
                      onClick={() => setSearch(item.value.split(',')[0])}
                    >
                      <span className="flex items-center gap-2">
                        <MapPin className="w-3 h-3 text-theme-secondary" />
                        <span className="text-sm text-theme-primary truncate max-w-[200px]">{item.value}</span>
                      </span>
                      <span className={`text-sm font-medium ${
                        item.severity === 'critical' ? 'text-pilot-red' :
                        item.severity === 'warning' ? 'text-warning' :
                        'text-theme-secondary'
                      }`}>
                        {item.alert_count}
                      </span>
                    </div>
                  ))}
                </div>
              )}

              {/* Region Correlations */}
              {correlations.by_region?.length > 0 && (
                <div className="space-y-2">
                  <div className="text-xs text-theme-muted uppercase tracking-wide">By Region</div>
                  {correlations.by_region.map((item) => (
                    <div
                      key={item.value}
                      className={`flex items-center justify-between px-3 py-2 rounded-lg cursor-pointer hover:bg-neutral-700/30 ${
                        item.severity === 'critical' ? 'bg-pilot-red/10 border border-pilot-red/30' :
                        item.severity === 'warning' ? 'bg-warning/10 border border-warning/30' :
                        'bg-neutral-700/20'
                      }`}
                      onClick={() => setSearch(item.value)}
                    >
                      <span className="flex items-center gap-2">
                        <MapPin className="w-3 h-3 text-theme-secondary" />
                        <span className="text-sm text-theme-primary">{item.value}</span>
                      </span>
                      <span className={`text-sm font-medium ${
                        item.severity === 'critical' ? 'text-pilot-red' :
                        item.severity === 'warning' ? 'text-warning' :
                        'text-theme-secondary'
                      }`}>
                        {item.alert_count}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </Card>
        )}

        {/* Filters */}
        <Card className="mb-6">
          <div className="flex flex-wrap gap-4 items-center">
            <SearchInput
              value={search}
              onChange={setSearch}
              placeholder="Search alerts..."
              className="w-72"
            />
            <Select
              options={severities}
              value={severityFilter}
              onChange={setSeverityFilter}
              className="w-40"
            />
            <Select
              options={alertStatuses}
              value={statusFilter}
              onChange={setStatusFilter}
              className="w-40"
            />
            <div className="flex-1" />
            <Button variant="ghost" size="sm" disabled>
              Acknowledge All
            </Button>
          </div>
        </Card>

        {/* Loading State */}
        {loading && alerts.length === 0 && (
          <Card>
            <div className="text-center py-12 text-theme-muted">
              <Loader2 className="w-12 h-12 mx-auto mb-4 animate-spin" />
              <p>Loading alerts...</p>
            </div>
          </Card>
        )}

        {/* Alert List */}
        {!loading || alerts.length > 0 ? (
          <div className="space-y-3">
            {filteredAlerts.map((alert) => {
              const severity = mapSeverity(alert.severity);
              const acknowledged = isAcknowledged(alert);
              const resolved = isResolved(alert);

              return (
                <Card
                  key={alert.id}
                  className={`
                    ${resolved ? 'opacity-60' : ''}
                    ${acknowledged && !resolved ? 'border-l-4 border-l-pilot-cyan' : ''}
                  `}
                >
                  <div className="flex items-start gap-4">
                    <div className={`
                      p-2 rounded-lg
                      ${severity === 'critical' ? 'bg-pilot-red/20' : ''}
                      ${severity === 'warning' ? 'bg-warning/20' : ''}
                      ${severity === 'info' ? 'bg-pilot-cyan/20' : ''}
                    `}>
                      {severity === 'critical' && <XCircle className="w-5 h-5 text-pilot-red" />}
                      {severity === 'warning' && <AlertTriangle className="w-5 h-5 text-warning" />}
                      {severity === 'info' && <Bell className="w-5 h-5 text-pilot-cyan" />}
                    </div>

                    <div className="flex-1">
                      <div className="flex items-start justify-between">
                        <div>
                          <div className="flex items-center gap-2 mb-1">
                            <span className={`
                              px-2 py-0.5 rounded text-xs font-medium uppercase
                              ${severity === 'critical' ? 'bg-pilot-red text-theme-primary' : ''}
                              ${severity === 'warning' ? 'bg-warning text-neutral-900' : ''}
                              ${severity === 'info' ? 'bg-pilot-cyan text-neutral-900' : ''}
                            `}>
                              {alert.severity}
                            </span>
                            <span className="text-xs text-theme-muted capitalize">
                              {alert.alert_type?.replace('_', ' ')}
                            </span>
                            {resolved && (
                              <span className="px-2 py-0.5 rounded text-xs bg-status-healthy/20 text-status-healthy">
                                Resolved
                              </span>
                            )}
                            {acknowledged && !resolved && (
                              <span className="px-2 py-0.5 rounded text-xs bg-pilot-cyan/20 text-pilot-cyan">
                                Acknowledged
                              </span>
                            )}
                            {alert.incident_id && (
                              <span className="px-2 py-0.5 rounded text-xs bg-pilot-purple/20 text-pilot-purple">
                                In Incident
                              </span>
                            )}
                          </div>
                          <h3 className="font-medium text-theme-primary">{alert.title}</h3>
                          <p className="text-sm text-theme-muted mt-1">{alert.description}</p>

                          {/* Timing and Agent Info */}
                          <div className="flex flex-wrap gap-4 mt-3 text-xs text-theme-muted">
                            <span className="flex items-center gap-1">
                              <Clock className="w-3 h-3" />
                              {formatRelativeTime(new Date(alert.created_at))}
                            </span>
                            {alert.target_id && (
                              <Link
                                to={`/targets/${alert.target_id}`}
                                className="text-pilot-cyan hover:text-pilot-cyan-light font-mono transition-colors"
                              >
                                {alert.target_ip}
                              </Link>
                            )}
                            {alert.subnet_cidr && (
                              <span className="text-theme-secondary font-mono">({alert.subnet_cidr})</span>
                            )}
                            {alert.agent_name && (
                              <span className="flex items-center gap-1">
                                <Server className="w-3 h-3" />
                                <span className="text-theme-secondary">{alert.agent_name}</span>
                              </span>
                            )}
                            {alert.acknowledged_by && (
                              <span>Acked by: <span className="text-theme-secondary">{alert.acknowledged_by}</span></span>
                            )}
                          </div>

                          {/* Enriched Subnet Metadata */}
                          {(alert.subscriber_name || alert.pop_name || alert.location_address) && (
                            <div className="flex flex-wrap gap-3 mt-2 text-xs">
                              {alert.subscriber_name && (
                                <span className="flex items-center gap-1 px-2 py-0.5 bg-pilot-purple/10 rounded text-pilot-purple">
                                  <Building2 className="w-3 h-3" />
                                  {alert.subscriber_name}
                                </span>
                              )}
                              {alert.pop_name && (
                                <span className="flex items-center gap-1 px-2 py-0.5 bg-pilot-cyan/10 rounded text-pilot-cyan">
                                  <Network className="w-3 h-3" />
                                  {alert.pop_name}
                                </span>
                              )}
                              {alert.gateway_device && (
                                <span className="flex items-center gap-1 px-2 py-0.5 bg-neutral-700/50 rounded text-theme-secondary">
                                  <Server className="w-3 h-3" />
                                  {alert.gateway_device}
                                </span>
                              )}
                              {alert.location_address && (
                                <span className="flex items-center gap-1 px-2 py-0.5 bg-neutral-700/50 rounded text-theme-secondary">
                                  <MapPin className="w-3 h-3" />
                                  {alert.location_address}
                                </span>
                              )}
                              {alert.city && alert.region && !alert.location_address && (
                                <span className="flex items-center gap-1 px-2 py-0.5 bg-neutral-700/50 rounded text-theme-secondary">
                                  <MapPin className="w-3 h-3" />
                                  {alert.city}, {alert.region}
                                </span>
                              )}
                            </div>
                          )}

                          {/* Metrics if available */}
                          {(alert.current_latency_ms || alert.current_packet_loss) && (
                            <div className="flex gap-4 mt-2 text-xs">
                              {alert.current_latency_ms && (
                                <span className="text-warning">
                                  Latency: {alert.current_latency_ms.toFixed(1)}ms
                                </span>
                              )}
                              {alert.current_packet_loss && (
                                <span className="text-pilot-red">
                                  Loss: {alert.current_packet_loss.toFixed(1)}%
                                </span>
                              )}
                            </div>
                          )}
                        </div>

                        <div className="flex gap-2">
                          {!acknowledged && !resolved && (
                            <Button
                              variant="secondary"
                              size="sm"
                              onClick={() => handleAcknowledge(alert.id)}
                              disabled={acknowledging === alert.id}
                            >
                              {acknowledging === alert.id ? (
                                <Loader2 className="w-4 h-4 animate-spin" />
                              ) : (
                                'Acknowledge'
                              )}
                            </Button>
                          )}
                          {acknowledged && !resolved && (
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleResolve(alert.id)}
                            >
                              Resolve
                            </Button>
                          )}
                          <Button variant="ghost" size="sm">
                            <ChevronRight className="w-4 h-4" />
                          </Button>
                        </div>
                      </div>
                    </div>
                  </div>
                </Card>
              );
            })}

            {filteredAlerts.length === 0 && !loading && (
              <Card>
                <div className="text-center py-12 text-theme-muted">
                  <Bell className="w-12 h-12 mx-auto mb-4 opacity-50" />
                  <p>No alerts match your filters</p>
                </div>
              </Card>
            )}
          </div>
        ) : null}
      </PageContent>
    </>
  );
}
