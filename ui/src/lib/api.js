const API_BASE = '/api/v1';

class ApiClient {
  constructor(baseUrl = API_BASE) {
    this.baseUrl = baseUrl;
  }

  async request(endpoint, options = {}) {
    const url = `${this.baseUrl}${endpoint}`;
    const config = {
      headers: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
      ...options,
    };

    if (config.body && typeof config.body === 'object') {
      config.body = JSON.stringify(config.body);
    }

    const response = await fetch(url, config);

    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new ApiError(response.status, error.message || response.statusText);
    }

    if (response.status === 204) {
      return null;
    }

    return response.json();
  }

  get(endpoint, options = {}) {
    return this.request(endpoint, { ...options, method: 'GET' });
  }

  post(endpoint, body, options = {}) {
    return this.request(endpoint, { ...options, method: 'POST', body });
  }

  put(endpoint, body, options = {}) {
    return this.request(endpoint, { ...options, method: 'PUT', body });
  }

  delete(endpoint, options = {}) {
    return this.request(endpoint, { ...options, method: 'DELETE' });
  }
}

class ApiError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
    this.name = 'ApiError';
  }
}

export const api = new ApiClient();

// API endpoints
export const endpoints = {
  // Health
  health: () => api.get('/health'),

  // Agents
  listAgents: () => api.get('/agents'),
  getAgent: (id) => api.get(`/agents/${id}`),
  getAgentMetrics: (id) => api.get(`/agents/${id}/metrics`),

  // Targets
  listTargets: (params = {}) => {
    const query = new URLSearchParams(params).toString();
    return api.get(`/targets${query ? `?${query}` : ''}`);
  },
  getTarget: (id) => api.get(`/targets/${id}`),
  getTargetStatus: (id) => api.get(`/targets/${id}/status`),
  getTargetHistory: (id, window = '1h') => api.get(`/targets/${id}/history?window=${window}`),
  getTargetHistoryByAgent: (id, window = '1h') => api.get(`/targets/${id}/history/by-agent?window=${window}`),
  getAllTargetStatuses: () => api.get('/targets/status'),
  createTarget: (data) => api.post('/targets', data),
  updateTarget: (id, data) => api.put(`/targets/${id}`, data),
  deleteTarget: (id) => api.delete(`/targets/${id}`),
  triggerMTR: (id, agentIds = []) => api.post(`/targets/${id}/mtr`, { agent_ids: agentIds }),
  getTargetLive: (id, seconds = 60) => api.get(`/targets/${id}/live?seconds=${seconds}`),

  // Tiers
  listTiers: () => api.get('/tiers'),
  getTier: (name) => api.get(`/tiers/${name}`),
  createTier: (data) => api.post('/tiers', data),
  updateTier: (name, data) => api.put(`/tiers/${name}`, data),
  deleteTier: (name) => api.delete(`/tiers/${name}`),

  // Results
  getTargetResults: (targetId, params = {}) => {
    const query = new URLSearchParams(params).toString();
    return api.get(`/targets/${targetId}/results${query ? `?${query}` : ''}`);
  },

  // Snapshots
  listSnapshots: () => api.get('/snapshots'),
  getSnapshot: (id) => api.get(`/snapshots/${id}`),
  createSnapshot: (data) => api.post('/snapshots', data),
  compareSnapshots: (id1, id2) => api.get(`/snapshots/${id1}/compare/${id2}`),

  // Alerts
  listAlerts: (params = {}) => {
    const query = new URLSearchParams(params).toString();
    return api.get(`/alerts${query ? `?${query}` : ''}`);
  },
  acknowledgeAlert: (id) => api.post(`/alerts/${id}/acknowledge`),

  // Commands
  submitCommand: (data) => api.post('/commands', data),
  getCommand: (id) => api.get(`/commands/${id}`),

  // Metrics
  getLatencyTrend: (window = '24h') => api.get(`/metrics/latency?window=${window}`),

  // Stats
  getFleetStats: () => api.get('/stats/fleet'),
  getAgentStats: () => api.get('/stats/agents'),

  // Incidents
  listIncidents: (status = '', limit = 100) => {
    const params = new URLSearchParams();
    if (status) params.set('status', status);
    if (limit) params.set('limit', limit);
    const query = params.toString();
    return api.get(`/incidents${query ? `?${query}` : ''}`);
  },
  getIncident: (id) => api.get(`/incidents/${id}`),
  acknowledgeIncident: (id, acknowledgedBy = 'ui') =>
    api.post(`/incidents/${id}/acknowledge`, { acknowledged_by: acknowledgedBy }),
  resolveIncident: (id) => api.post(`/incidents/${id}/resolve`),
  addIncidentNote: (id, note) => api.put(`/incidents/${id}/notes`, { note }),

  // Baselines
  getTargetBaselines: (targetId) => api.get(`/targets/${targetId}/baselines`),
  getBaseline: (agentId, targetId) => api.get(`/baselines/${agentId}/${targetId}`),
  recalculateBaselines: () => api.post('/baselines/recalculate'),

  // Reports
  getTargetReport: (targetId, window = '90d') =>
    api.get(`/reports/targets/${targetId}?window=${window}`),
};
