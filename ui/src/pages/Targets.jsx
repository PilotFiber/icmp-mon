import { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import {
  Target,
  Plus,
  RefreshCw,
  Tag,
  ChevronRight,
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
  AreaChart,
  Area,
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from 'recharts';

import { PageHeader, PageContent } from '../components/Layout';
import { Card, CardTitle, CardContent } from '../components/Card';
import { MetricCard, MetricCardCompact } from '../components/MetricCard';
import { StatusBadge, StatusDot } from '../components/StatusBadge';
import { Button } from '../components/Button';
import { SearchInput, Select } from '../components/Input';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '../components/Table';
import { Modal, ModalFooter, ConfirmModal } from '../components/Modal';
import { formatRelativeTime } from '../lib/utils';
import { endpoints } from '../lib/api';

// Target Create/Edit Modal
function TargetModal({ isOpen, onClose, target, tiers, onSave }) {
  const [formData, setFormData] = useState({
    ip: '',
    tier: 'standard',
    tags: {},
  });
  const [tagInput, setTagInput] = useState({ key: '', value: '' });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);
  const isEditing = !!target;

  useEffect(() => {
    if (target) {
      setFormData({
        ip: target.ip || '',
        tier: target.tier || 'standard',
        tags: { ...(target.tags || {}) },
      });
    } else {
      setFormData({ ip: '', tier: 'standard', tags: {} });
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
      if (isEditing) {
        await endpoints.updateTarget(target.id, formData);
      } else {
        await endpoints.createTarget(formData);
      }
      onSave();
      onClose();
    } catch (err) {
      setError(err.message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={isEditing ? 'Edit Target' : 'Add Target'} size="md">
      <div className="space-y-4">
        {error && (
          <div className="p-3 bg-pilot-red/20 border border-pilot-red/30 rounded-lg text-pilot-red text-sm">
            {error}
          </div>
        )}

        <div>
          <label className="block text-sm font-medium text-gray-300 mb-1">IP Address</label>
          <input
            type="text"
            value={formData.ip}
            onChange={(e) => setFormData(prev => ({ ...prev, ip: e.target.value }))}
            placeholder="e.g., 192.168.1.1"
            disabled={isEditing}
            className="w-full px-3 py-2 bg-pilot-navy-dark border border-pilot-navy-light rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-pilot-cyan disabled:opacity-50"
          />
          {isEditing && <p className="text-xs text-gray-500 mt-1">IP address cannot be changed</p>}
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-300 mb-1">Tier</label>
          <select
            value={formData.tier}
            onChange={(e) => setFormData(prev => ({ ...prev, tier: e.target.value }))}
            className="w-full px-3 py-2 bg-pilot-navy-dark border border-pilot-navy-light rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
          >
            {tiers.map(tier => (
              <option key={tier.name} value={tier.name}>{tier.display_name || tier.name}</option>
            ))}
          </select>
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-300 mb-1">Tags</label>
          <div className="flex flex-wrap gap-2 mb-2">
            {Object.entries(formData.tags).map(([key, value]) => (
              <span
                key={key}
                className="inline-flex items-center gap-1 px-2 py-1 bg-pilot-navy-light rounded text-sm"
              >
                <span className="text-gray-400">{key}:</span>
                <span className="text-white">{value}</span>
                <button
                  onClick={() => handleRemoveTag(key)}
                  className="text-gray-400 hover:text-pilot-red ml-1"
                >
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
              className="flex-1 px-3 py-2 bg-pilot-navy-dark border border-pilot-navy-light rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-pilot-cyan text-sm"
            />
            <input
              type="text"
              value={tagInput.value}
              onChange={(e) => setTagInput(prev => ({ ...prev, value: e.target.value }))}
              placeholder="Value"
              className="flex-1 px-3 py-2 bg-pilot-navy-dark border border-pilot-navy-light rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-pilot-cyan text-sm"
            />
            <Button variant="secondary" size="sm" onClick={handleAddTag}>Add</Button>
          </div>
        </div>
      </div>

      <ModalFooter>
        <Button variant="ghost" onClick={onClose} disabled={saving}>Cancel</Button>
        <Button onClick={handleSubmit} disabled={saving || !formData.ip}>
          {saving ? 'Saving...' : (isEditing ? 'Update' : 'Create')}
        </Button>
      </ModalFooter>
    </Modal>
  );
}

const statuses = [
  { value: '', label: 'All Statuses' },
  { value: 'healthy', label: 'Healthy' },
  { value: 'degraded', label: 'Degraded' },
  { value: 'down', label: 'Down' },
  { value: 'unknown', label: 'Unknown' },
];

const timeWindows = [
  { value: '15m', label: '15 min' },
  { value: '1h', label: '1 hour' },
  { value: '6h', label: '6 hours' },
  { value: '24h', label: '24 hours' },
  { value: '168h', label: '7 days' },
  { value: '720h', label: '30 days' },
];

// Agent colors for chart lines
const agentColors = [
  '#6EDBE0', // cyan
  '#FC534E', // red
  '#F7B84B', // yellow
  '#4CAF50', // green
  '#9C27B0', // purple
  '#FF9800', // orange
  '#2196F3', // blue
  '#E91E63', // pink
];

function TagList({ tags }) {
  const entries = Object.entries(tags || {}).filter(([k]) => k !== 'expectedOutcome');
  if (entries.length === 0) return <span className="text-gray-500 text-xs">No tags</span>;

  return (
    <div className="flex flex-wrap gap-1">
      {entries.slice(0, 3).map(([key, value]) => (
        <span
          key={key}
          className="inline-flex items-center px-2 py-0.5 rounded text-xs bg-pilot-navy-light text-gray-300"
        >
          <span className="text-gray-500">{key}:</span>
          <span className="ml-1">{value}</span>
        </span>
      ))}
      {entries.length > 3 && (
        <span className="text-xs text-gray-500">+{entries.length - 3} more</span>
      )}
    </div>
  );
}

function PerAgentChart({ data, visibleAgents, metric = 'latency', timeWindow }) {
  if (!data || data.length === 0) {
    return (
      <div className="h-64 flex items-center justify-center text-gray-500">
        No history data available
      </div>
    );
  }

  // Get unique agents and assign colors
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

  // Group data by time, with each agent as a separate key
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

  // Sort by time and format
  const chartData = Object.values(timeMap)
    .sort((a, b) => new Date(a.time) - new Date(b.time))
    .map(point => {
      const timeObj = new Date(point.time);
      // Format time based on window size
      let timeLabel;
      if (timeWindow === '15m' || timeWindow === '1h') {
        timeLabel = timeObj.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
      } else if (timeWindow === '6h' || timeWindow === '24h') {
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
        <XAxis
          dataKey="timeLabel"
          stroke="#6B7280"
          fontSize={10}
          tickLine={false}
          axisLine={false}
          interval="preserveStartEnd"
        />
        <YAxis
          stroke="#6B7280"
          fontSize={10}
          tickLine={false}
          axisLine={false}
          tickFormatter={(v) => metric === 'latency' ? `${v?.toFixed(0) || 0}ms` : `${v?.toFixed(0) || 0}%`}
        />
        <Tooltip
          contentStyle={{
            backgroundColor: '#18284F',
            border: '1px solid #2A3D6B',
            borderRadius: '8px',
          }}
          labelStyle={{ color: '#9CA3AF' }}
          formatter={(value, name) => [
            metric === 'latency' ? `${value?.toFixed(1) || 0}ms` : `${value?.toFixed(1) || 0}%`,
            name
          ]}
        />
        <Legend
          wrapperStyle={{ paddingTop: 10 }}
          formatter={(value) => <span style={{ color: '#9CA3AF', fontSize: 11 }}>{value}</span>}
        />
        {agents.map(agent => {
          const agentKey = agent.name.replace(/[^a-zA-Z0-9]/g, '_');
          const isVisible = visibleAgents.length === 0 || visibleAgents.includes(agent.id);
          return (
            <Line
              key={agent.id}
              type="monotone"
              dataKey={agentKey}
              stroke={agent.color}
              strokeWidth={2}
              dot={false}
              name={agent.name}
              hide={!isVisible}
              connectNulls
            />
          );
        })}
      </LineChart>
    </ResponsiveContainer>
  );
}

function LiveView({ targetId, agents }) {
  const [liveData, setLiveData] = useState([]);
  const [isRunning, setIsRunning] = useState(true);
  const [lastUpdate, setLastUpdate] = useState(null);
  const [error, setError] = useState(null);
  const [viewMode, setViewMode] = useState('graph'); // 'cards' or 'graph'
  const [visibleAgents, setVisibleAgents] = useState([]);
  const seenKeys = useRef(new Set());

  // Reset data when target changes
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

        // Filter to only truly new results we haven't seen
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

  // Group results by agent for display
  const resultsByAgent = useMemo(() => {
    const grouped = {};
    liveData.forEach(result => {
      if (!grouped[result.agent_id]) {
        grouped[result.agent_id] = {
          agent_id: result.agent_id,
          agent_name: result.agent_name,
          agent_region: result.agent_region,
          agent_provider: result.agent_provider,
          results: [],
        };
      }
      grouped[result.agent_id].results.push(result);
    });
    // Sort results within each agent by time descending
    Object.values(grouped).forEach(agent => {
      agent.results.sort((a, b) => new Date(b.time) - new Date(a.time));
    });
    return Object.values(grouped);
  }, [liveData]);

  // Prepare chart data - group by time with agent values
  const chartData = useMemo(() => {
    if (liveData.length === 0) return { data: [], agents: [] };

    // Get unique agents with colors
    const agentMap = {};
    liveData.forEach(result => {
      if (!agentMap[result.agent_id]) {
        agentMap[result.agent_id] = {
          id: result.agent_id,
          name: result.agent_name,
          color: agentColors[Object.keys(agentMap).length % agentColors.length],
        };
      }
    });

    // Group by time bucket (round to nearest second)
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

    // Sort by time ascending
    const sortedData = Object.values(timeMap)
      .sort((a, b) => a.time - b.time)
      .map(point => ({
        ...point,
        timeLabel: new Date(point.time).toLocaleTimeString([], {
          hour: '2-digit',
          minute: '2-digit',
          second: '2-digit'
        }),
      }));

    return { data: sortedData, agents: Object.values(agentMap) };
  }, [liveData]);

  const getLatencyColor = (latency) => {
    if (latency === null) return 'text-gray-500';
    if (latency < 50) return 'text-status-healthy';
    if (latency < 100) return 'text-pilot-cyan';
    if (latency < 200) return 'text-pilot-yellow';
    return 'text-pilot-red';
  };

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-3">
          <h4 className="text-sm font-medium text-gray-400 flex items-center gap-2">
            <Radio className={`w-4 h-4 ${isRunning ? 'text-status-healthy animate-pulse' : 'text-gray-500'}`} />
            Live Probe Results
          </h4>
          {lastUpdate && (
            <span className="text-xs text-gray-500">
              Last update: {lastUpdate.toLocaleTimeString()}
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {/* View Mode Toggle */}
          <div className="flex bg-pilot-navy-dark rounded-lg p-0.5">
            <button
              onClick={() => setViewMode('graph')}
              className={`px-2 py-1 text-xs rounded-md transition-colors flex items-center gap-1 ${
                viewMode === 'graph'
                  ? 'bg-pilot-cyan text-pilot-navy font-medium'
                  : 'text-gray-400 hover:text-white'
              }`}
            >
              <TrendingUp className="w-3 h-3" />
              Graph
            </button>
            <button
              onClick={() => setViewMode('cards')}
              className={`px-2 py-1 text-xs rounded-md transition-colors flex items-center gap-1 ${
                viewMode === 'cards'
                  ? 'bg-pilot-cyan text-pilot-navy font-medium'
                  : 'text-gray-400 hover:text-white'
              }`}
            >
              <LayoutGrid className="w-3 h-3" />
              Cards
            </button>
          </div>
          {/* Pause/Resume */}
          <button
            onClick={() => setIsRunning(!isRunning)}
            className={`flex items-center gap-1.5 px-3 py-1 rounded text-xs transition-colors ${
              isRunning
                ? 'bg-pilot-red/20 text-pilot-red hover:bg-pilot-red/30'
                : 'bg-status-healthy/20 text-status-healthy hover:bg-status-healthy/30'
            }`}
          >
            {isRunning ? (
              <>
                <Pause className="w-3 h-3" />
                Pause
              </>
            ) : (
              <>
                <Play className="w-3 h-3" />
                Resume
              </>
            )}
          </button>
        </div>
      </div>

      {error && (
        <div className="text-pilot-red text-sm mb-3">{error}</div>
      )}

      {liveData.length === 0 ? (
        <div className="text-center py-8 text-gray-500">
          {isRunning ? (
            <div className="flex items-center justify-center gap-2">
              <RefreshCw className="w-4 h-4 animate-spin" />
              Waiting for probe results...
            </div>
          ) : (
            'No results in the last 60 seconds'
          )}
        </div>
      ) : viewMode === 'graph' ? (
        <div>
          {/* Agent visibility toggles */}
          <div className="flex flex-wrap gap-2 mb-3">
            {chartData.agents.map(agent => {
              const isVisible = visibleAgents.length === 0 || visibleAgents.includes(agent.id);
              return (
                <button
                  key={agent.id}
                  onClick={() => {
                    if (visibleAgents.length === 0) {
                      setVisibleAgents([agent.id]);
                    } else if (visibleAgents.includes(agent.id)) {
                      const newVisible = visibleAgents.filter(id => id !== agent.id);
                      setVisibleAgents(newVisible.length === 0 ? [] : newVisible);
                    } else {
                      setVisibleAgents([...visibleAgents, agent.id]);
                    }
                  }}
                  className={`flex items-center gap-1.5 px-2 py-1 rounded text-xs transition-colors ${
                    isVisible
                      ? 'bg-pilot-navy-light text-white'
                      : 'bg-pilot-navy-dark text-gray-500'
                  }`}
                >
                  <span
                    className="w-2 h-2 rounded-full"
                    style={{ backgroundColor: isVisible ? agent.color : '#4B5563' }}
                  />
                  {agent.name}
                </button>
              );
            })}
            {visibleAgents.length > 0 && (
              <button
                onClick={() => setVisibleAgents([])}
                className="text-xs text-pilot-cyan hover:text-pilot-cyan-light"
              >
                Show All
              </button>
            )}
          </div>

          {/* Real-time chart */}
          <ResponsiveContainer width="100%" height={280}>
            <LineChart data={chartData.data}>
              <XAxis
                dataKey="timeLabel"
                stroke="#6B7280"
                fontSize={10}
                tickLine={false}
                axisLine={false}
                interval="preserveStartEnd"
              />
              <YAxis
                stroke="#6B7280"
                fontSize={10}
                tickLine={false}
                axisLine={false}
                tickFormatter={(v) => `${v?.toFixed(0) || 0}ms`}
                domain={['auto', 'auto']}
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: '#18284F',
                  border: '1px solid #2A3D6B',
                  borderRadius: '8px',
                }}
                labelStyle={{ color: '#9CA3AF' }}
                formatter={(value, name) => [`${value?.toFixed(1) || 0}ms`, name]}
              />
              <Legend
                wrapperStyle={{ paddingTop: 10 }}
                formatter={(value) => <span style={{ color: '#9CA3AF', fontSize: 11 }}>{value}</span>}
              />
              {chartData.agents.map(agent => {
                const agentKey = agent.name.replace(/[^a-zA-Z0-9]/g, '_');
                const isVisible = visibleAgents.length === 0 || visibleAgents.includes(agent.id);
                return (
                  <Line
                    key={agent.id}
                    type="monotone"
                    dataKey={agentKey}
                    stroke={agent.color}
                    strokeWidth={2}
                    dot={{ r: 3, fill: agent.color }}
                    name={agent.name}
                    hide={!isVisible}
                    connectNulls
                    isAnimationActive={false}
                  />
                );
              })}
            </LineChart>
          </ResponsiveContainer>

          {/* Summary stats */}
          <div className="mt-3 grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-2">
            {resultsByAgent.map(agent => {
              const agentColor = chartData.agents.find(a => a.id === agent.agent_id)?.color || '#6B7280';
              const successResults = agent.results.filter(r => r.success && r.latency_ms != null);
              const avgLatency = successResults.length > 0
                ? successResults.reduce((sum, r) => sum + r.latency_ms, 0) / successResults.length
                : null;
              const minLatency = successResults.length > 0
                ? Math.min(...successResults.map(r => r.latency_ms))
                : null;
              const maxLatency = successResults.length > 0
                ? Math.max(...successResults.map(r => r.latency_ms))
                : null;
              return (
                <div key={agent.agent_id} className="bg-pilot-navy-dark rounded-lg p-2">
                  <div className="flex items-center gap-1.5 mb-1">
                    <span className="w-2 h-2 rounded-full" style={{ backgroundColor: agentColor }} />
                    <span className="text-xs font-medium text-white truncate">{agent.agent_name}</span>
                  </div>
                  <div className="text-xs text-gray-400">
                    <div>Avg: <span className="text-white font-mono">{avgLatency?.toFixed(1) || '—'}ms</span></div>
                    <div>Min/Max: <span className="text-white font-mono">{minLatency?.toFixed(0) || '—'}/{maxLatency?.toFixed(0) || '—'}ms</span></div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {resultsByAgent.map(agent => (
            <div
              key={agent.agent_id}
              className="bg-pilot-navy-dark rounded-lg p-3"
            >
              <div className="flex items-center gap-2 mb-2 border-b border-pilot-navy-light pb-2">
                <Server className="w-3 h-3 text-pilot-cyan" />
                <span className="font-medium text-white text-sm">{agent.agent_name}</span>
                <span className="text-xs text-gray-500 ml-auto">
                  {agent.agent_region} • {agent.agent_provider}
                </span>
              </div>
              <div className="space-y-1 max-h-32 overflow-y-auto">
                {agent.results.slice(0, 10).map((result, idx) => (
                  <div
                    key={idx}
                    className="flex items-center justify-between text-xs"
                  >
                    <span className="text-gray-500">
                      {new Date(result.time).toLocaleTimeString([], {
                        hour: '2-digit',
                        minute: '2-digit',
                        second: '2-digit'
                      })}
                    </span>
                    <div className="flex items-center gap-3">
                      {result.success ? (
                        <>
                          <span className={`font-mono ${getLatencyColor(result.latency_ms)}`}>
                            {result.latency_ms?.toFixed(1)}ms
                          </span>
                          <span className="text-status-healthy">
                            <Check className="w-3 h-3" />
                          </span>
                        </>
                      ) : (
                        <>
                          <span className="text-gray-500 font-mono">—</span>
                          <span className="text-pilot-red">
                            <X className="w-3 h-3" />
                          </span>
                        </>
                      )}
                    </div>
                  </div>
                ))}
              </div>
              {agent.results.length > 0 && (
                <div className="mt-2 pt-2 border-t border-pilot-navy-light flex items-center justify-between text-xs">
                  <span className="text-gray-500">
                    {agent.results.filter(r => r.success).length}/{agent.results.length} success
                  </span>
                  {agent.results.some(r => r.success && r.latency_ms != null) && (
                    <span className="text-gray-400">
                      Avg: {(
                        agent.results
                          .filter(r => r.success && r.latency_ms != null)
                          .reduce((sum, r) => sum + r.latency_ms, 0) /
                        agent.results.filter(r => r.success && r.latency_ms != null).length
                      ).toFixed(1)}ms
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

export function Targets() {
  const [targets, setTargets] = useState([]);
  const [targetStatuses, setTargetStatuses] = useState({});
  const [tiers, setTiers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [search, setSearch] = useState('');
  const [tierFilter, setTierFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [selectedTarget, setSelectedTarget] = useState(null);
  const [targetHistory, setTargetHistory] = useState([]);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [mtrLoading, setMtrLoading] = useState(false);
  const [mtrResult, setMtrResult] = useState(null);
  const [agents, setAgents] = useState([]);
  const [selectedAgents, setSelectedAgents] = useState([]);
  const [showAgentSelector, setShowAgentSelector] = useState(false);
  const [timeWindow, setTimeWindow] = useState('1h');
  const [chartMetric, setChartMetric] = useState('latency');
  const [visibleChartAgents, setVisibleChartAgents] = useState([]);
  const [perAgentHistory, setPerAgentHistory] = useState([]);
  const [showLiveView, setShowLiveView] = useState(false);
  const [showTargetModal, setShowTargetModal] = useState(false);
  const [editingTarget, setEditingTarget] = useState(null);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [deletingTarget, setDeletingTarget] = useState(null);
  const [deleteLoading, setDeleteLoading] = useState(false);

  const fetchData = async () => {
    try {
      setLoading(true);
      setError(null);
      const [targetsRes, tiersRes, statusesRes, agentsRes] = await Promise.all([
        endpoints.listTargets(),
        endpoints.listTiers(),
        endpoints.getAllTargetStatuses(),
        endpoints.listAgents(),
      ]);
      setTargets(targetsRes.targets || []);
      setTiers(tiersRes.tiers || []);
      setAgents((agentsRes.agents || []).filter(a => a.status === 'active'));

      // Convert statuses array to map by target_id
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

  const fetchTargetHistory = useCallback(async (targetId, window = '1h') => {
    if (!targetId) return;
    setHistoryLoading(true);
    try {
      const [aggregatedRes, perAgentRes] = await Promise.all([
        endpoints.getTargetHistory(targetId, window),
        endpoints.getTargetHistoryByAgent(targetId, window),
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
  }, []);

  const handleTriggerMTR = async (agentIds = []) => {
    if (!selectedTarget) return;
    setMtrLoading(true);
    setMtrResult(null);
    setShowAgentSelector(false);
    try {
      const res = await endpoints.triggerMTR(selectedTarget.id, agentIds);
      setMtrResult({
        commandId: res.command_id,
        status: 'pending',
        message: res.message,
        expectedAgents: agentIds.length > 0 ? agentIds.length : agents.length,
      });
      pollMTRResults(res.command_id);
    } catch (err) {
      console.error('Failed to trigger MTR:', err);
      setMtrResult({ error: err.message });
    } finally {
      setMtrLoading(false);
    }
  };

  const toggleAgentSelection = (agentId) => {
    setSelectedAgents(prev =>
      prev.includes(agentId)
        ? prev.filter(id => id !== agentId)
        : [...prev, agentId]
    );
  };

  const selectAllAgents = () => {
    setSelectedAgents(agents.map(a => a.id));
  };

  const clearAgentSelection = () => {
    setSelectedAgents([]);
  };

  const pollMTRResults = async (commandId) => {
    let attempts = 0;
    const maxAttempts = 30;

    const poll = async () => {
      attempts++;
      try {
        const res = await endpoints.getCommand(commandId);
        if (res.command?.status === 'completed' || res.results?.length > 0) {
          setMtrResult({
            commandId,
            status: 'completed',
            results: res.results || [],
          });
          return;
        }

        if (attempts < maxAttempts) {
          setTimeout(poll, 2000);
        } else {
          setMtrResult({
            commandId,
            status: 'timeout',
            message: 'MTR request timed out',
          });
        }
      } catch (err) {
        console.error('MTR poll error:', err);
        if (attempts < maxAttempts) {
          setTimeout(poll, 2000);
        }
      }
    };

    setTimeout(poll, 2000);
  };

  const handleDeleteTarget = async () => {
    if (!deletingTarget) return;
    setDeleteLoading(true);
    try {
      await endpoints.deleteTarget(deletingTarget.id);
      setShowDeleteConfirm(false);
      setDeletingTarget(null);
      setSelectedTarget(null);
      fetchData();
    } catch (err) {
      console.error('Delete target failed:', err);
      alert('Failed to delete target: ' + err.message);
    } finally {
      setDeleteLoading(false);
    }
  };

  const handleOpenEdit = (target) => {
    setEditingTarget(target);
    setShowTargetModal(true);
  };

  const handleOpenCreate = () => {
    setEditingTarget(null);
    setShowTargetModal(true);
  };

  const handleOpenDelete = (target) => {
    setDeletingTarget(target);
    setShowDeleteConfirm(true);
  };

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 10000);
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    if (selectedTarget) {
      fetchTargetHistory(selectedTarget.id, timeWindow);
      setMtrResult(null);
      setVisibleChartAgents([]);
      setShowLiveView(false);
    }
  }, [selectedTarget, timeWindow, fetchTargetHistory]);

  // Build dynamic tier filter options
  const tierOptions = useMemo(() => {
    return [
      { value: '', label: 'All Tiers' },
      ...tiers.map(t => ({ value: t.name, label: t.display_name || t.name })),
    ];
  }, [tiers]);

  // Merge targets with their status
  const targetsWithStatus = useMemo(() => {
    return targets.map(target => ({
      ...target,
      status: targetStatuses[target.id] || null,
    }));
  }, [targets, targetStatuses]);

  const filteredTargets = useMemo(() => {
    return targetsWithStatus.filter((target) => {
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
      if (statusFilter && target.status?.status !== statusFilter) return false;
      return true;
    });
  }, [targetsWithStatus, search, tierFilter, statusFilter]);

  const stats = useMemo(() => {
    const total = targetsWithStatus.length;
    const healthy = targetsWithStatus.filter(t => t.status?.status === 'healthy').length;
    const degraded = targetsWithStatus.filter(t => t.status?.status === 'degraded').length;
    const down = targetsWithStatus.filter(t => t.status?.status === 'down').length;
    const unknown = targetsWithStatus.filter(t => !t.status?.status || t.status?.status === 'unknown').length;
    return { total, healthy, degraded, down, unknown };
  }, [targetsWithStatus]);

  const tierColors = {
    infrastructure: 'bg-pilot-red/20 text-pilot-red',
    vip: 'bg-pilot-yellow/20 text-pilot-yellow',
    standard: 'bg-pilot-cyan/20 text-pilot-cyan',
  };

  const getStatusColor = (status) => {
    switch (status) {
      case 'healthy': return 'healthy';
      case 'degraded': return 'degraded';
      case 'down': return 'down';
      default: return 'unknown';
    }
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
                <h3 className="font-medium text-white">Failed to load targets</h3>
                <p className="text-sm text-gray-400">{error}</p>
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
            <Button className="gap-2" onClick={handleOpenCreate}>
              <Plus className="w-4 h-4" />
              Add Target
            </Button>
          </div>
        }
      />

      <PageContent>
        {/* Summary Cards */}
        <div className="grid grid-cols-1 md:grid-cols-5 gap-4 mb-6">
          <MetricCard
            title="Total Targets"
            value={stats.total.toLocaleString()}
            icon={Target}
          />
          <MetricCard
            title="Healthy"
            value={stats.healthy.toLocaleString()}
            status="healthy"
          />
          <MetricCard
            title="Degraded"
            value={stats.degraded.toLocaleString()}
            status={stats.degraded > 0 ? 'degraded' : 'healthy'}
          />
          <MetricCard
            title="Down"
            value={stats.down.toLocaleString()}
            status={stats.down > 0 ? 'down' : 'healthy'}
          />
          <MetricCard
            title="Unknown"
            value={stats.unknown.toLocaleString()}
          />
        </div>

        {/* Filters */}
        <Card className="mb-6">
          <div className="flex flex-wrap gap-4 items-center">
            <SearchInput
              value={search}
              onChange={setSearch}
              placeholder="Search IP or tags..."
              className="w-72"
            />
            <Select
              options={tierOptions}
              value={tierFilter}
              onChange={setTierFilter}
              className="w-40"
            />
            <Select
              options={statuses}
              value={statusFilter}
              onChange={setStatusFilter}
              className="w-40"
            />
          </div>
        </Card>

        {/* Target List */}
        <Card>
          {filteredTargets.length === 0 ? (
            <div className="text-center py-12 text-gray-400">
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
                  return (
                    <TableRow
                      key={target.id}
                      onClick={() => setSelectedTarget(target)}
                      className="cursor-pointer"
                    >
                      <TableCell>
                        <div className="flex items-center gap-3">
                          <StatusDot
                            status={getStatusColor(status?.status)}
                            pulse={status?.status === 'down'}
                          />
                          <span className="font-mono text-white">{target.ip}</span>
                          {hasExpectedOutcome && (
                            <span className="text-xs px-1.5 py-0.5 bg-pilot-navy-light text-gray-400 rounded">
                              Expected Fail
                            </span>
                          )}
                        </div>
                      </TableCell>
                      <TableCell>
                        <span className={`px-2 py-0.5 rounded text-xs font-medium capitalize ${tierColors[target.tier] || 'bg-gray-500/20 text-gray-400'}`}>
                          {target.tier}
                        </span>
                      </TableCell>
                      <TableCell>
                        <StatusBadge
                          status={getStatusColor(status?.status)}
                          label={status?.status || 'unknown'}
                          size="sm"
                        />
                      </TableCell>
                      <TableCell className="text-right font-mono">
                        {status?.avg_latency_ms != null
                          ? `${status.avg_latency_ms.toFixed(1)}ms`
                          : '—'}
                      </TableCell>
                      <TableCell className="text-right font-mono">
                        <span className={status?.packet_loss_pct > 0 ? 'text-warning' : ''}>
                          {status?.packet_loss_pct != null
                            ? `${status.packet_loss_pct.toFixed(1)}%`
                            : '—'}
                        </span>
                      </TableCell>
                      <TableCell>
                        {status?.total_agents > 0 ? (
                          <span className={status.reachable_agents < status.total_agents ? 'text-warning' : ''}>
                            {status.reachable_agents}/{status.total_agents}
                          </span>
                        ) : '—'}
                      </TableCell>
                      <TableCell className="text-gray-400 text-sm">
                        {status?.last_probe ? formatRelativeTime(status.last_probe) : '—'}
                      </TableCell>
                      <TableCell>
                        <ChevronRight className="w-4 h-4 text-gray-500" />
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </Card>

        {/* Target Detail Panel */}
        {selectedTarget && (
          <Card
            className="mt-6"
            accent={
              selectedTarget.status?.status === 'down' ? 'red' :
              selectedTarget.status?.status === 'degraded' ? 'warning' :
              'cyan'
            }
          >
            <div className="flex items-start justify-between mb-6">
              <div>
                <div className="flex items-center gap-3 mb-1">
                  <h3 className="text-xl font-mono font-semibold text-white">
                    {selectedTarget.ip}
                  </h3>
                  <StatusBadge
                    status={getStatusColor(selectedTarget.status?.status)}
                    label={selectedTarget.status?.status || 'unknown'}
                  />
                </div>
                <p className="text-gray-400">
                  {selectedTarget.tags?.device || selectedTarget.tags?.subscriber_name || selectedTarget.subscriber_id || 'No name'}
                </p>
              </div>
              <div className="flex gap-2">
                <div className="relative">
                  <div className="flex">
                    <Button
                      variant="secondary"
                      size="sm"
                      className="gap-1 rounded-r-none border-r-0"
                      onClick={() => handleTriggerMTR([])}
                      disabled={mtrLoading}
                    >
                      {mtrLoading ? (
                        <RefreshCw className="w-3 h-3 animate-spin" />
                      ) : (
                        <ExternalLink className="w-3 h-3" />
                      )}
                      MTR (All)
                    </Button>
                    <Button
                      variant="secondary"
                      size="sm"
                      className="px-2 rounded-l-none"
                      onClick={() => setShowAgentSelector(!showAgentSelector)}
                      disabled={mtrLoading}
                    >
                      <ChevronDown className={`w-3 h-3 transition-transform ${showAgentSelector ? 'rotate-180' : ''}`} />
                    </Button>
                  </div>
                  {showAgentSelector && (
                    <div className="absolute right-0 top-full mt-1 w-72 bg-pilot-navy border border-pilot-navy-light rounded-lg shadow-xl z-50">
                      <div className="p-3 border-b border-pilot-navy-light">
                        <div className="flex items-center justify-between mb-2">
                          <span className="text-sm font-medium text-white">Select Agents</span>
                          <button
                            onClick={() => setShowAgentSelector(false)}
                            className="text-gray-400 hover:text-white"
                          >
                            <X className="w-4 h-4" />
                          </button>
                        </div>
                        <div className="flex gap-2">
                          <button
                            onClick={selectAllAgents}
                            className="text-xs text-pilot-cyan hover:text-pilot-cyan-light"
                          >
                            Select All
                          </button>
                          <span className="text-gray-600">|</span>
                          <button
                            onClick={clearAgentSelection}
                            className="text-xs text-pilot-cyan hover:text-pilot-cyan-light"
                          >
                            Clear
                          </button>
                        </div>
                      </div>
                      <div className="max-h-64 overflow-y-auto p-2">
                        {agents.length === 0 ? (
                          <div className="text-center py-4 text-gray-400 text-sm">
                            No active agents
                          </div>
                        ) : (
                          <div className="space-y-1">
                            {agents.map(agent => (
                              <button
                                key={agent.id}
                                onClick={() => toggleAgentSelection(agent.id)}
                                className={`w-full flex items-center gap-2 p-2 rounded text-sm text-left transition-colors ${
                                  selectedAgents.includes(agent.id)
                                    ? 'bg-pilot-cyan/20 text-white'
                                    : 'hover:bg-pilot-navy-light text-gray-300'
                                }`}
                              >
                                <div className={`w-4 h-4 rounded border flex items-center justify-center ${
                                  selectedAgents.includes(agent.id)
                                    ? 'bg-pilot-cyan border-pilot-cyan'
                                    : 'border-gray-500'
                                }`}>
                                  {selectedAgents.includes(agent.id) && (
                                    <Check className="w-3 h-3 text-pilot-navy" />
                                  )}
                                </div>
                                <Server className="w-3 h-3 text-gray-500" />
                                <div className="flex-1 min-w-0">
                                  <div className="font-medium truncate">{agent.name}</div>
                                  <div className="text-xs text-gray-500 truncate">
                                    {agent.location || agent.region} • {agent.provider}
                                  </div>
                                </div>
                              </button>
                            ))}
                          </div>
                        )}
                      </div>
                      <div className="p-3 border-t border-pilot-navy-light">
                        <Button
                          size="sm"
                          className="w-full"
                          onClick={() => handleTriggerMTR(selectedAgents)}
                          disabled={selectedAgents.length === 0}
                        >
                          Run MTR from {selectedAgents.length} Agent{selectedAgents.length !== 1 ? 's' : ''}
                        </Button>
                      </div>
                    </div>
                  )}
                </div>
                <Button variant="secondary" size="sm">History</Button>
              </div>
            </div>

            {/* Metrics Grid */}
            <div className="grid grid-cols-2 md:grid-cols-6 gap-4 mb-6">
              <MetricCardCompact title="Tier" value={selectedTarget.tier} />
              <MetricCardCompact
                title="Avg Latency"
                value={selectedTarget.status?.avg_latency_ms != null
                  ? `${selectedTarget.status.avg_latency_ms.toFixed(1)}ms`
                  : '—'}
              />
              <MetricCardCompact
                title="Min/Max"
                value={selectedTarget.status?.min_latency_ms != null
                  ? `${selectedTarget.status.min_latency_ms.toFixed(0)}/${selectedTarget.status.max_latency_ms?.toFixed(0)}ms`
                  : '—'}
              />
              <MetricCardCompact
                title="Packet Loss"
                value={selectedTarget.status?.packet_loss_pct != null
                  ? `${selectedTarget.status.packet_loss_pct.toFixed(1)}%`
                  : '—'}
              />
              <MetricCardCompact
                title="Agents"
                value={selectedTarget.status?.total_agents > 0
                  ? `${selectedTarget.status.reachable_agents}/${selectedTarget.status.total_agents}`
                  : '—'}
              />
              <MetricCardCompact
                title="Probes"
                value={selectedTarget.status?.probe_count?.toLocaleString() || '0'}
              />
            </div>

            {/* View Mode Toggle */}
            <div className="mb-6">
              <div className="flex items-center justify-between mb-3">
                <div className="flex items-center gap-4">
                  {/* View Toggle */}
                  <div className="flex bg-pilot-navy-dark rounded-lg p-0.5">
                    <button
                      onClick={() => setShowLiveView(false)}
                      className={`px-3 py-1 text-xs rounded-md transition-colors flex items-center gap-1.5 ${
                        !showLiveView
                          ? 'bg-pilot-cyan text-pilot-navy font-medium'
                          : 'text-gray-400 hover:text-white'
                      }`}
                    >
                      <Activity className="w-3 h-3" />
                      Charts
                    </button>
                    <button
                      onClick={() => setShowLiveView(true)}
                      className={`px-3 py-1 text-xs rounded-md transition-colors flex items-center gap-1.5 ${
                        showLiveView
                          ? 'bg-status-healthy text-pilot-navy font-medium'
                          : 'text-gray-400 hover:text-white'
                      }`}
                    >
                      <Radio className="w-3 h-3" />
                      Live
                    </button>
                  </div>
                </div>

                {/* Chart Controls (only show when not in live view) */}
                {!showLiveView && (
                  <div className="flex items-center gap-2">
                    {/* Metric Toggle */}
                    <div className="flex bg-pilot-navy-dark rounded-lg p-0.5">
                      <button
                        onClick={() => setChartMetric('latency')}
                        className={`px-3 py-1 text-xs rounded-md transition-colors ${
                          chartMetric === 'latency'
                            ? 'bg-pilot-cyan text-pilot-navy font-medium'
                            : 'text-gray-400 hover:text-white'
                        }`}
                      >
                        Latency
                      </button>
                      <button
                        onClick={() => setChartMetric('loss')}
                        className={`px-3 py-1 text-xs rounded-md transition-colors ${
                          chartMetric === 'loss'
                            ? 'bg-pilot-cyan text-pilot-navy font-medium'
                            : 'text-gray-400 hover:text-white'
                        }`}
                      >
                        Packet Loss
                      </button>
                    </div>
                    {/* Time Window Selector */}
                    <select
                      value={timeWindow}
                      onChange={(e) => setTimeWindow(e.target.value)}
                      className="px-3 py-1 text-xs bg-pilot-navy-dark border border-pilot-navy-light rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
                    >
                      {timeWindows.map(tw => (
                        <option key={tw.value} value={tw.value}>{tw.label}</option>
                      ))}
                    </select>
                  </div>
                )}
              </div>

              {showLiveView ? (
                <LiveView targetId={selectedTarget.id} agents={agents} />
              ) : (
                <>
                  {/* Agent Visibility Toggles */}
                  {perAgentHistory.length > 0 && (
                    <div className="flex flex-wrap gap-2 mb-3">
                      {(() => {
                        const uniqueAgents = {};
                        perAgentHistory.forEach(p => {
                          if (!uniqueAgents[p.agent_id]) {
                            uniqueAgents[p.agent_id] = {
                              id: p.agent_id,
                              name: p.agent_name,
                              color: agentColors[Object.keys(uniqueAgents).length % agentColors.length],
                            };
                          }
                        });
                        return Object.values(uniqueAgents).map(agent => {
                          const isVisible = visibleChartAgents.length === 0 || visibleChartAgents.includes(agent.id);
                          return (
                            <button
                              key={agent.id}
                              onClick={() => {
                                if (visibleChartAgents.length === 0) {
                                  // First click: show only this agent
                                  setVisibleChartAgents([agent.id]);
                                } else if (visibleChartAgents.includes(agent.id)) {
                                  // Already visible: remove it (unless it's the only one)
                                  const newVisible = visibleChartAgents.filter(id => id !== agent.id);
                                  setVisibleChartAgents(newVisible.length === 0 ? [] : newVisible);
                                } else {
                                  // Not visible: add it
                                  setVisibleChartAgents([...visibleChartAgents, agent.id]);
                                }
                              }}
                              className={`flex items-center gap-1.5 px-2 py-1 rounded text-xs transition-colors ${
                                isVisible
                                  ? 'bg-pilot-navy-light text-white'
                                  : 'bg-pilot-navy-dark text-gray-500'
                              }`}
                            >
                              <span
                                className="w-2 h-2 rounded-full"
                                style={{ backgroundColor: isVisible ? agent.color : '#4B5563' }}
                              />
                              {agent.name}
                            </button>
                          );
                        });
                      })()}
                      {visibleChartAgents.length > 0 && (
                        <button
                          onClick={() => setVisibleChartAgents([])}
                          className="text-xs text-pilot-cyan hover:text-pilot-cyan-light"
                        >
                          Show All
                        </button>
                      )}
                    </div>
                  )}

                  {historyLoading ? (
                    <div className="h-64 flex items-center justify-center">
                      <RefreshCw className="w-6 h-6 animate-spin text-gray-500" />
                    </div>
                  ) : (
                    <PerAgentChart
                      data={perAgentHistory}
                      visibleAgents={visibleChartAgents}
                      metric={chartMetric}
                      timeWindow={timeWindow}
                    />
                  )}
                </>
              )}
            </div>

            {/* MTR Results */}
            {mtrResult && (
              <div className="mb-6 border-t border-pilot-navy-light pt-4">
                <h4 className="text-sm font-medium text-gray-400 mb-3">MTR Results</h4>
                {mtrResult.error ? (
                  <div className="text-pilot-red text-sm">{mtrResult.error}</div>
                ) : mtrResult.status === 'pending' ? (
                  <div className="flex items-center gap-2 text-gray-400">
                    <RefreshCw className="w-4 h-4 animate-spin" />
                    <span>Waiting for results from agents...</span>
                  </div>
                ) : mtrResult.status === 'timeout' ? (
                  <div className="text-warning text-sm">{mtrResult.message}</div>
                ) : mtrResult.results?.length > 0 ? (
                  <div className="space-y-4">
                    {mtrResult.results.map((result, idx) => (
                      <div key={idx} className="bg-pilot-navy-dark rounded-lg p-3">
                        <div className="flex items-center justify-between mb-3">
                          <div className="flex items-center gap-2">
                            <span className="text-white font-medium">{result.agent_name}</span>
                            <span className="text-gray-500">→</span>
                            <span className="text-gray-400 font-mono">{result.payload?.target}</span>
                          </div>
                          <span className={`text-sm ${result.success ? 'text-status-healthy' : 'text-pilot-red'}`}>
                            {result.success ? (result.payload?.reached_dst ? 'Reached' : 'Unreachable') : 'Failed'}
                          </span>
                        </div>
                        {result.success && result.payload?.hops?.length > 0 ? (
                          <div className="overflow-x-auto">
                            <table className="w-full text-xs font-mono">
                              <thead>
                                <tr className="text-gray-500 border-b border-pilot-navy-light">
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
                                  <tr key={hop.number} className="text-gray-300 border-b border-pilot-navy-light/50">
                                    <td className="py-1 pr-4 text-gray-500">{hop.number}.</td>
                                    <td className="py-1 pr-4 text-white">{hop.host || '???'}</td>
                                    <td className={`text-right py-1 px-2 ${hop.loss_pct > 0 ? 'text-pilot-red' : ''}`}>
                                      {hop.loss_pct?.toFixed(1)}%
                                    </td>
                                    <td className="text-right py-1 px-2">{hop.sent}</td>
                                    <td className="text-right py-1 px-2">{hop.avg_ms?.toFixed(1)}</td>
                                    <td className="text-right py-1 px-2 text-status-healthy">{hop.best_ms?.toFixed(1)}</td>
                                    <td className="text-right py-1 px-2 text-pilot-yellow">{hop.worst_ms?.toFixed(1)}</td>
                                    <td className="text-right py-1 pl-2">{hop.stddev_ms?.toFixed(1)}</td>
                                  </tr>
                                ))}
                              </tbody>
                            </table>
                            {result.payload.reached_dst && (
                              <div className="mt-2 text-xs text-gray-500">
                                Destination reached in {result.payload.total_hops} hops, avg latency: {result.payload.dst_latency_ms?.toFixed(1)}ms
                              </div>
                            )}
                          </div>
                        ) : result.success ? (
                          <div className="text-gray-500 text-sm">No hops recorded</div>
                        ) : null}
                        {result.error && (
                          <div className="text-pilot-red text-sm">{result.error}</div>
                        )}
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="text-gray-400 text-sm">No results yet</div>
                )}
              </div>
            )}

            {/* Tags */}
            {selectedTarget.tags && Object.keys(selectedTarget.tags).length > 0 && (
              <div className="border-t border-pilot-navy-light pt-4 mb-4">
                <h4 className="text-sm font-medium text-gray-400 mb-2">Tags</h4>
                <div className="flex flex-wrap gap-2">
                  {Object.entries(selectedTarget.tags || {}).map(([key, value]) => (
                    <span
                      key={key}
                      className="inline-flex items-center px-3 py-1 rounded-full text-sm bg-pilot-navy-light"
                    >
                      <Tag className="w-3 h-3 text-pilot-cyan mr-2" />
                      <span className="text-gray-400">{key}:</span>
                      <span className="ml-1 text-white">{value}</span>
                    </span>
                  ))}
                </div>
              </div>
            )}

            {/* Expected Outcome Alert */}
            {selectedTarget.expected_outcome?.should_succeed === false && (
              <div className="border-t border-pilot-navy-light pt-4 mb-4">
                <h4 className="text-sm font-medium text-gray-400 mb-2">Expected Outcome</h4>
                <div className="bg-pilot-navy-dark rounded-lg p-3">
                  <p className="text-sm text-white">
                    <span className="text-pilot-yellow">Security Test:</span> This target is expected to fail
                  </p>
                  {selectedTarget.expected_outcome.alert_message && (
                    <p className="text-sm text-gray-400 mt-1">
                      Alert on success: {selectedTarget.expected_outcome.alert_message}
                    </p>
                  )}
                  {selectedTarget.expected_outcome.alert_severity && (
                    <p className="text-sm text-gray-400 mt-1">
                      Severity: {selectedTarget.expected_outcome.alert_severity}
                    </p>
                  )}
                </div>
              </div>
            )}

            <div className="flex gap-3">
              <Button variant="secondary" size="sm" onClick={() => handleOpenEdit(selectedTarget)} className="gap-1">
                <Edit2 className="w-3 h-3" />
                Edit
              </Button>
              <Button variant="danger" size="sm" onClick={() => handleOpenDelete(selectedTarget)} className="gap-1">
                <Trash2 className="w-3 h-3" />
                Delete
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setSelectedTarget(null)}>Close</Button>
            </div>
          </Card>
        )}

        {/* Target Create/Edit Modal */}
        <TargetModal
          isOpen={showTargetModal}
          onClose={() => {
            setShowTargetModal(false);
            setEditingTarget(null);
          }}
          target={editingTarget}
          tiers={tiers}
          onSave={fetchData}
        />

        {/* Delete Confirmation */}
        <ConfirmModal
          isOpen={showDeleteConfirm}
          onClose={() => {
            setShowDeleteConfirm(false);
            setDeletingTarget(null);
          }}
          onConfirm={handleDeleteTarget}
          title="Delete Target"
          message={`Are you sure you want to delete target ${deletingTarget?.ip}? This will remove all historical data for this target.`}
          confirmText="Delete"
          confirmVariant="danger"
          loading={deleteLoading}
        />
      </PageContent>
    </>
  );
}
