import { useState, useMemo } from 'react';
import {
  Bell,
  Filter,
  CheckCircle,
  Clock,
  AlertTriangle,
  XCircle,
  ChevronRight,
} from 'lucide-react';

import { PageHeader, PageContent } from '../components/Layout';
import { Card, CardTitle, CardContent } from '../components/Card';
import { MetricCard } from '../components/MetricCard';
import { Button } from '../components/Button';
import { SearchInput, Select } from '../components/Input';
import { AlertCard } from '../components/Alert';
import { formatRelativeTime } from '../lib/utils';

// Demo data
const mockAlerts = [
  {
    id: 'alert-001',
    severity: 'critical',
    title: 'Target Unreachable - Infrastructure',
    message: 'Core router chi-01 (10.0.1.1) has been unreachable for 5 minutes from all 127 agents.',
    target: '10.0.1.1',
    targetName: 'core-router-chi-01',
    tier: 'infrastructure',
    createdAt: new Date(Date.now() - 300000),
    acknowledged: false,
    acknowledgedBy: null,
    resolvedAt: null,
  },
  {
    id: 'alert-002',
    severity: 'warning',
    title: 'High Latency - VIP Customer',
    message: 'Latency to Acme Corp (192.168.5.42) exceeds threshold: 156ms (threshold: 100ms) from 3 agents.',
    target: '192.168.5.42',
    targetName: 'Acme Corp - CIR-12345',
    tier: 'vip',
    createdAt: new Date(Date.now() - 900000),
    acknowledged: true,
    acknowledgedBy: 'jsmith',
    acknowledgedAt: new Date(Date.now() - 600000),
    resolvedAt: null,
  },
  {
    id: 'alert-003',
    severity: 'warning',
    title: 'Packet Loss Detected',
    message: 'Packet loss of 5% detected to edge-router-nyc-02 (10.0.2.15) from 2 agents.',
    target: '10.0.2.15',
    targetName: 'edge-router-nyc-02',
    tier: 'infrastructure',
    createdAt: new Date(Date.now() - 1800000),
    acknowledged: false,
    acknowledgedBy: null,
    resolvedAt: null,
  },
  {
    id: 'alert-004',
    severity: 'critical',
    title: 'Security Test PASSED - Expected Failure',
    message: 'SSH port on fw-internal-01 (10.100.0.1) is reachable from external agents! Expected connection to fail.',
    target: '10.100.0.1',
    targetName: 'fw-internal-01 (Security Test)',
    tier: 'standard',
    createdAt: new Date(Date.now() - 3600000),
    acknowledged: true,
    acknowledgedBy: 'secops',
    acknowledgedAt: new Date(Date.now() - 3000000),
    resolvedAt: null,
  },
  {
    id: 'alert-005',
    severity: 'info',
    title: 'Agent Reconnected',
    message: 'Agent asia-03 reconnected after being offline for 15 minutes.',
    agent: 'asia-03',
    createdAt: new Date(Date.now() - 7200000),
    acknowledged: false,
    acknowledgedBy: null,
    resolvedAt: new Date(Date.now() - 7100000),
  },
];

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
  const [alerts, setAlerts] = useState(mockAlerts);

  const filteredAlerts = useMemo(() => {
    return alerts.filter((alert) => {
      if (search) {
        const searchLower = search.toLowerCase();
        const matchesTitle = alert.title.toLowerCase().includes(searchLower);
        const matchesTarget = alert.target?.includes(search) || alert.targetName?.toLowerCase().includes(searchLower);
        if (!matchesTitle && !matchesTarget) return false;
      }
      if (severityFilter && alert.severity !== severityFilter) return false;
      if (statusFilter) {
        if (statusFilter === 'active' && (alert.acknowledged || alert.resolvedAt)) return false;
        if (statusFilter === 'acknowledged' && !alert.acknowledged) return false;
        if (statusFilter === 'resolved' && !alert.resolvedAt) return false;
      }
      return true;
    });
  }, [alerts, search, severityFilter, statusFilter]);

  const stats = useMemo(() => {
    const active = alerts.filter((a) => !a.resolvedAt && !a.acknowledged).length;
    const acknowledged = alerts.filter((a) => a.acknowledged && !a.resolvedAt).length;
    const critical = alerts.filter((a) => a.severity === 'critical' && !a.resolvedAt).length;

    return { active, acknowledged, critical };
  }, [alerts]);

  const handleAcknowledge = (alertId) => {
    setAlerts((prev) =>
      prev.map((a) =>
        a.id === alertId
          ? { ...a, acknowledged: true, acknowledgedBy: 'you', acknowledgedAt: new Date() }
          : a
      )
    );
  };

  return (
    <>
      <PageHeader
        title="Alerts"
        description="Active and historical alert notifications"
        actions={
          <Button variant="secondary" className="gap-2">
            <Bell className="w-4 h-4" />
            Configure Rules
          </Button>
        }
      />

      <PageContent>
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
            title="This Week"
            value={alerts.length}
            icon={Clock}
          />
        </div>

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
            <Button variant="ghost" size="sm">
              Acknowledge All
            </Button>
          </div>
        </Card>

        {/* Alert List */}
        <div className="space-y-3">
          {filteredAlerts.map((alert) => (
            <Card
              key={alert.id}
              className={`
                ${alert.resolvedAt ? 'opacity-60' : ''}
                ${alert.acknowledged && !alert.resolvedAt ? 'border-l-4 border-l-pilot-cyan' : ''}
              `}
            >
              <div className="flex items-start gap-4">
                <div className={`
                  p-2 rounded-lg
                  ${alert.severity === 'critical' ? 'bg-pilot-red/20' : ''}
                  ${alert.severity === 'warning' ? 'bg-warning/20' : ''}
                  ${alert.severity === 'info' ? 'bg-pilot-cyan/20' : ''}
                `}>
                  {alert.severity === 'critical' && <XCircle className="w-5 h-5 text-pilot-red" />}
                  {alert.severity === 'warning' && <AlertTriangle className="w-5 h-5 text-warning" />}
                  {alert.severity === 'info' && <Bell className="w-5 h-5 text-pilot-cyan" />}
                </div>

                <div className="flex-1">
                  <div className="flex items-start justify-between">
                    <div>
                      <div className="flex items-center gap-2 mb-1">
                        <span className={`
                          px-2 py-0.5 rounded text-xs font-medium uppercase
                          ${alert.severity === 'critical' ? 'bg-pilot-red text-theme-primary' : ''}
                          ${alert.severity === 'warning' ? 'bg-warning text-neutral-900' : ''}
                          ${alert.severity === 'info' ? 'bg-pilot-cyan text-neutral-900' : ''}
                        `}>
                          {alert.severity}
                        </span>
                        {alert.tier && (
                          <span className="text-xs text-theme-muted capitalize">{alert.tier}</span>
                        )}
                        {alert.resolvedAt && (
                          <span className="px-2 py-0.5 rounded text-xs bg-status-healthy/20 text-status-healthy">
                            Resolved
                          </span>
                        )}
                        {alert.acknowledged && !alert.resolvedAt && (
                          <span className="px-2 py-0.5 rounded text-xs bg-pilot-cyan/20 text-pilot-cyan">
                            Acknowledged
                          </span>
                        )}
                      </div>
                      <h3 className="font-medium text-theme-primary">{alert.title}</h3>
                      <p className="text-sm text-theme-muted mt-1">{alert.message}</p>

                      <div className="flex gap-4 mt-3 text-xs text-theme-muted">
                        <span className="flex items-center gap-1">
                          <Clock className="w-3 h-3" />
                          {formatRelativeTime(alert.createdAt)}
                        </span>
                        {alert.target && (
                          <span>Target: <span className="text-theme-secondary font-mono">{alert.target}</span></span>
                        )}
                        {alert.agent && (
                          <span>Agent: <span className="text-theme-secondary">{alert.agent}</span></span>
                        )}
                        {alert.acknowledgedBy && (
                          <span>Acked by: <span className="text-theme-secondary">{alert.acknowledgedBy}</span></span>
                        )}
                      </div>
                    </div>

                    <div className="flex gap-2">
                      {!alert.acknowledged && !alert.resolvedAt && (
                        <Button
                          variant="secondary"
                          size="sm"
                          onClick={() => handleAcknowledge(alert.id)}
                        >
                          Acknowledge
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
          ))}

          {filteredAlerts.length === 0 && (
            <Card>
              <div className="text-center py-12 text-theme-muted">
                <Bell className="w-12 h-12 mx-auto mb-4 opacity-50" />
                <p>No alerts match your filters</p>
              </div>
            </Card>
          )}
        </div>
      </PageContent>
    </>
  );
}
