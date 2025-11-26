import { useState, useEffect, useMemo } from 'react';
import {
  AlertCircle,
  AlertTriangle,
  Clock,
  RefreshCw,
  ChevronRight,
  CheckCircle,
  XCircle,
  MessageSquare,
  User,
  Server,
  Target,
  Activity,
  Filter,
} from 'lucide-react';

import { PageHeader, PageContent } from '../components/Layout';
import { Card, CardTitle, CardContent } from '../components/Card';
import { MetricCard } from '../components/MetricCard';
import { StatusBadge, StatusDot } from '../components/StatusBadge';
import { Button } from '../components/Button';
import { SearchInput, Select } from '../components/Input';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '../components/Table';
import { formatRelativeTime } from '../lib/utils';
import { endpoints } from '../lib/api';

const statusOptions = [
  { value: '', label: 'All Statuses' },
  { value: 'active', label: 'Active' },
  { value: 'acknowledged', label: 'Acknowledged' },
  { value: 'resolved', label: 'Resolved' },
];

const severityOptions = [
  { value: '', label: 'All Severities' },
  { value: 'critical', label: 'Critical' },
  { value: 'major', label: 'Major' },
  { value: 'minor', label: 'Minor' },
  { value: 'warning', label: 'Warning' },
];

const typeOptions = [
  { value: '', label: 'All Types' },
  { value: 'target_down', label: 'Target Down' },
  { value: 'latency_spike', label: 'Latency Spike' },
  { value: 'packet_loss', label: 'Packet Loss' },
  { value: 'agent_down', label: 'Agent Down' },
];

const severityColors = {
  critical: 'text-pilot-red',
  major: 'text-warning',
  minor: 'text-accent',
  warning: 'text-theme-muted',
};

const severityBgColors = {
  critical: 'bg-pilot-red/20',
  major: 'bg-warning/20',
  minor: 'bg-pilot-yellow/20',
  warning: 'bg-gray-500/20',
};

export function Incidents() {
  const [incidents, setIncidents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [statusFilter, setStatusFilter] = useState('');
  const [severityFilter, setSeverityFilter] = useState('');
  const [typeFilter, setTypeFilter] = useState('');
  const [search, setSearch] = useState('');
  const [selectedIncident, setSelectedIncident] = useState(null);
  const [actionLoading, setActionLoading] = useState(false);
  const [newNote, setNewNote] = useState('');

  const fetchIncidents = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await endpoints.listIncidents(statusFilter, 100);
      setIncidents(res.incidents || []);
    } catch (err) {
      console.error('Failed to fetch incidents:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const fetchIncidentDetails = async (id) => {
    try {
      const res = await endpoints.getIncident(id);
      setSelectedIncident(res.incident || res);
    } catch (err) {
      console.error('Failed to fetch incident details:', err);
    }
  };

  useEffect(() => {
    fetchIncidents();
    const interval = setInterval(fetchIncidents, 15000);
    return () => clearInterval(interval);
  }, [statusFilter]);

  useEffect(() => {
    if (selectedIncident?.id) {
      fetchIncidentDetails(selectedIncident.id);
    }
  }, []);

  const handleAcknowledge = async (incident) => {
    setActionLoading(true);
    try {
      await endpoints.acknowledgeIncident(incident.id, 'ui');
      await fetchIncidents();
      if (selectedIncident?.id === incident.id) {
        await fetchIncidentDetails(incident.id);
      }
    } catch (err) {
      console.error('Failed to acknowledge incident:', err);
    } finally {
      setActionLoading(false);
    }
  };

  const handleResolve = async (incident) => {
    setActionLoading(true);
    try {
      await endpoints.resolveIncident(incident.id);
      await fetchIncidents();
      if (selectedIncident?.id === incident.id) {
        await fetchIncidentDetails(incident.id);
      }
    } catch (err) {
      console.error('Failed to resolve incident:', err);
    } finally {
      setActionLoading(false);
    }
  };

  const handleAddNote = async () => {
    if (!selectedIncident || !newNote.trim()) return;
    setActionLoading(true);
    try {
      await endpoints.addIncidentNote(selectedIncident.id, newNote.trim());
      setNewNote('');
      await fetchIncidentDetails(selectedIncident.id);
    } catch (err) {
      console.error('Failed to add note:', err);
    } finally {
      setActionLoading(false);
    }
  };

  // Calculate stats
  const stats = useMemo(() => {
    const active = incidents.filter(i => i.status === 'active').length;
    const acknowledged = incidents.filter(i => i.status === 'acknowledged').length;
    const resolved = incidents.filter(i => i.status === 'resolved').length;
    const critical = incidents.filter(i => i.severity === 'critical' && i.status !== 'resolved').length;
    return { active, acknowledged, resolved, critical };
  }, [incidents]);

  // Filter incidents
  const filteredIncidents = useMemo(() => {
    return incidents.filter(incident => {
      if (severityFilter && incident.severity !== severityFilter) return false;
      if (typeFilter && incident.incident_type !== typeFilter) return false;
      if (search) {
        const searchLower = search.toLowerCase();
        const matchesIP = incident.target_ip?.toLowerCase().includes(searchLower);
        const matchesAgent = incident.agent_name?.toLowerCase().includes(searchLower);
        const matchesTitle = incident.title?.toLowerCase().includes(searchLower);
        if (!matchesIP && !matchesAgent && !matchesTitle) return false;
      }
      return true;
    });
  }, [incidents, severityFilter, typeFilter, search]);

  const getIncidentTitle = (incident) => {
    if (incident.title) return incident.title;
    const typeLabels = {
      target_down: 'Target Unreachable',
      latency_spike: 'Latency Anomaly',
      packet_loss: 'Packet Loss Detected',
      agent_down: 'Agent Offline',
    };
    return typeLabels[incident.incident_type] || incident.incident_type;
  };

  const getBlastRadiusIcon = (blastRadius) => {
    switch (blastRadius) {
      case 'single_pair':
        return <Activity className="w-4 h-4" />;
      case 'agent_wide':
        return <Server className="w-4 h-4" />;
      case 'target_wide':
        return <Target className="w-4 h-4" />;
      case 'global':
        return <AlertCircle className="w-4 h-4" />;
      default:
        return <Activity className="w-4 h-4" />;
    }
  };

  const getBlastRadiusLabel = (blastRadius) => {
    const labels = {
      single_pair: 'Single Pair',
      agent_wide: 'Agent-Wide',
      target_wide: 'Target-Wide',
      global: 'Global',
    };
    return labels[blastRadius] || blastRadius;
  };

  if (error) {
    return (
      <>
        <PageHeader title="Incidents" />
        <PageContent>
          <Card accent="red">
            <div className="flex items-center gap-3">
              <AlertTriangle className="w-6 h-6 text-pilot-red" />
              <div>
                <h3 className="font-medium text-theme-primary">Failed to load incidents</h3>
                <p className="text-sm text-theme-muted">{error}</p>
              </div>
              <Button variant="secondary" size="sm" onClick={fetchIncidents} className="ml-auto">
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
        title="Incidents"
        description={`${stats.active + stats.acknowledged} open incidents`}
        actions={
          <Button variant="secondary" onClick={fetchIncidents} className="gap-2">
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
            Refresh
          </Button>
        }
      />

      <PageContent>
        {/* Summary Cards */}
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
          <MetricCard
            title="Active"
            value={stats.active}
            status={stats.active > 0 ? 'down' : 'healthy'}
            icon={AlertCircle}
          />
          <MetricCard
            title="Acknowledged"
            value={stats.acknowledged}
            status={stats.acknowledged > 0 ? 'degraded' : 'healthy'}
            icon={CheckCircle}
          />
          <MetricCard
            title="Resolved (Today)"
            value={stats.resolved}
            status="healthy"
            icon={XCircle}
          />
          <MetricCard
            title="Critical"
            value={stats.critical}
            status={stats.critical > 0 ? 'down' : 'healthy'}
            icon={AlertTriangle}
          />
        </div>

        {/* Filters */}
        <Card className="mb-6">
          <div className="flex flex-wrap gap-4 items-center">
            <SearchInput
              value={search}
              onChange={setSearch}
              placeholder="Search IP, agent, or title..."
              className="w-72"
            />
            <Select
              options={statusOptions}
              value={statusFilter}
              onChange={setStatusFilter}
              className="w-40"
            />
            <Select
              options={severityOptions}
              value={severityFilter}
              onChange={setSeverityFilter}
              className="w-40"
            />
            <Select
              options={typeOptions}
              value={typeFilter}
              onChange={setTypeFilter}
              className="w-44"
            />
          </div>
        </Card>

        {/* Incidents List */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <div className="lg:col-span-2">
            <Card>
              {filteredIncidents.length === 0 ? (
                <div className="text-center py-12 text-theme-muted">
                  <AlertCircle className="w-12 h-12 mx-auto mb-4 opacity-50" />
                  {incidents.length === 0 ? (
                    <>
                      <p>No incidents recorded</p>
                      <p className="text-sm mt-1">All systems operational</p>
                    </>
                  ) : (
                    <p>No incidents match your filters</p>
                  )}
                </div>
              ) : (
                <div className="divide-y divide-pilot-navy-light">
                  {filteredIncidents.map(incident => (
                    <div
                      key={incident.id}
                      onClick={() => {
                        setSelectedIncident(incident);
                        fetchIncidentDetails(incident.id);
                      }}
                      className={`p-4 cursor-pointer transition-colors ${
                        selectedIncident?.id === incident.id
                          ? 'bg-surface-tertiary'
                          : 'hover:bg-surface-primary'
                      }`}
                    >
                      <div className="flex items-start gap-3">
                        <div className={`p-2 rounded-lg ${severityBgColors[incident.severity] || 'bg-gray-500/20'}`}>
                          <AlertCircle className={`w-5 h-5 ${severityColors[incident.severity] || 'text-theme-muted'}`} />
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 mb-1">
                            <span className="font-medium text-theme-primary truncate">
                              {getIncidentTitle(incident)}
                            </span>
                            <StatusBadge
                              status={incident.status === 'active' ? 'down' : incident.status === 'acknowledged' ? 'degraded' : 'healthy'}
                              label={incident.status}
                              size="sm"
                            />
                          </div>
                          <div className="flex items-center gap-4 text-sm text-theme-muted">
                            {incident.target_ip && (
                              <span className="font-mono">{incident.target_ip}</span>
                            )}
                            {incident.agent_name && (
                              <span className="flex items-center gap-1">
                                <Server className="w-3 h-3" />
                                {incident.agent_name}
                              </span>
                            )}
                            <span className="flex items-center gap-1">
                              {getBlastRadiusIcon(incident.blast_radius)}
                              {getBlastRadiusLabel(incident.blast_radius)}
                            </span>
                          </div>
                          <div className="flex items-center gap-4 mt-2 text-xs text-theme-muted">
                            <span className="flex items-center gap-1">
                              <Clock className="w-3 h-3" />
                              Started {formatRelativeTime(incident.started_at)}
                            </span>
                            {incident.acknowledged_at && (
                              <span className="flex items-center gap-1">
                                <CheckCircle className="w-3 h-3 text-pilot-cyan" />
                                Ack'd by {incident.acknowledged_by}
                              </span>
                            )}
                          </div>
                        </div>
                        <ChevronRight className="w-5 h-5 text-theme-muted flex-shrink-0" />
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </Card>
          </div>

          {/* Incident Detail Panel */}
          <div className="lg:col-span-1">
            {selectedIncident ? (
              <Card
                accent={
                  selectedIncident.severity === 'critical' ? 'red' :
                  selectedIncident.severity === 'major' ? 'warning' :
                  'cyan'
                }
              >
                <div className="mb-4">
                  <div className="flex items-center gap-2 mb-2">
                    <span className={`px-2 py-0.5 rounded text-xs font-medium uppercase ${severityBgColors[selectedIncident.severity]} ${severityColors[selectedIncident.severity]}`}>
                      {selectedIncident.severity}
                    </span>
                    <StatusBadge
                      status={selectedIncident.status === 'active' ? 'down' : selectedIncident.status === 'acknowledged' ? 'degraded' : 'healthy'}
                      label={selectedIncident.status}
                      size="sm"
                    />
                  </div>
                  <h3 className="text-lg font-semibold text-theme-primary">
                    {getIncidentTitle(selectedIncident)}
                  </h3>
                </div>

                {/* Incident Details */}
                <div className="space-y-3 mb-6">
                  {selectedIncident.target_ip && (
                    <div className="flex items-center justify-between">
                      <span className="text-theme-muted text-sm">Target</span>
                      <span className="font-mono text-theme-primary">{selectedIncident.target_ip}</span>
                    </div>
                  )}
                  {selectedIncident.agent_name && (
                    <div className="flex items-center justify-between">
                      <span className="text-theme-muted text-sm">Agent</span>
                      <span className="text-theme-primary">{selectedIncident.agent_name}</span>
                    </div>
                  )}
                  <div className="flex items-center justify-between">
                    <span className="text-theme-muted text-sm">Blast Radius</span>
                    <span className="flex items-center gap-1 text-theme-primary">
                      {getBlastRadiusIcon(selectedIncident.blast_radius)}
                      {getBlastRadiusLabel(selectedIncident.blast_radius)}
                    </span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-theme-muted text-sm">Started</span>
                    <span className="text-theme-primary">{new Date(selectedIncident.started_at).toLocaleString()}</span>
                  </div>
                  {selectedIncident.resolved_at && (
                    <div className="flex items-center justify-between">
                      <span className="text-theme-muted text-sm">Resolved</span>
                      <span className="text-theme-primary">{new Date(selectedIncident.resolved_at).toLocaleString()}</span>
                    </div>
                  )}
                  {selectedIncident.acknowledged_by && (
                    <div className="flex items-center justify-between">
                      <span className="text-theme-muted text-sm">Acknowledged By</span>
                      <span className="flex items-center gap-1 text-theme-primary">
                        <User className="w-3 h-3" />
                        {selectedIncident.acknowledged_by}
                      </span>
                    </div>
                  )}
                </div>

                {/* Actions */}
                {selectedIncident.status !== 'resolved' && (
                  <div className="flex gap-2 mb-6">
                    {selectedIncident.status === 'active' && (
                      <Button
                        variant="secondary"
                        size="sm"
                        className="flex-1"
                        onClick={() => handleAcknowledge(selectedIncident)}
                        disabled={actionLoading}
                      >
                        {actionLoading ? (
                          <RefreshCw className="w-4 h-4 animate-spin" />
                        ) : (
                          <CheckCircle className="w-4 h-4 mr-1" />
                        )}
                        Acknowledge
                      </Button>
                    )}
                    <Button
                      variant="primary"
                      size="sm"
                      className="flex-1"
                      onClick={() => handleResolve(selectedIncident)}
                      disabled={actionLoading}
                    >
                      {actionLoading ? (
                        <RefreshCw className="w-4 h-4 animate-spin" />
                      ) : (
                        <XCircle className="w-4 h-4 mr-1" />
                      )}
                      Resolve
                    </Button>
                  </div>
                )}

                {/* Timeline / Notes */}
                <div className="border-t border-theme pt-4">
                  <h4 className="text-sm font-medium text-theme-muted mb-3 flex items-center gap-2">
                    <MessageSquare className="w-4 h-4" />
                    Notes & Timeline
                  </h4>

                  {/* Add Note */}
                  {selectedIncident.status !== 'resolved' && (
                    <div className="mb-4">
                      <textarea
                        value={newNote}
                        onChange={(e) => setNewNote(e.target.value)}
                        placeholder="Add a note..."
                        className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary text-sm placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan resize-none"
                        rows={2}
                      />
                      <Button
                        size="sm"
                        className="mt-2"
                        onClick={handleAddNote}
                        disabled={!newNote.trim() || actionLoading}
                      >
                        Add Note
                      </Button>
                    </div>
                  )}

                  {/* Notes List */}
                  <div className="space-y-3 max-h-64 overflow-y-auto">
                    {(selectedIncident.notes || []).length === 0 ? (
                      <p className="text-sm text-theme-muted text-center py-4">No notes yet</p>
                    ) : (
                      selectedIncident.notes.map((note, idx) => (
                        <div key={idx} className="bg-surface-primary rounded-lg p-3">
                          <p className="text-sm text-theme-primary">{note.text || note}</p>
                          {note.created_at && (
                            <p className="text-xs text-theme-muted mt-1">
                              {formatRelativeTime(note.created_at)}
                              {note.author && ` by ${note.author}`}
                            </p>
                          )}
                        </div>
                      ))
                    )}
                  </div>
                </div>

                <div className="mt-4 pt-4 border-t border-theme">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setSelectedIncident(null)}
                  >
                    Close
                  </Button>
                </div>
              </Card>
            ) : (
              <Card>
                <div className="text-center py-12 text-theme-muted">
                  <AlertCircle className="w-12 h-12 mx-auto mb-4 opacity-50" />
                  <p>Select an incident to view details</p>
                </div>
              </Card>
            )}
          </div>
        </div>
      </PageContent>
    </>
  );
}
