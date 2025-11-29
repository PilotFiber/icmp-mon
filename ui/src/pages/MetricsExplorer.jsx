import { useState, useEffect, useMemo, useRef } from 'react';
import {
  BarChart3,
  Play,
  Clock,
  Server,
  Target,
  Layers,
  Filter,
  X,
  Plus,
  RefreshCw,
  Download,
  Info,
  TrendingUp,
  ChevronDown,
  Check,
} from 'lucide-react';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  Legend,
  CartesianGrid,
} from 'recharts';

import { PageHeader, PageContent } from '../components/Layout';
import { Card, CardTitle, CardContent } from '../components/Card';
import { Button } from '../components/Button';
import { Select } from '../components/Input';
import { endpoints } from '../lib/api';

// Color palette for multiple series
const SERIES_COLORS = [
  '#6EDBE0', // cyan
  '#F7B955', // yellow
  '#FC534E', // red
  '#8B5CF6', // purple
  '#10B981', // green
  '#F472B6', // pink
  '#60A5FA', // blue
  '#FBBF24', // amber
];

// Available metrics
const METRIC_OPTIONS = [
  { value: 'avg_latency', label: 'Avg Latency (ms)' },
  { value: 'p50_latency', label: 'P50 Latency (ms)' },
  { value: 'p95_latency', label: 'P95 Latency (ms)' },
  { value: 'p99_latency', label: 'P99 Latency (ms)' },
  { value: 'min_latency', label: 'Min Latency (ms)' },
  { value: 'max_latency', label: 'Max Latency (ms)' },
  { value: 'packet_loss', label: 'Packet Loss (%)' },
  { value: 'jitter', label: 'Jitter (ms)' },
  { value: 'success_rate', label: 'Success Rate (%)' },
  { value: 'probe_count', label: 'Probe Count' },
];

// Time range options
const TIME_RANGE_OPTIONS = [
  { value: '1h', label: 'Last 1 hour' },
  { value: '6h', label: 'Last 6 hours' },
  { value: '24h', label: 'Last 24 hours' },
  { value: '7d', label: 'Last 7 days' },
  { value: '30d', label: 'Last 30 days' },
  { value: '90d', label: 'Last 90 days' },
];

// Group by options
const GROUP_BY_OPTIONS = [
  { value: 'time', label: 'Time only (single series)' },
  { value: 'agent_region', label: 'By Agent Region' },
  { value: 'agent_provider', label: 'By Agent Provider' },
  { value: 'target_region', label: 'By Target Region' },
  { value: 'target_tier', label: 'By Target Tier' },
  { value: 'agent', label: 'By Individual Agent' },
];

// Filter operators for tag filtering
const TAG_OPERATORS = [
  { value: 'equals', label: '=' },
  { value: 'not_equals', label: '≠' },
  { value: 'in', label: 'in' },
  { value: 'not_in', label: 'not in' },
  { value: 'contains', label: 'contains' },
  { value: 'not_contains', label: '!contains' },
  { value: 'starts_with', label: 'starts with' },
  { value: 'regex', label: 'regex' },
];

// Multi-select dropdown component
function MultiSelectDropdown({ label, options, selected, onChange, placeholder = 'Select...' }) {
  const [isOpen, setIsOpen] = useState(false);
  const dropdownRef = useRef(null);

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target)) {
        setIsOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const toggleOption = (value) => {
    if (selected.includes(value)) {
      onChange(selected.filter((v) => v !== value));
    } else {
      onChange([...selected, value]);
    }
  };

  const selectedLabels = selected
    .map((v) => options.find((o) => o.value === v)?.label || v)
    .join(', ');

  return (
    <div className="space-y-1.5">
      {label && (
        <label className="block text-sm font-medium text-theme-secondary">{label}</label>
      )}
      <div className="relative" ref={dropdownRef}>
        <button
          type="button"
          onClick={() => setIsOpen(!isOpen)}
          className="w-full px-3 py-2 bg-surface-tertiary border border-theme rounded-lg text-left flex items-center justify-between focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
        >
          <span className={`truncate ${selected.length === 0 ? 'text-theme-muted' : 'text-theme-primary'}`}>
            {selected.length === 0 ? placeholder : selectedLabels}
          </span>
          <ChevronDown className={`w-4 h-4 text-theme-muted transition-transform ${isOpen ? 'rotate-180' : ''}`} />
        </button>

        {isOpen && (
          <div className="absolute z-50 w-full mt-1 bg-surface-secondary border border-theme rounded-lg shadow-lg max-h-60 overflow-auto">
            {options.map((option) => (
              <button
                key={option.value}
                type="button"
                onClick={() => toggleOption(option.value)}
                className="w-full px-3 py-2 text-left text-sm hover:bg-surface-tertiary flex items-center justify-between"
              >
                <span className="text-theme-primary">{option.label}</span>
                {selected.includes(option.value) && (
                  <Check className="w-4 h-4 text-pilot-cyan" />
                )}
              </button>
            ))}
          </div>
        )}
      </div>
      {/* Selected chips below dropdown */}
      {selected.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {selected.map((value) => {
            const option = options.find((o) => o.value === value);
            return (
              <span
                key={value}
                className="inline-flex items-center gap-1 px-2 py-0.5 bg-pilot-yellow/20 text-pilot-yellow rounded text-xs"
              >
                {option?.label || value}
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    toggleOption(value);
                  }}
                  className="hover:text-white"
                >
                  <X className="w-3 h-3" />
                </button>
              </span>
            );
          })}
        </div>
      )}
    </div>
  );
}

// Searchable dropdown for tag keys with text input fallback
function SearchableKeyDropdown({ value, onChange, options, loading, placeholder = 'Select or type key...' }) {
  const [isOpen, setIsOpen] = useState(false);
  const [search, setSearch] = useState(value);
  const dropdownRef = useRef(null);
  const inputRef = useRef(null);

  useEffect(() => {
    setSearch(value);
  }, [value]);

  useEffect(() => {
    const handleClickOutside = (event) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target)) {
        setIsOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const filteredOptions = options.filter((opt) =>
    opt.toLowerCase().includes(search.toLowerCase())
  );

  const handleInputChange = (e) => {
    const newValue = e.target.value;
    setSearch(newValue);
    onChange(newValue);
    if (!isOpen) setIsOpen(true);
  };

  const selectOption = (opt) => {
    setSearch(opt);
    onChange(opt);
    setIsOpen(false);
  };

  return (
    <div className="relative" ref={dropdownRef}>
      <div className="relative">
        <input
          ref={inputRef}
          type="text"
          value={search}
          onChange={handleInputChange}
          onFocus={() => setIsOpen(true)}
          placeholder={loading ? 'Loading...' : placeholder}
          disabled={loading}
          className="w-full px-2 py-1.5 pr-7 bg-surface-tertiary border border-theme rounded-lg text-theme-primary text-sm placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan disabled:opacity-50"
        />
        <button
          type="button"
          onClick={() => setIsOpen(!isOpen)}
          className="absolute right-1.5 top-1/2 -translate-y-1/2 p-0.5 text-theme-muted hover:text-theme-primary"
        >
          <ChevronDown className={`w-4 h-4 transition-transform ${isOpen ? 'rotate-180' : ''}`} />
        </button>
      </div>

      {isOpen && filteredOptions.length > 0 && (
        <div className="absolute z-50 w-full mt-1 bg-surface-secondary border border-theme rounded-lg shadow-lg max-h-48 overflow-auto">
          {filteredOptions.map((opt) => (
            <button
              key={opt}
              type="button"
              onClick={() => selectOption(opt)}
              className={`w-full px-3 py-1.5 text-left text-sm hover:bg-surface-tertiary ${
                opt === value ? 'bg-surface-tertiary text-pilot-cyan' : 'text-theme-primary'
              }`}
            >
              {opt}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// Tag filter input component with searchable dropdown for keys and operator selection
function TagFilterInput({ label, filters, onChange, availableKeys, loading }) {
  const [selectedKey, setSelectedKey] = useState('');
  const [operator, setOperator] = useState('equals');
  const [value, setValue] = useState('');

  const addFilter = () => {
    if (selectedKey && value) {
      onChange([...filters, { key: selectedKey, operator, value }]);
      setSelectedKey('');
      setValue('');
    }
  };

  const removeFilter = (index) => {
    onChange(filters.filter((_, i) => i !== index));
  };

  const getOperatorLabel = (op) => {
    return TAG_OPERATORS.find((o) => o.value === op)?.label || op;
  };

  return (
    <div className="space-y-2">
      <label className="block text-sm font-medium text-theme-secondary">{label}</label>
      <div className="flex flex-col gap-2">
        {/* Row 1: Key dropdown and operator */}
        <div className="flex gap-2">
          <div className="flex-1 min-w-0">
            <SearchableKeyDropdown
              value={selectedKey}
              onChange={setSelectedKey}
              options={availableKeys}
              loading={loading}
              placeholder="Select or type key..."
            />
          </div>
          <select
            value={operator}
            onChange={(e) => setOperator(e.target.value)}
            className="w-auto px-2 py-1.5 bg-surface-tertiary border border-theme rounded-lg text-theme-primary text-sm focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
          >
            {TAG_OPERATORS.map((op) => (
              <option key={op.value} value={op.value}>
                {op.label}
              </option>
            ))}
          </select>
        </div>

        {/* Row 2: Value input and add button */}
        <div className="flex gap-2">
          <input
            type="text"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder={operator === 'in' || operator === 'not_in' ? 'val1, val2, val3' : 'value'}
            className="flex-1 min-w-0 px-2 py-1.5 bg-surface-tertiary border border-theme rounded-lg text-theme-primary text-sm placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
            onKeyDown={(e) => e.key === 'Enter' && addFilter()}
          />
          <button
            onClick={addFilter}
            disabled={!selectedKey || !value}
            className="px-3 py-1.5 bg-pilot-cyan text-neutral-900 rounded-lg text-sm font-medium hover:bg-pilot-cyan/80 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1"
          >
            <Plus className="w-4 h-4" />
            Add
          </button>
        </div>
      </div>

      {/* Active filters */}
      {filters.length > 0 && (
        <div className="flex flex-wrap gap-1.5 mt-2">
          {filters.map((f, idx) => (
            <span
              key={idx}
              className="inline-flex items-center gap-1 px-2 py-1 bg-surface-tertiary rounded text-xs text-theme-secondary max-w-full"
            >
              <span className="text-pilot-cyan truncate">{f.key}</span>
              <span className="text-theme-muted">{getOperatorLabel(f.operator)}</span>
              <span className="truncate">{f.value}</span>
              <button onClick={() => removeFilter(idx)} className="ml-1 hover:text-theme-primary flex-shrink-0">
                <X className="w-3 h-3" />
              </button>
            </span>
          ))}
        </div>
      )}

      {/* Helper text for operators */}
      {(operator === 'in' || operator === 'not_in') && (
        <p className="text-xs text-theme-muted">Use comma-separated values for multiple matches</p>
      )}
    </div>
  );
}

// Legacy tag input component (for agent tags which don't have predefined keys)
function TagInput({ label, tags, onChange }) {
  const [inputValue, setInputValue] = useState('');
  const [inputKey, setInputKey] = useState('');

  const addTag = () => {
    if (inputKey && inputValue) {
      onChange({ ...tags, [inputKey]: inputValue });
      setInputKey('');
      setInputValue('');
    }
  };

  const removeTag = (key) => {
    const newTags = { ...tags };
    delete newTags[key];
    onChange(newTags);
  };

  return (
    <div className="space-y-2">
      <label className="block text-sm font-medium text-theme-secondary">{label}</label>
      <div className="grid grid-cols-[1fr_1fr_auto] gap-2">
        <input
          type="text"
          value={inputKey}
          onChange={(e) => setInputKey(e.target.value)}
          placeholder="key"
          className="w-full min-w-0 px-2 py-1.5 bg-surface-tertiary border border-theme rounded-lg text-theme-primary text-sm placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
        />
        <input
          type="text"
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
          placeholder="value"
          className="w-full min-w-0 px-2 py-1.5 bg-surface-tertiary border border-theme rounded-lg text-theme-primary text-sm placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
          onKeyDown={(e) => e.key === 'Enter' && addTag()}
        />
        <button
          onClick={addTag}
          disabled={!inputKey || !inputValue}
          className="p-1.5 bg-surface-tertiary rounded-lg text-theme-muted hover:text-theme-primary disabled:opacity-50 flex-shrink-0"
        >
          <Plus className="w-4 h-4" />
        </button>
      </div>
      {Object.keys(tags).length > 0 && (
        <div className="flex flex-wrap gap-1.5 mt-2">
          {Object.entries(tags).map(([key, value]) => (
            <span
              key={key}
              className="inline-flex items-center gap-1 px-2 py-1 bg-surface-tertiary rounded text-xs text-theme-secondary max-w-full"
            >
              <span className="text-pilot-cyan truncate">{key}</span>
              <span className="text-theme-muted">=</span>
              <span className="truncate">{value}</span>
              <button onClick={() => removeTag(key)} className="ml-1 hover:text-theme-primary flex-shrink-0">
                <X className="w-3 h-3" />
              </button>
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

// Multi-select for metrics using dropdown
function MetricSelector({ selected, onChange }) {
  return (
    <MultiSelectDropdown
      label="Metrics"
      options={METRIC_OPTIONS}
      selected={selected}
      onChange={onChange}
      placeholder="Select metrics..."
    />
  );
}

// Multi-select chips for regions/providers/tiers
function ChipSelector({ label, options, selected, onChange, loading }) {
  const toggleOption = (value) => {
    if (selected.includes(value)) {
      onChange(selected.filter((v) => v !== value));
    } else {
      onChange([...selected, value]);
    }
  };

  if (loading) {
    return (
      <div className="space-y-2">
        <label className="block text-sm font-medium text-theme-secondary">{label}</label>
        <div className="text-theme-muted text-sm">Loading...</div>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      <label className="block text-sm font-medium text-theme-secondary">{label}</label>
      <div className="flex flex-wrap gap-2">
        {options.length === 0 ? (
          <span className="text-theme-muted text-sm">No options available</span>
        ) : (
          options.map((option) => (
            <button
              key={option}
              onClick={() => toggleOption(option)}
              className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
                selected.includes(option)
                  ? 'bg-pilot-cyan text-neutral-900'
                  : 'bg-surface-tertiary text-theme-secondary hover:text-theme-primary'
              }`}
            >
              {option}
            </button>
          ))
        )}
      </div>
    </div>
  );
}

export function MetricsExplorer() {
  // Query state
  const [timeRange, setTimeRange] = useState('1h');
  const [selectedMetrics, setSelectedMetrics] = useState(['avg_latency', 'packet_loss']);
  const [groupBy, setGroupBy] = useState('time');

  // Agent filter state
  const [agentRegions, setAgentRegions] = useState([]);
  const [agentProviders, setAgentProviders] = useState([]);
  const [agentTagFilters, setAgentTagFilters] = useState([]); // Array of {key, operator, value}

  // Target filter state
  const [targetTiers, setTargetTiers] = useState([]);
  const [targetRegions, setTargetRegions] = useState([]);
  const [targetTagFilters, setTargetTagFilters] = useState([]); // Array of {key, operator, value}

  // Available options (from API)
  const [availableRegions, setAvailableRegions] = useState([]);
  const [availableProviders, setAvailableProviders] = useState([]);
  const [availableTiers, setAvailableTiers] = useState([]);
  const [availableTargetRegions, setAvailableTargetRegions] = useState([]);
  const [availableTargetTagKeys, setAvailableTargetTagKeys] = useState([]);
  const [availableAgentTagKeys, setAvailableAgentTagKeys] = useState([]);
  const [optionsLoading, setOptionsLoading] = useState(true);

  // Results state
  const [result, setResult] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  // Fetch available options on mount
  useEffect(() => {
    const fetchOptions = async () => {
      try {
        setOptionsLoading(true);
        const [agentsRes, tiersRes, targetTagKeysRes, subnetsRes] = await Promise.all([
          endpoints.listAgents(),
          endpoints.listTiers(),
          endpoints.getTargetTagKeys().catch(() => ({ keys: [] })), // Fallback if endpoint unavailable
          endpoints.listSubnets().catch(() => ({ subnets: [] })), // Fallback if endpoint unavailable
        ]);

        const agents = agentsRes.agents || [];
        const tiers = tiersRes.tiers || [];
        const targetTagKeys = targetTagKeysRes.keys || [];
        const subnets = subnetsRes.subnets || [];

        // Extract unique regions and providers from agents
        const regions = [...new Set(agents.map((a) => a.region).filter(Boolean))];
        const providers = [...new Set(agents.map((a) => a.provider).filter(Boolean))];
        const tierNames = tiers.map((t) => t.name);

        // Extract unique regions from subnets (for target regions)
        const targetRegions = [...new Set(subnets.map((s) => s.region).filter(Boolean))].sort();

        // Extract unique tag keys from agents
        const agentTagKeys = [...new Set(
          agents.flatMap((a) => Object.keys(a.tags || {}))
        )].sort();

        setAvailableRegions(regions);
        setAvailableProviders(providers);
        setAvailableTiers(tierNames);
        setAvailableTargetRegions(targetRegions);
        setAvailableTargetTagKeys(targetTagKeys);
        setAvailableAgentTagKeys(agentTagKeys);
      } catch (err) {
        console.error('Failed to fetch options:', err);
      } finally {
        setOptionsLoading(false);
      }
    };

    fetchOptions();
  }, []);

  // Build query object
  const buildQuery = () => {
    const query = {
      time_range: { window: timeRange },
      metrics: selectedMetrics,
      group_by: groupBy === 'time' ? ['time'] : ['time', groupBy],
    };

    // Add agent filter if any filters are set
    if (agentRegions.length > 0 || agentProviders.length > 0 || agentTagFilters.length > 0) {
      query.agent_filter = {};
      if (agentRegions.length > 0) query.agent_filter.regions = agentRegions;
      if (agentProviders.length > 0) query.agent_filter.providers = agentProviders;
      if (agentTagFilters.length > 0) query.agent_filter.tag_filters = agentTagFilters;
    }

    // Add target filter if any filters are set
    if (targetTiers.length > 0 || targetRegions.length > 0 || targetTagFilters.length > 0) {
      query.target_filter = {};
      if (targetTiers.length > 0) query.target_filter.tiers = targetTiers;
      if (targetRegions.length > 0) query.target_filter.regions = targetRegions;
      if (targetTagFilters.length > 0) query.target_filter.tag_filters = targetTagFilters;
    }

    return query;
  };

  // Execute query
  const executeQuery = async () => {
    if (selectedMetrics.length === 0) {
      setError('Please select at least one metric');
      return;
    }

    try {
      setLoading(true);
      setError(null);

      const query = buildQuery();
      const res = await endpoints.queryMetrics(query);
      setResult(res);
    } catch (err) {
      console.error('Query failed:', err);
      setError(err.message || 'Query failed');
    } finally {
      setLoading(false);
    }
  };

  // Format chart data
  const chartData = useMemo(() => {
    if (!result || !result.series || result.series.length === 0) return [];

    // If single series, flatten points
    if (result.series.length === 1) {
      return result.series[0].points.map((point) => ({
        time: new Date(point.time).toLocaleString([], {
          month: 'short',
          day: 'numeric',
          hour: '2-digit',
          minute: '2-digit',
        }),
        ...Object.fromEntries(
          selectedMetrics.map((m) => [m, point[m.replace('_latency', '_latency').replace('_', '')] ?? point[m]])
        ),
        avg_latency: point.avg_latency,
        min_latency: point.min_latency,
        max_latency: point.max_latency,
        p50_latency: point.p50_latency,
        p95_latency: point.p95_latency,
        p99_latency: point.p99_latency,
        packet_loss: point.packet_loss,
        jitter: point.jitter,
        success_rate: point.success_rate,
        probe_count: point.probe_count,
      }));
    }

    // Multiple series - merge by time
    const timeMap = new Map();

    result.series.forEach((series, idx) => {
      const seriesName = series.agent_region || series.agent_provider || series.agent_name || series.target_tier || `Series ${idx + 1}`;

      series.points.forEach((point) => {
        const timeKey = point.time;
        if (!timeMap.has(timeKey)) {
          timeMap.set(timeKey, {
            time: new Date(point.time).toLocaleString([], {
              month: 'short',
              day: 'numeric',
              hour: '2-digit',
              minute: '2-digit',
            }),
          });
        }

        const entry = timeMap.get(timeKey);
        // Store each metric with series name prefix
        selectedMetrics.forEach((metric) => {
          const value = point[metric];
          if (value !== undefined && value !== null) {
            entry[`${seriesName}_${metric}`] = value;
          }
        });
      });
    });

    return Array.from(timeMap.values()).sort((a, b) =>
      new Date(a.time) - new Date(b.time)
    );
  }, [result, selectedMetrics]);

  // Get series names for legend
  const seriesNames = useMemo(() => {
    if (!result || !result.series) return [];
    return result.series.map((s, idx) =>
      s.agent_region || s.agent_provider || s.agent_name || s.target_region || s.target_tier || `Series ${idx + 1}`
    );
  }, [result]);

  // Clear all filters
  const clearFilters = () => {
    setAgentRegions([]);
    setAgentProviders([]);
    setAgentTagFilters([]);
    setTargetTiers([]);
    setTargetRegions([]);
    setTargetTagFilters([]);
  };

  const hasFilters = agentRegions.length > 0 || agentProviders.length > 0 ||
    agentTagFilters.length > 0 || targetTiers.length > 0 ||
    targetRegions.length > 0 || targetTagFilters.length > 0;

  return (
    <>
      <PageHeader
        title="Metrics Explorer"
        description="Query and visualize probe metrics with flexible filtering by agent and target attributes"
        icon={BarChart3}
        actions={
          <Button onClick={executeQuery} disabled={loading || selectedMetrics.length === 0}>
            {loading ? (
              <RefreshCw className="w-4 h-4 mr-2 animate-spin" />
            ) : (
              <Play className="w-4 h-4 mr-2" />
            )}
            Run Query
          </Button>
        }
      />

      <PageContent>
        <div className="grid grid-cols-1 xl:grid-cols-3 gap-6">
          {/* Query Builder Panel */}
          <div className="xl:col-span-1 space-y-4 min-w-0">
            <Card>
              <CardTitle className="flex items-center gap-2">
                <Filter className="w-4 h-4" />
                Query Builder
                {hasFilters && (
                  <button
                    onClick={clearFilters}
                    className="ml-auto text-xs text-theme-muted hover:text-theme-primary"
                  >
                    Clear all
                  </button>
                )}
              </CardTitle>
              <CardContent className="space-y-6">
                {/* Time Range */}
                <Select
                  label="Time Range"
                  options={TIME_RANGE_OPTIONS}
                  value={timeRange}
                  onChange={setTimeRange}
                />

                {/* Group By */}
                <Select
                  label="Group By"
                  options={GROUP_BY_OPTIONS}
                  value={groupBy}
                  onChange={setGroupBy}
                />

                {/* Metrics */}
                <MetricSelector
                  selected={selectedMetrics}
                  onChange={setSelectedMetrics}
                />
              </CardContent>
            </Card>

            {/* Agent Filters */}
            <Card>
              <CardTitle className="flex items-center gap-2">
                <Server className="w-4 h-4" />
                Agent Filters
              </CardTitle>
              <CardContent className="space-y-4">
                <ChipSelector
                  label="Regions"
                  options={availableRegions}
                  selected={agentRegions}
                  onChange={setAgentRegions}
                  loading={optionsLoading}
                />

                <ChipSelector
                  label="Providers"
                  options={availableProviders}
                  selected={agentProviders}
                  onChange={setAgentProviders}
                  loading={optionsLoading}
                />

                <TagFilterInput
                  label="Tag Filters"
                  filters={agentTagFilters}
                  onChange={setAgentTagFilters}
                  availableKeys={availableAgentTagKeys}
                  loading={optionsLoading}
                />
              </CardContent>
            </Card>

            {/* Target Filters */}
            <Card>
              <CardTitle className="flex items-center gap-2">
                <Target className="w-4 h-4" />
                Target Filters
              </CardTitle>
              <CardContent className="space-y-4">
                <ChipSelector
                  label="Regions"
                  options={availableTargetRegions}
                  selected={targetRegions}
                  onChange={setTargetRegions}
                  loading={optionsLoading}
                />

                <ChipSelector
                  label="Tiers"
                  options={availableTiers}
                  selected={targetTiers}
                  onChange={setTargetTiers}
                  loading={optionsLoading}
                />

                <TagFilterInput
                  label="Tag Filters"
                  filters={targetTagFilters}
                  onChange={setTargetTagFilters}
                  availableKeys={availableTargetTagKeys}
                  loading={optionsLoading}
                />
              </CardContent>
            </Card>
          </div>

          {/* Results Panel */}
          <div className="xl:col-span-2 space-y-4 min-w-0">
            {/* Error */}
            {error && (
              <Card className="border-pilot-red">
                <CardContent>
                  <div className="flex items-center gap-2 text-pilot-red">
                    <Info className="w-5 h-5" />
                    <span>{error}</span>
                  </div>
                </CardContent>
              </Card>
            )}

            {/* Chart */}
            <Card>
              <CardTitle className="flex items-center justify-between">
                <span className="flex items-center gap-2">
                  <TrendingUp className="w-4 h-4" />
                  Results
                </span>
                {result && (
                  <span className="text-xs font-normal text-theme-muted">
                    {result.total_points} data points from {result.aggregate_table}
                  </span>
                )}
              </CardTitle>
              <CardContent>
                {!result ? (
                  <div className="h-80 flex items-center justify-center text-theme-muted">
                    <div className="text-center">
                      <BarChart3 className="w-12 h-12 mx-auto mb-3 opacity-50" />
                      <p>Run a query to see results</p>
                    </div>
                  </div>
                ) : chartData.length === 0 ? (
                  <div className="h-80 flex items-center justify-center text-theme-muted">
                    <div className="text-center">
                      <Info className="w-12 h-12 mx-auto mb-3 opacity-50" />
                      <p>No data found for the selected filters</p>
                    </div>
                  </div>
                ) : (
                  <div className="h-80">
                    <ResponsiveContainer width="100%" height="100%">
                      <LineChart data={chartData}>
                        <CartesianGrid strokeDasharray="3 3" stroke="#2A3D6B" />
                        <XAxis
                          dataKey="time"
                          stroke="#6B7280"
                          fontSize={11}
                          tickLine={false}
                          axisLine={false}
                          interval="preserveStartEnd"
                        />
                        <YAxis
                          stroke="#6B7280"
                          fontSize={11}
                          tickLine={false}
                          axisLine={false}
                          tickFormatter={(v) => v?.toFixed(1)}
                        />
                        <Tooltip
                          contentStyle={{
                            backgroundColor: '#18284F',
                            border: '1px solid #2A3D6B',
                            borderRadius: '8px',
                          }}
                          labelStyle={{ color: '#9CA3AF' }}
                        />
                        <Legend />

                        {/* Single series */}
                        {result.series.length === 1 && selectedMetrics.map((metric, idx) => (
                          <Line
                            key={metric}
                            type="monotone"
                            dataKey={metric}
                            stroke={SERIES_COLORS[idx % SERIES_COLORS.length]}
                            strokeWidth={2}
                            dot={false}
                            name={METRIC_OPTIONS.find((m) => m.value === metric)?.label || metric}
                          />
                        ))}

                        {/* Multiple series - show first metric for each */}
                        {result.series.length > 1 && seriesNames.map((name, idx) => (
                          <Line
                            key={name}
                            type="monotone"
                            dataKey={`${name}_${selectedMetrics[0]}`}
                            stroke={SERIES_COLORS[idx % SERIES_COLORS.length]}
                            strokeWidth={2}
                            dot={false}
                            name={name}
                          />
                        ))}
                      </LineChart>
                    </ResponsiveContainer>
                  </div>
                )}
              </CardContent>
            </Card>

            {/* Query Metadata */}
            {result && (
              <Card>
                <CardTitle className="flex items-center gap-2">
                  <Info className="w-4 h-4" />
                  Query Details
                </CardTitle>
                <CardContent>
                  <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                    <div>
                      <div className="text-xs text-theme-muted mb-1">Execution Time</div>
                      <div className="text-lg font-semibold text-theme-primary">{result.execution_ms}ms</div>
                    </div>
                    <div>
                      <div className="text-xs text-theme-muted mb-1">Matched Agents</div>
                      <div className="text-lg font-semibold text-theme-primary">{result.matched_agents}</div>
                    </div>
                    <div>
                      <div className="text-xs text-theme-muted mb-1">Matched Targets</div>
                      <div className="text-lg font-semibold text-theme-primary">{result.matched_targets}</div>
                    </div>
                    <div>
                      <div className="text-xs text-theme-muted mb-1">Data Source</div>
                      <div className="text-lg font-semibold text-pilot-cyan">{result.aggregate_table}</div>
                    </div>
                  </div>

                  {/* Series breakdown for multi-series */}
                  {result.series.length > 1 && (
                    <div className="mt-4 pt-4 border-t border-theme">
                      <div className="text-xs text-theme-muted mb-2">Series Breakdown</div>
                      <div className="flex flex-wrap gap-2">
                        {result.series.map((s, idx) => {
                          const name = s.agent_region || s.agent_provider || s.agent_name || s.target_region || s.target_tier || `Series ${idx + 1}`;
                          return (
                            <span
                              key={idx}
                              className="inline-flex items-center gap-2 px-2 py-1 bg-surface-tertiary rounded text-sm"
                            >
                              <span
                                className="w-3 h-3 rounded-full"
                                style={{ backgroundColor: SERIES_COLORS[idx % SERIES_COLORS.length] }}
                              />
                              <span className="text-theme-secondary">{name}</span>
                              <span className="text-theme-muted">({s.points.length} pts)</span>
                            </span>
                          );
                        })}
                      </div>
                    </div>
                  )}
                </CardContent>
              </Card>
            )}
          </div>
        </div>
      </PageContent>
    </>
  );
}
