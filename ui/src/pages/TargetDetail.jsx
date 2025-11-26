import { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Target,
  RefreshCw,
  Tag,
  ChevronDown,
  ExternalLink,
  AlertTriangle,
  Activity,
  Server,
  Check,
  X,
  Radio,
  Pause,
  Play,
  LayoutGrid,
  TrendingUp,
  Edit2,
  Trash2,
} from 'lucide-react';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from 'recharts';

import { PageHeader, PageContent } from '../components/Layout';
import { Card } from '../components/Card';
import { MetricCardCompact } from '../components/MetricCard';
import { StatusBadge } from '../components/StatusBadge';
import { Button } from '../components/Button';
import { Modal, ModalFooter, ConfirmModal } from '../components/Modal';
import { formatRelativeTime } from '../lib/utils';
import { endpoints } from '../lib/api';

const timeWindows = [
  { value: '15m', label: '15 min' },
  { value: '1h', label: '1 hour' },
  { value: '6h', label: '6 hours' },
  { value: '24h', label: '24 hours' },
  { value: '168h', label: '7 days' },
  { value: '720h', label: '30 days' },
];

const agentColors = [
  '#6EDBE0', '#FC534E', '#F7B84B', '#4CAF50',
  '#9C27B0', '#FF9800', '#2196F3', '#E91E63',
];

// Target Edit Modal
function TargetModal({ isOpen, onClose, target, tiers, onSave }) {
  const [formData, setFormData] = useState({
    ip: '',
    tier: 'standard',
    tags: {},
  });
  const [tagInput, setTagInput] = useState({ key: '', value: '' });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (target) {
      setFormData({
        ip: target.ip || '',
        tier: target.tier || 'standard',
        tags: { ...(target.tags || {}) },
      });
    }
    setError(null);
  }, [target, isOpen]);

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
      await endpoints.updateTarget(target.id, formData);
      onSave();
      onClose();
    } catch (err) {
      setError(err.message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Edit Target" size="md">
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
            disabled
            className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none disabled:opacity-50"
          />
          <p className="text-xs text-theme-muted mt-1">IP address cannot be changed</p>
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
              <span
                key={key}
                className="inline-flex items-center gap-1 px-2 py-1 bg-surface-tertiary rounded text-sm"
              >
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
        <Button onClick={handleSubmit} disabled={saving}>
          {saving ? 'Saving...' : 'Update'}
        </Button>
      </ModalFooter>
    </Modal>
  );
}

function PerAgentChart({ data, visibleAgents, metric = 'latency', timeWindow }) {
  if (!data || data.length === 0) {
    return (
      <div className="h-64 flex items-center justify-center text-theme-muted">
        No history data available
      </div>
    );
  }

  const agentMap = {};
  data.forEach(point => {
    if (!agentMap[point.agent_id]) {
      agentMap[point.agent_id] = {
        id: point.agent_id,
        name: point.agent_name,
        color: agentColors[Object.keys(agentMap).length % agentColors.length],
      };
    }
  });

  const timeMap = {};
  data.forEach(point => {
    const timeKey = new Date(point.time).getTime();
    if (!timeMap[timeKey]) {
      timeMap[timeKey] = { time: point.time };
    }
    const agentKey = point.agent_name.replace(/[^a-zA-Z0-9]/g, '_');
    if (metric === 'latency') {
      timeMap[timeKey][agentKey] = point.avg_latency_ms;
    } else {
      timeMap[timeKey][agentKey] = point.packet_loss_pct;
    }
  });

  const chartData = Object.values(timeMap)
    .sort((a, b) => new Date(a.time) - new Date(b.time))
    .map(point => {
      const timeObj = new Date(point.time);
      let timeLabel;
      if (timeWindow === '15m' || timeWindow === '1h' || timeWindow === '6h' || timeWindow === '24h') {
        timeLabel = timeObj.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
      } else {
        timeLabel = timeObj.toLocaleDateString([], { month: 'short', day: 'numeric' });
      }
      return { ...point, timeLabel };
    });

  const agents = Object.values(agentMap);

  return (
    <ResponsiveContainer width="100%" height={280}>
      <LineChart data={chartData}>
        <XAxis dataKey="timeLabel" stroke="#6B7280" fontSize={10} tickLine={false} axisLine={false} interval="preserveStartEnd" />
        <YAxis stroke="#6B7280" fontSize={10} tickLine={false} axisLine={false} tickFormatter={(v) => metric === 'latency' ? `${v?.toFixed(0) || 0}ms` : `${v?.toFixed(0) || 0}%`} />
        <Tooltip
          contentStyle={{ backgroundColor: '#18284F', border: '1px solid #2A3D6B', borderRadius: '8px' }}
          labelStyle={{ color: '#9CA3AF' }}
          formatter={(value, name) => [metric === 'latency' ? `${value?.toFixed(1) || 0}ms` : `${value?.toFixed(1) || 0}%`, name]}
        />
        <Legend wrapperStyle={{ paddingTop: 10 }} formatter={(value) => <span style={{ color: '#9CA3AF', fontSize: 11 }}>{value}</span>} />
        {agents.map(agent => {
          const agentKey = agent.name.replace(/[^a-zA-Z0-9]/g, '_');
          const isVisible = visibleAgents.length === 0 || visibleAgents.includes(agent.id);
          return (
            <Line key={agent.id} type="monotone" dataKey={agentKey} stroke={agent.color} strokeWidth={2} dot={false} name={agent.name} hide={!isVisible} connectNulls />
          );
        })}
      </LineChart>
    </ResponsiveContainer>
  );
}

function LiveView({ targetId }) {
  const [liveData, setLiveData] = useState([]);
  const [isRunning, setIsRunning] = useState(true);
  const [lastUpdate, setLastUpdate] = useState(null);
  const [error, setError] = useState(null);
  const [viewMode, setViewMode] = useState('graph');
  const [visibleAgents, setVisibleAgents] = useState([]);
  const seenKeys = useRef(new Set());

  useEffect(() => {
    setLiveData([]);
    seenKeys.current = new Set();
  }, [targetId]);

  useEffect(() => {
    if (!targetId || !isRunning) return;

    const fetchLive = async () => {
      try {
        const res = await endpoints.getTargetLive(targetId, 60);
        const newResults = res.results || [];

        const newUniqueResults = newResults.filter(r => {
          const key = `${r.agent_id}-${r.time}`;
          if (seenKeys.current.has(key)) return false;
          seenKeys.current.add(key);
          return true;
        });

        if (newUniqueResults.length > 0) {
          setLiveData(prev => [...prev, ...newUniqueResults]);
        }

        setLastUpdate(new Date());
        setError(null);
      } catch (err) {
        console.error('Failed to fetch live data:', err);
        setError(err.message);
      }
    };

    fetchLive();
    const interval = setInterval(fetchLive, 2000);
    return () => clearInterval(interval);
  }, [targetId, isRunning]);

  const resultsByAgent = useMemo(() => {
    const grouped = {};
    liveData.forEach(result => {
      if (!grouped[result.agent_id]) {
        grouped[result.agent_id] = { agent_id: result.agent_id, agent_name: result.agent_name, agent_region: result.agent_region, agent_provider: result.agent_provider, results: [] };
      }
      grouped[result.agent_id].results.push(result);
    });
    Object.values(grouped).forEach(agent => {
      agent.results.sort((a, b) => new Date(b.time) - new Date(a.time));
    });
    return Object.values(grouped);
  }, [liveData]);

  const chartData = useMemo(() => {
    if (liveData.length === 0) return { data: [], agents: [] };

    const agentMap = {};
    liveData.forEach(result => {
      if (!agentMap[result.agent_id]) {
        agentMap[result.agent_id] = { id: result.agent_id, name: result.agent_name, color: agentColors[Object.keys(agentMap).length % agentColors.length] };
      }
    });

    const timeMap = {};
    liveData.forEach(result => {
      const time = new Date(result.time);
      const timeKey = Math.floor(time.getTime() / 1000) * 1000;
      if (!timeMap[timeKey]) {
        timeMap[timeKey] = { time: timeKey };
      }
      const agentKey = result.agent_name.replace(/[^a-zA-Z0-9]/g, '_');
      if (result.success && result.latency_ms != null) {
        timeMap[timeKey][agentKey] = result.latency_ms;
      }
    });

    const sortedData = Object.values(timeMap)
      .sort((a, b) => a.time - b.time)
      .map(point => ({
        ...point,
        timeLabel: new Date(point.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
      }));

    return { data: sortedData, agents: Object.values(agentMap) };
  }, [liveData]);

  const getLatencyColor = (latency) => {
    if (latency === null) return 'text-theme-muted';
    if (latency < 50) return 'text-status-healthy';
    if (latency < 100) return 'text-pilot-cyan';
    if (latency < 200) return 'text-accent';
    return 'text-pilot-red';
  };

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-3">
          <h4 className="text-sm font-medium text-theme-muted flex items-center gap-2">
            <Radio className={`w-4 h-4 ${isRunning ? 'text-status-healthy animate-pulse' : 'text-theme-muted'}`} />
            Live Probe Results
          </h4>
          {lastUpdate && <span className="text-xs text-theme-muted">Last update: {lastUpdate.toLocaleTimeString()}</span>}
        </div>
        <div className="flex items-center gap-2">
          <div className="flex bg-surface-primary rounded-lg p-0.5">
            <button onClick={() => setViewMode('graph')} className={`px-2 py-1 text-xs rounded-md transition-colors flex items-center gap-1 ${viewMode === 'graph' ? 'bg-pilot-cyan text-neutral-900 font-medium' : 'text-theme-muted hover:text-theme-primary'}`}>
              <TrendingUp className="w-3 h-3" />Graph
            </button>
            <button onClick={() => setViewMode('cards')} className={`px-2 py-1 text-xs rounded-md transition-colors flex items-center gap-1 ${viewMode === 'cards' ? 'bg-pilot-cyan text-neutral-900 font-medium' : 'text-theme-muted hover:text-theme-primary'}`}>
              <LayoutGrid className="w-3 h-3" />Cards
            </button>
          </div>
          <button onClick={() => setIsRunning(!isRunning)} className={`flex items-center gap-1.5 px-3 py-1 rounded text-xs transition-colors ${isRunning ? 'bg-pilot-red/20 text-pilot-red hover:bg-pilot-red/30' : 'bg-status-healthy/20 text-status-healthy hover:bg-status-healthy/30'}`}>
            {isRunning ? <><Pause className="w-3 h-3" />Pause</> : <><Play className="w-3 h-3" />Resume</>}
          </button>
        </div>
      </div>

      {error && <div className="text-pilot-red text-sm mb-3">{error}</div>}

      {liveData.length === 0 ? (
        <div className="text-center py-8 text-theme-muted">
          {isRunning ? <div className="flex items-center justify-center gap-2"><RefreshCw className="w-4 h-4 animate-spin" />Waiting for probe results...</div> : 'No results in the last 60 seconds'}
        </div>
      ) : viewMode === 'graph' ? (
        <div>
          <div className="flex flex-wrap gap-2 mb-3">
            {chartData.agents.map(agent => {
              const isVisible = visibleAgents.length === 0 || visibleAgents.includes(agent.id);
              return (
                <button key={agent.id} onClick={() => {
                  if (visibleAgents.length === 0) setVisibleAgents([agent.id]);
                  else if (visibleAgents.includes(agent.id)) {
                    const newVisible = visibleAgents.filter(id => id !== agent.id);
                    setVisibleAgents(newVisible.length === 0 ? [] : newVisible);
                  } else setVisibleAgents([...visibleAgents, agent.id]);
                }} className={`flex items-center gap-1.5 px-2 py-1 rounded text-xs transition-colors ${isVisible ? 'bg-surface-tertiary text-theme-primary' : 'bg-surface-primary text-theme-muted'}`}>
                  <span className="w-2 h-2 rounded-full" style={{ backgroundColor: isVisible ? agent.color : '#4B5563' }} />
                  {agent.name}
                </button>
              );
            })}
            {visibleAgents.length > 0 && <button onClick={() => setVisibleAgents([])} className="text-xs text-pilot-cyan hover:text-pilot-cyan-light">Show All</button>}
          </div>

          <ResponsiveContainer width="100%" height={280}>
            <LineChart data={chartData.data}>
              <XAxis dataKey="timeLabel" stroke="#6B7280" fontSize={10} tickLine={false} axisLine={false} interval="preserveStartEnd" />
              <YAxis stroke="#6B7280" fontSize={10} tickLine={false} axisLine={false} tickFormatter={(v) => `${v?.toFixed(0) || 0}ms`} domain={['auto', 'auto']} />
              <Tooltip contentStyle={{ backgroundColor: '#18284F', border: '1px solid #2A3D6B', borderRadius: '8px' }} labelStyle={{ color: '#9CA3AF' }} formatter={(value, name) => [`${value?.toFixed(1) || 0}ms`, name]} />
              <Legend wrapperStyle={{ paddingTop: 10 }} formatter={(value) => <span style={{ color: '#9CA3AF', fontSize: 11 }}>{value}</span>} />
              {chartData.agents.map(agent => {
                const agentKey = agent.name.replace(/[^a-zA-Z0-9]/g, '_');
                const isVisible = visibleAgents.length === 0 || visibleAgents.includes(agent.id);
                return <Line key={agent.id} type="monotone" dataKey={agentKey} stroke={agent.color} strokeWidth={2} dot={{ r: 3, fill: agent.color }} name={agent.name} hide={!isVisible} connectNulls isAnimationActive={false} />;
              })}
            </LineChart>
          </ResponsiveContainer>

          <div className="mt-3 grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-2">
            {resultsByAgent.map(agent => {
              const agentColor = chartData.agents.find(a => a.id === agent.agent_id)?.color || '#6B7280';
              const successResults = agent.results.filter(r => r.success && r.latency_ms != null);
              const avgLatency = successResults.length > 0 ? successResults.reduce((sum, r) => sum + r.latency_ms, 0) / successResults.length : null;
              const minLatency = successResults.length > 0 ? Math.min(...successResults.map(r => r.latency_ms)) : null;
              const maxLatency = successResults.length > 0 ? Math.max(...successResults.map(r => r.latency_ms)) : null;
              return (
                <div key={agent.agent_id} className="bg-surface-primary rounded-lg p-2">
                  <div className="flex items-center gap-1.5 mb-1">
                    <span className="w-2 h-2 rounded-full" style={{ backgroundColor: agentColor }} />
                    <span className="text-xs font-medium text-theme-primary truncate">{agent.agent_name}</span>
                  </div>
                  <div className="text-xs text-theme-muted">
                    <div>Avg: <span className="text-theme-primary font-mono">{avgLatency?.toFixed(1) || '—'}ms</span></div>
                    <div>Min/Max: <span className="text-theme-primary font-mono">{minLatency?.toFixed(0) || '—'}/{maxLatency?.toFixed(0) || '—'}ms</span></div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {resultsByAgent.map(agent => (
            <div key={agent.agent_id} className="bg-surface-primary rounded-lg p-3">
              <div className="flex items-center gap-2 mb-2 border-b border-theme pb-2">
                <Server className="w-3 h-3 text-pilot-cyan" />
                <span className="font-medium text-theme-primary text-sm">{agent.agent_name}</span>
                <span className="text-xs text-theme-muted ml-auto">{agent.agent_region} • {agent.agent_provider}</span>
              </div>
              <div className="space-y-1 max-h-32 overflow-y-auto">
                {agent.results.slice(0, 10).map((result, idx) => (
                  <div key={idx} className="flex items-center justify-between text-xs">
                    <span className="text-theme-muted">{new Date(result.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}</span>
                    <div className="flex items-center gap-3">
                      {result.success ? (
                        <><span className={`font-mono ${getLatencyColor(result.latency_ms)}`}>{result.latency_ms?.toFixed(1)}ms</span><span className="text-status-healthy"><Check className="w-3 h-3" /></span></>
                      ) : (
                        <><span className="text-theme-muted font-mono">—</span><span className="text-pilot-red"><X className="w-3 h-3" /></span></>
                      )}
                    </div>
                  </div>
                ))}
              </div>
              {agent.results.length > 0 && (
                <div className="mt-2 pt-2 border-t border-theme flex items-center justify-between text-xs">
                  <span className="text-theme-muted">{agent.results.filter(r => r.success).length}/{agent.results.length} success</span>
                  {agent.results.some(r => r.success && r.latency_ms != null) && (
                    <span className="text-theme-muted">
                      Avg: {(agent.results.filter(r => r.success && r.latency_ms != null).reduce((sum, r) => sum + r.latency_ms, 0) / agent.results.filter(r => r.success && r.latency_ms != null).length).toFixed(1)}ms
                    </span>
                  )}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function TargetDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [target, setTarget] = useState(null);
  const [status, setStatus] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [tiers, setTiers] = useState([]);
  const [agents, setAgents] = useState([]);
  const [targetHistory, setTargetHistory] = useState([]);
  const [perAgentHistory, setPerAgentHistory] = useState([]);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [timeWindow, setTimeWindow] = useState('1h');
  const [chartMetric, setChartMetric] = useState('latency');
  const [visibleChartAgents, setVisibleChartAgents] = useState([]);
  const [showLiveView, setShowLiveView] = useState(false);
  const [showTargetModal, setShowTargetModal] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [mtrLoading, setMtrLoading] = useState(false);
  const [mtrResult, setMtrResult] = useState(null);
  const [showAgentSelector, setShowAgentSelector] = useState(false);
  const [selectedAgents, setSelectedAgents] = useState([]);

  const fetchTarget = async () => {
    try {
      setLoading(true);
      setError(null);
      const [targetRes, tiersRes, agentsRes, statusesRes] = await Promise.all([
        endpoints.getTarget(id),
        endpoints.listTiers(),
        endpoints.listAgents(),
        endpoints.getAllTargetStatuses(),
      ]);
      setTarget(targetRes);
      setTiers(tiersRes.tiers || []);
      setAgents((agentsRes.agents || []).filter(a => a.status === 'active'));
      const statusMap = {};
      (statusesRes.statuses || []).forEach(s => { statusMap[s.target_id] = s; });
      setStatus(statusMap[id] || null);
    } catch (err) {
      console.error('Failed to fetch target:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const fetchTargetHistory = useCallback(async (window = '1h') => {
    if (!id) return;
    setHistoryLoading(true);
    try {
      const [aggregatedRes, perAgentRes] = await Promise.all([
        endpoints.getTargetHistory(id, window),
        endpoints.getTargetHistoryByAgent(id, window),
      ]);
      setTargetHistory(aggregatedRes.history || []);
      setPerAgentHistory(perAgentRes.history || []);
    } catch (err) {
      console.error('Failed to fetch target history:', err);
      setTargetHistory([]);
      setPerAgentHistory([]);
    } finally {
      setHistoryLoading(false);
    }
  }, [id]);

  useEffect(() => {
    fetchTarget();
  }, [id]);

  useEffect(() => {
    if (target) {
      fetchTargetHistory(timeWindow);
      setMtrResult(null);
      setVisibleChartAgents([]);
    }
  }, [target, timeWindow, fetchTargetHistory]);

  const handleTriggerMTR = async (agentIds = []) => {
    if (!target) return;
    setMtrLoading(true);
    setMtrResult(null);
    setShowAgentSelector(false);
    try {
      const res = await endpoints.triggerMTR(target.id, agentIds);
      setMtrResult({ commandId: res.command_id, status: 'pending', message: res.message, expectedAgents: agentIds.length > 0 ? agentIds.length : agents.length });
      pollMTRResults(res.command_id);
    } catch (err) {
      console.error('Failed to trigger MTR:', err);
      setMtrResult({ error: err.message });
    } finally {
      setMtrLoading(false);
    }
  };

  const pollMTRResults = async (commandId) => {
    let attempts = 0;
    const maxAttempts = 30;

    const poll = async () => {
      attempts++;
      try {
        const res = await endpoints.getCommand(commandId);
        if (res.command?.status === 'completed' || res.results?.length > 0) {
          setMtrResult({ commandId, status: 'completed', results: res.results || [] });
          return;
        }
        if (attempts < maxAttempts) setTimeout(poll, 2000);
        else setMtrResult({ commandId, status: 'timeout', message: 'MTR request timed out' });
      } catch (err) {
        console.error('MTR poll error:', err);
        if (attempts < maxAttempts) setTimeout(poll, 2000);
      }
    };

    setTimeout(poll, 2000);
  };

  const handleDeleteTarget = async () => {
    if (!target) return;
    setDeleteLoading(true);
    try {
      await endpoints.deleteTarget(target.id);
      navigate('/targets');
    } catch (err) {
      console.error('Delete target failed:', err);
      alert('Failed to delete target: ' + err.message);
    } finally {
      setDeleteLoading(false);
    }
  };

  const getStatusColor = (s) => {
    switch (s) {
      case 'healthy': return 'healthy';
      case 'degraded': return 'degraded';
      case 'down': return 'down';
      default: return 'unknown';
    }
  };

  const tierColors = {
    infrastructure: 'bg-pilot-red/20 text-pilot-red',
    vip: 'bg-pilot-yellow/20 text-accent',
    standard: 'bg-pilot-cyan/20 text-pilot-cyan',
  };

  if (loading) {
    return (
      <>
        <PageHeader title="Loading..." breadcrumbs={[{ label: 'Targets', href: '/targets' }, { label: '...' }]} />
        <PageContent>
          <div className="flex items-center justify-center py-12">
            <RefreshCw className="w-6 h-6 animate-spin text-theme-muted" />
          </div>
        </PageContent>
      </>
    );
  }

  if (error || !target) {
    return (
      <>
        <PageHeader title="Target Not Found" breadcrumbs={[{ label: 'Targets', href: '/targets' }, { label: 'Error' }]} />
        <PageContent>
          <Card accent="red">
            <div className="flex items-center gap-3">
              <AlertTriangle className="w-6 h-6 text-pilot-red" />
              <div>
                <h3 className="font-medium text-theme-primary">Failed to load target</h3>
                <p className="text-sm text-theme-muted">{error || 'Target not found'}</p>
              </div>
              <Button variant="secondary" size="sm" onClick={() => navigate('/targets')} className="ml-auto">Back to Targets</Button>
            </div>
          </Card>
        </PageContent>
      </>
    );
  }

  const hasExpectedOutcome = target.expected_outcome?.should_succeed === false;

  return (
    <>
      <PageHeader
        title={target.ip}
        breadcrumbs={[{ label: 'Targets', href: '/targets' }, { label: target.ip }]}
        actions={
          <div className="flex gap-3">
            <Button variant="secondary" onClick={fetchTarget} className="gap-2">
              <RefreshCw className="w-4 h-4" />Refresh
            </Button>
          </div>
        }
      />

      <PageContent>
        <Card accent={status?.status === 'down' ? 'red' : status?.status === 'degraded' ? 'warning' : 'cyan'}>
          <div className="flex items-start justify-between mb-6">
            <div>
              <div className="flex items-center gap-3 mb-1">
                <Target className="w-6 h-6 text-pilot-cyan" />
                <h3 className="text-xl font-mono font-semibold text-theme-primary">{target.ip}</h3>
                <StatusBadge status={getStatusColor(status?.status)} label={status?.status || 'unknown'} />
                {hasExpectedOutcome && <span className="text-xs px-1.5 py-0.5 bg-surface-tertiary text-theme-muted rounded">Expected Fail</span>}
              </div>
              <p className="text-theme-muted">{target.tags?.device || target.tags?.subscriber_name || target.subscriber_id || 'No name'}</p>
            </div>
            <div className="flex gap-2">
              <div className="relative">
                <div className="flex">
                  <Button variant="secondary" size="sm" className="gap-1 rounded-r-none border-r-0" onClick={() => handleTriggerMTR([])} disabled={mtrLoading}>
                    {mtrLoading ? <RefreshCw className="w-3 h-3 animate-spin" /> : <ExternalLink className="w-3 h-3" />}MTR (All)
                  </Button>
                  <Button variant="secondary" size="sm" className="px-2 rounded-l-none" onClick={() => setShowAgentSelector(!showAgentSelector)} disabled={mtrLoading}>
                    <ChevronDown className={`w-3 h-3 transition-transform ${showAgentSelector ? 'rotate-180' : ''}`} />
                  </Button>
                </div>
                {showAgentSelector && (
                  <div className="absolute right-0 top-full mt-1 w-72 bg-surface-secondary border border-theme rounded-lg shadow-xl z-50">
                    <div className="p-3 border-b border-theme">
                      <div className="flex items-center justify-between mb-2">
                        <span className="text-sm font-medium text-theme-primary">Select Agents</span>
                        <button onClick={() => setShowAgentSelector(false)} className="text-theme-muted hover:text-theme-primary"><X className="w-4 h-4" /></button>
                      </div>
                      <div className="flex gap-2">
                        <button onClick={() => setSelectedAgents(agents.map(a => a.id))} className="text-xs text-pilot-cyan hover:text-pilot-cyan-light">Select All</button>
                        <span className="text-gray-600">|</span>
                        <button onClick={() => setSelectedAgents([])} className="text-xs text-pilot-cyan hover:text-pilot-cyan-light">Clear</button>
                      </div>
                    </div>
                    <div className="max-h-64 overflow-y-auto p-2">
                      {agents.length === 0 ? (
                        <div className="text-center py-4 text-theme-muted text-sm">No active agents</div>
                      ) : (
                        <div className="space-y-1">
                          {agents.map(agent => (
                            <button key={agent.id} onClick={() => setSelectedAgents(prev => prev.includes(agent.id) ? prev.filter(id => id !== agent.id) : [...prev, agent.id])} className={`w-full flex items-center gap-2 p-2 rounded text-sm text-left transition-colors ${selectedAgents.includes(agent.id) ? 'bg-pilot-cyan/20 text-theme-primary' : 'hover:bg-surface-tertiary text-theme-secondary'}`}>
                              <div className={`w-4 h-4 rounded border flex items-center justify-center ${selectedAgents.includes(agent.id) ? 'bg-pilot-cyan border-pilot-cyan' : 'border-gray-500'}`}>
                                {selectedAgents.includes(agent.id) && <Check className="w-3 h-3 text-neutral-900" />}
                              </div>
                              <Server className="w-3 h-3 text-theme-muted" />
                              <div className="flex-1 min-w-0">
                                <div className="font-medium truncate">{agent.name}</div>
                                <div className="text-xs text-theme-muted truncate">{agent.location || agent.region} • {agent.provider}</div>
                              </div>
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                    <div className="p-3 border-t border-theme">
                      <Button size="sm" className="w-full" onClick={() => handleTriggerMTR(selectedAgents)} disabled={selectedAgents.length === 0}>
                        Run MTR from {selectedAgents.length} Agent{selectedAgents.length !== 1 ? 's' : ''}
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* Metrics Grid */}
          <div className="grid grid-cols-2 md:grid-cols-6 gap-4 mb-6">
            <MetricCardCompact title="Tier" value={<span className={`px-2 py-0.5 rounded text-xs font-medium capitalize ${tierColors[target.tier] || 'bg-gray-500/20 text-theme-muted'}`}>{target.tier}</span>} />
            <MetricCardCompact title="Avg Latency" value={status?.avg_latency_ms != null ? `${status.avg_latency_ms.toFixed(1)}ms` : '—'} />
            <MetricCardCompact title="Min/Max" value={status?.min_latency_ms != null ? `${status.min_latency_ms.toFixed(0)}/${status.max_latency_ms?.toFixed(0)}ms` : '—'} />
            <MetricCardCompact title="Packet Loss" value={status?.packet_loss_pct != null ? `${status.packet_loss_pct.toFixed(1)}%` : '—'} />
            <MetricCardCompact title="Agents" value={status?.total_agents > 0 ? `${status.reachable_agents}/${status.total_agents}` : '—'} />
            <MetricCardCompact title="Probes" value={status?.probe_count?.toLocaleString() || '0'} />
          </div>

          {/* View Mode Toggle */}
          <div className="mb-6">
            <div className="flex items-center justify-between mb-3">
              <div className="flex items-center gap-4">
                <div className="flex bg-surface-primary rounded-lg p-0.5">
                  <button onClick={() => setShowLiveView(false)} className={`px-3 py-1 text-xs rounded-md transition-colors flex items-center gap-1.5 ${!showLiveView ? 'bg-pilot-cyan text-neutral-900 font-medium' : 'text-theme-muted hover:text-theme-primary'}`}>
                    <Activity className="w-3 h-3" />Charts
                  </button>
                  <button onClick={() => setShowLiveView(true)} className={`px-3 py-1 text-xs rounded-md transition-colors flex items-center gap-1.5 ${showLiveView ? 'bg-status-healthy text-neutral-900 font-medium' : 'text-theme-muted hover:text-theme-primary'}`}>
                    <Radio className="w-3 h-3" />Live
                  </button>
                </div>
              </div>

              {!showLiveView && (
                <div className="flex items-center gap-2">
                  <div className="flex bg-surface-primary rounded-lg p-0.5">
                    <button onClick={() => setChartMetric('latency')} className={`px-3 py-1 text-xs rounded-md transition-colors ${chartMetric === 'latency' ? 'bg-pilot-cyan text-neutral-900 font-medium' : 'text-theme-muted hover:text-theme-primary'}`}>Latency</button>
                    <button onClick={() => setChartMetric('loss')} className={`px-3 py-1 text-xs rounded-md transition-colors ${chartMetric === 'loss' ? 'bg-pilot-cyan text-neutral-900 font-medium' : 'text-theme-muted hover:text-theme-primary'}`}>Packet Loss</button>
                  </div>
                  <select value={timeWindow} onChange={(e) => setTimeWindow(e.target.value)} className="px-3 py-1 text-xs bg-surface-primary border border-theme rounded-lg text-theme-primary focus:outline-none focus:ring-2 focus:ring-pilot-cyan">
                    {timeWindows.map(tw => <option key={tw.value} value={tw.value}>{tw.label}</option>)}
                  </select>
                </div>
              )}
            </div>

            {showLiveView ? (
              <LiveView targetId={target.id} />
            ) : (
              <>
                {perAgentHistory.length > 0 && (
                  <div className="flex flex-wrap gap-2 mb-3">
                    {(() => {
                      const uniqueAgents = {};
                      perAgentHistory.forEach(p => {
                        if (!uniqueAgents[p.agent_id]) {
                          uniqueAgents[p.agent_id] = { id: p.agent_id, name: p.agent_name, color: agentColors[Object.keys(uniqueAgents).length % agentColors.length] };
                        }
                      });
                      return Object.values(uniqueAgents).map(agent => {
                        const isVisible = visibleChartAgents.length === 0 || visibleChartAgents.includes(agent.id);
                        return (
                          <button key={agent.id} onClick={() => {
                            if (visibleChartAgents.length === 0) setVisibleChartAgents([agent.id]);
                            else if (visibleChartAgents.includes(agent.id)) {
                              const newVisible = visibleChartAgents.filter(id => id !== agent.id);
                              setVisibleChartAgents(newVisible.length === 0 ? [] : newVisible);
                            } else setVisibleChartAgents([...visibleChartAgents, agent.id]);
                          }} className={`flex items-center gap-1.5 px-2 py-1 rounded text-xs transition-colors ${isVisible ? 'bg-surface-tertiary text-theme-primary' : 'bg-surface-primary text-theme-muted'}`}>
                            <span className="w-2 h-2 rounded-full" style={{ backgroundColor: isVisible ? agent.color : '#4B5563' }} />
                            {agent.name}
                          </button>
                        );
                      });
                    })()}
                    {visibleChartAgents.length > 0 && <button onClick={() => setVisibleChartAgents([])} className="text-xs text-pilot-cyan hover:text-pilot-cyan-light">Show All</button>}
                  </div>
                )}

                {historyLoading ? (
                  <div className="h-64 flex items-center justify-center"><RefreshCw className="w-6 h-6 animate-spin text-theme-muted" /></div>
                ) : (
                  <PerAgentChart data={perAgentHistory} visibleAgents={visibleChartAgents} metric={chartMetric} timeWindow={timeWindow} />
                )}
              </>
            )}
          </div>

          {/* MTR Results */}
          {mtrResult && (
            <div className="mb-6 border-t border-theme pt-4">
              <h4 className="text-sm font-medium text-theme-muted mb-3">MTR Results</h4>
              {mtrResult.error ? (
                <div className="text-pilot-red text-sm">{mtrResult.error}</div>
              ) : mtrResult.status === 'pending' ? (
                <div className="flex items-center gap-2 text-theme-muted"><RefreshCw className="w-4 h-4 animate-spin" /><span>Waiting for results from agents...</span></div>
              ) : mtrResult.status === 'timeout' ? (
                <div className="text-warning text-sm">{mtrResult.message}</div>
              ) : mtrResult.results?.length > 0 ? (
                <div className="space-y-4">
                  {mtrResult.results.map((result, idx) => (
                    <div key={idx} className="bg-surface-primary rounded-lg p-3">
                      <div className="flex items-center justify-between mb-3">
                        <div className="flex items-center gap-2">
                          <span className="text-theme-primary font-medium">{result.agent_name}</span>
                          <span className="text-theme-muted">→</span>
                          <span className="text-theme-muted font-mono">{result.payload?.target}</span>
                        </div>
                        <span className={`text-sm ${result.success ? 'text-status-healthy' : 'text-pilot-red'}`}>
                          {result.success ? (result.payload?.reached_dst ? 'Reached' : 'Unreachable') : 'Failed'}
                        </span>
                      </div>
                      {result.success && result.payload?.hops?.length > 0 ? (
                        <div className="overflow-x-auto">
                          <table className="w-full text-xs font-mono">
                            <thead>
                              <tr className="text-theme-muted border-b border-theme">
                                <th className="text-left py-1 pr-4">#</th>
                                <th className="text-left py-1 pr-4">Host</th>
                                <th className="text-right py-1 px-2">Loss%</th>
                                <th className="text-right py-1 px-2">Sent</th>
                                <th className="text-right py-1 px-2">Avg</th>
                                <th className="text-right py-1 px-2">Best</th>
                                <th className="text-right py-1 px-2">Worst</th>
                                <th className="text-right py-1 pl-2">StDev</th>
                              </tr>
                            </thead>
                            <tbody>
                              {result.payload.hops.map((hop) => (
                                <tr key={hop.number} className="text-theme-secondary border-b border-theme/50">
                                  <td className="py-1 pr-4 text-theme-muted">{hop.number}.</td>
                                  <td className="py-1 pr-4 text-theme-primary">{hop.host || '???'}</td>
                                  <td className={`text-right py-1 px-2 ${hop.loss_pct > 0 ? 'text-pilot-red' : ''}`}>{hop.loss_pct?.toFixed(1)}%</td>
                                  <td className="text-right py-1 px-2">{hop.sent}</td>
                                  <td className="text-right py-1 px-2">{hop.avg_ms?.toFixed(1)}</td>
                                  <td className="text-right py-1 px-2 text-status-healthy">{hop.best_ms?.toFixed(1)}</td>
                                  <td className="text-right py-1 px-2 text-accent">{hop.worst_ms?.toFixed(1)}</td>
                                  <td className="text-right py-1 pl-2">{hop.stddev_ms?.toFixed(1)}</td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                          {result.payload.reached_dst && (
                            <div className="mt-2 text-xs text-theme-muted">
                              Destination reached in {result.payload.total_hops} hops, avg latency: {result.payload.dst_latency_ms?.toFixed(1)}ms
                            </div>
                          )}
                        </div>
                      ) : result.success ? (
                        <div className="text-theme-muted text-sm">No hops recorded</div>
                      ) : null}
                      {result.error && <div className="text-pilot-red text-sm">{result.error}</div>}
                    </div>
                  ))}
                </div>
              ) : (
                <div className="text-theme-muted text-sm">No results yet</div>
              )}
            </div>
          )}

          {/* Tags */}
          {target.tags && Object.keys(target.tags).length > 0 && (
            <div className="border-t border-theme pt-4 mb-4">
              <h4 className="text-sm font-medium text-theme-muted mb-2">Tags</h4>
              <div className="flex flex-wrap gap-2">
                {Object.entries(target.tags || {}).map(([key, value]) => (
                  <span key={key} className="inline-flex items-center px-3 py-1 rounded-full text-sm bg-surface-tertiary">
                    <Tag className="w-3 h-3 text-pilot-cyan mr-2" />
                    <span className="text-theme-muted">{key}:</span>
                    <span className="ml-1 text-theme-primary">{value}</span>
                  </span>
                ))}
              </div>
            </div>
          )}

          {/* Expected Outcome Alert */}
          {hasExpectedOutcome && (
            <div className="border-t border-theme pt-4 mb-4">
              <h4 className="text-sm font-medium text-theme-muted mb-2">Expected Outcome</h4>
              <div className="bg-surface-primary rounded-lg p-3">
                <p className="text-sm text-theme-primary"><span className="text-accent">Security Test:</span> This target is expected to fail</p>
                {target.expected_outcome.alert_message && <p className="text-sm text-theme-muted mt-1">Alert on success: {target.expected_outcome.alert_message}</p>}
                {target.expected_outcome.alert_severity && <p className="text-sm text-theme-muted mt-1">Severity: {target.expected_outcome.alert_severity}</p>}
              </div>
            </div>
          )}

          <div className="flex gap-3">
            <Button variant="secondary" size="sm" onClick={() => setShowTargetModal(true)} className="gap-1"><Edit2 className="w-3 h-3" />Edit</Button>
            <Button variant="danger" size="sm" onClick={() => setShowDeleteConfirm(true)} className="gap-1"><Trash2 className="w-3 h-3" />Delete</Button>
          </div>
        </Card>

        {/* Target Edit Modal */}
        <TargetModal isOpen={showTargetModal} onClose={() => setShowTargetModal(false)} target={target} tiers={tiers} onSave={fetchTarget} />

        {/* Delete Confirmation */}
        <ConfirmModal
          isOpen={showDeleteConfirm}
          onClose={() => setShowDeleteConfirm(false)}
          onConfirm={handleDeleteTarget}
          title="Delete Target"
          message={`Are you sure you want to delete target ${target?.ip}? This will remove all historical data for this target.`}
          confirmText="Delete"
          confirmVariant="danger"
          loading={deleteLoading}
        />
      </PageContent>
    </>
  );
}
