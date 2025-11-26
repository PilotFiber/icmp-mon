import { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Server,
  MapPin,
  RefreshCw,
  AlertTriangle,
  Cpu,
  HardDrive,
  Activity,
  Clock,
  Globe,
  Shield,
} from 'lucide-react';

import { PageHeader, PageContent } from '../components/Layout';
import { Card } from '../components/Card';
import { MetricCardCompact } from '../components/MetricCard';
import { StatusBadge } from '../components/StatusBadge';
import { Button } from '../components/Button';
import { formatRelativeTime } from '../lib/utils';
import { endpoints } from '../lib/api';

export function AgentDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [agent, setAgent] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const fetchAgent = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await endpoints.getAgent(id);
      setAgent(res);
    } catch (err) {
      console.error('Failed to fetch agent:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchAgent();
    const interval = setInterval(fetchAgent, 10000);
    return () => clearInterval(interval);
  }, [id]);

  const getStatusColor = (status) => {
    switch (status) {
      case 'active': return 'healthy';
      case 'degraded': return 'degraded';
      case 'offline': return 'down';
      default: return 'unknown';
    }
  };

  if (loading) {
    return (
      <>
        <PageHeader
          title="Loading..."
          breadcrumbs={[
            { label: 'Agents', href: '/agents' },
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

  if (error || !agent) {
    return (
      <>
        <PageHeader
          title="Agent Not Found"
          breadcrumbs={[
            { label: 'Agents', href: '/agents' },
            { label: 'Error' },
          ]}
        />
        <PageContent>
          <Card accent="red">
            <div className="flex items-center gap-3">
              <AlertTriangle className="w-6 h-6 text-pilot-red" />
              <div>
                <h3 className="font-medium text-theme-primary">Failed to load agent</h3>
                <p className="text-sm text-theme-muted">{error || 'Agent not found'}</p>
              </div>
              <Button variant="secondary" size="sm" onClick={() => navigate('/agents')} className="ml-auto">
                Back to Agents
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
        title={agent.name}
        breadcrumbs={[
          { label: 'Agents', href: '/agents' },
          { label: agent.name },
        ]}
        actions={
          <div className="flex gap-3">
            <Button variant="secondary" onClick={fetchAgent} className="gap-2">
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </Button>
          </div>
        }
      />

      <PageContent>
        {/* Main Info Card */}
        <Card accent={agent.status === 'active' ? 'cyan' : agent.status === 'degraded' ? 'warning' : 'red'} className="mb-6">
          <div className="flex items-start justify-between mb-6">
            <div>
              <div className="flex items-center gap-3 mb-1">
                <div className="p-2 bg-surface-tertiary rounded-lg">
                  <Server className="w-6 h-6 text-pilot-cyan" />
                </div>
                <div>
                  <h2 className="text-xl font-semibold text-theme-primary">{agent.name}</h2>
                  <p className="text-theme-muted flex items-center gap-2">
                    <MapPin className="w-4 h-4" />
                    {agent.location || agent.region || 'Unknown location'} • {agent.provider || 'Unknown provider'}
                  </p>
                </div>
              </div>
            </div>
            <StatusBadge
              status={getStatusColor(agent.status)}
              label={agent.status}
              pulse={agent.status === 'offline'}
            />
          </div>

          {/* Key Metrics */}
          <div className="grid grid-cols-2 md:grid-cols-5 gap-4 mb-6">
            <MetricCardCompact
              title="Region"
              value={agent.region || 'Unknown'}
            />
            <MetricCardCompact
              title="Provider"
              value={agent.provider || 'Unknown'}
            />
            <MetricCardCompact
              title="Version"
              value={agent.version || 'unknown'}
            />
            <MetricCardCompact
              title="Max Targets"
              value={agent.max_targets?.toLocaleString() || '—'}
            />
            <MetricCardCompact
              title="Public IP"
              value={agent.public_ip || '—'}
            />
          </div>

          {/* Connection Info */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <div className="bg-surface-primary rounded-lg p-4">
              <h4 className="text-sm font-medium text-theme-muted mb-3 flex items-center gap-2">
                <Clock className="w-4 h-4" />
                Connection Status
              </h4>
              <div className="space-y-3">
                <div className="flex justify-between">
                  <span className="text-theme-muted">Last Heartbeat</span>
                  <span className="text-theme-primary">
                    {agent.last_heartbeat ? formatRelativeTime(agent.last_heartbeat) : 'Never'}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-theme-muted">Enrolled</span>
                  <span className="text-theme-primary">
                    {agent.created_at ? formatRelativeTime(agent.created_at) : 'Unknown'}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-theme-muted">Agent ID</span>
                  <span className="text-theme-primary font-mono text-xs">{agent.id}</span>
                </div>
              </div>
            </div>

            <div className="bg-surface-primary rounded-lg p-4">
              <h4 className="text-sm font-medium text-theme-muted mb-3 flex items-center gap-2">
                <Globe className="w-4 h-4" />
                Network Info
              </h4>
              <div className="space-y-3">
                <div className="flex justify-between">
                  <span className="text-theme-muted">Public IP</span>
                  <span className="text-theme-primary font-mono">{agent.public_ip || '—'}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-theme-muted">Location</span>
                  <span className="text-theme-primary">{agent.location || '—'}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-theme-muted">Region</span>
                  <span className="text-theme-primary">{agent.region || '—'}</span>
                </div>
              </div>
            </div>
          </div>
        </Card>

        {/* Executors */}
        {agent.executors && agent.executors.length > 0 && (
          <Card className="mb-6">
            <h3 className="text-sm font-medium text-theme-muted mb-4 flex items-center gap-2">
              <Cpu className="w-4 h-4" />
              Available Executors
            </h3>
            <div className="flex flex-wrap gap-2">
              {agent.executors.map((executor) => (
                <span
                  key={executor}
                  className="px-3 py-1.5 rounded-lg text-sm bg-surface-tertiary text-pilot-cyan border border-pilot-cyan/20"
                >
                  {executor}
                </span>
              ))}
            </div>
          </Card>
        )}

        {/* Capabilities */}
        <Card>
          <h3 className="text-sm font-medium text-theme-muted mb-4 flex items-center gap-2">
            <Shield className="w-4 h-4" />
            Agent Capabilities
          </h3>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div className="bg-surface-primary rounded-lg p-3 text-center">
              <Activity className="w-5 h-5 mx-auto mb-2 text-pilot-cyan" />
              <div className="text-sm text-theme-primary">ICMP Ping</div>
              <div className="text-xs text-theme-muted">Basic connectivity</div>
            </div>
            {agent.executors?.includes('mtr') && (
              <div className="bg-surface-primary rounded-lg p-3 text-center">
                <HardDrive className="w-5 h-5 mx-auto mb-2 text-status-healthy" />
                <div className="text-sm text-theme-primary">MTR</div>
                <div className="text-xs text-theme-muted">Path analysis</div>
              </div>
            )}
            {agent.executors?.includes('tcp') && (
              <div className="bg-surface-primary rounded-lg p-3 text-center">
                <Globe className="w-5 h-5 mx-auto mb-2 text-accent" />
                <div className="text-sm text-theme-primary">TCP Probe</div>
                <div className="text-xs text-theme-muted">Port connectivity</div>
              </div>
            )}
            {agent.executors?.includes('http') && (
              <div className="bg-surface-primary rounded-lg p-3 text-center">
                <Server className="w-5 h-5 mx-auto mb-2 text-pilot-red" />
                <div className="text-sm text-theme-primary">HTTP Check</div>
                <div className="text-xs text-theme-muted">Web requests</div>
              </div>
            )}
          </div>
        </Card>
      </PageContent>
    </>
  );
}
