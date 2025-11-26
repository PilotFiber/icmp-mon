import { useState, useEffect } from 'react';
import {
  Settings as SettingsIcon,
  Bell,
  Server,
  Database,
  Shield,
  ChevronRight,
  Plus,
  Edit2,
  Trash2,
  Clock,
  Users,
} from 'lucide-react';

import { PageHeader, PageContent } from '../components/Layout';
import { Card, CardTitle, CardContent, CardDescription } from '../components/Card';
import { Button } from '../components/Button';
import { Input, Select } from '../components/Input';
import { Modal, ModalFooter, ConfirmModal } from '../components/Modal';
import { endpoints } from '../lib/api';

const settingsSections = [
  {
    id: 'tiers',
    name: 'Monitoring Tiers',
    description: 'Configure monitoring intervals and agent selection policies',
    icon: Server,
  },
  {
    id: 'alerts',
    name: 'Alert Rules',
    description: 'Define alert thresholds and notification routing',
    icon: Bell,
  },
  {
    id: 'retention',
    name: 'Data Retention',
    description: 'Configure data retention and compression policies',
    icon: Database,
  },
  {
    id: 'security',
    name: 'Security',
    description: 'API keys, authentication, and access control',
    icon: Shield,
  },
];

// Tier Create/Edit Modal
function TierModal({ isOpen, onClose, tier, onSave }) {
  const [formData, setFormData] = useState({
    name: '',
    display_name: '',
    probe_interval_seconds: 30,
    probe_timeout_seconds: 5,
    probe_retries: 0,
    agent_selection: {
      strategy: 'all',
      count: 0,
      diversity: {
        min_regions: 0,
        min_providers: 0,
      },
    },
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);
  const isEditing = !!tier;

  useEffect(() => {
    if (tier) {
      setFormData({
        name: tier.name || '',
        display_name: tier.display_name || '',
        probe_interval_seconds: Math.floor((tier.probe_interval || 30000000000) / 1000000000),
        probe_timeout_seconds: Math.floor((tier.probe_timeout || 5000000000) / 1000000000),
        probe_retries: tier.probe_retries || 0,
        agent_selection: {
          strategy: tier.agent_selection?.strategy || 'all',
          count: tier.agent_selection?.count || 0,
          diversity: {
            min_regions: tier.agent_selection?.diversity?.min_regions || 0,
            min_providers: tier.agent_selection?.diversity?.min_providers || 0,
          },
        },
      });
    } else {
      setFormData({
        name: '',
        display_name: '',
        probe_interval_seconds: 30,
        probe_timeout_seconds: 5,
        probe_retries: 0,
        agent_selection: {
          strategy: 'all',
          count: 0,
          diversity: { min_regions: 0, min_providers: 0 },
        },
      });
    }
    setError(null);
  }, [tier, isOpen]);

  const handleSubmit = async () => {
    setError(null);
    setSaving(true);
    try {
      const payload = {
        ...formData,
        agent_selection: formData.agent_selection.strategy === 'all'
          ? { strategy: 'all' }
          : formData.agent_selection,
      };

      if (isEditing) {
        await endpoints.updateTier(tier.name, payload);
      } else {
        await endpoints.createTier(payload);
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
    <Modal isOpen={isOpen} onClose={onClose} title={isEditing ? 'Edit Tier' : 'Create Tier'} size="lg">
      <div className="space-y-4">
        {error && (
          <div className="p-3 bg-pilot-red/20 border border-pilot-red/30 rounded-lg text-pilot-red text-sm">
            {error}
          </div>
        )}

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-theme-secondary mb-1">Name (ID)</label>
            <input
              type="text"
              value={formData.name}
              onChange={(e) => setFormData(prev => ({ ...prev, name: e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, '-') }))}
              placeholder="e.g., enterprise"
              disabled={isEditing}
              className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan disabled:opacity-50"
            />
            {isEditing && <p className="text-xs text-theme-muted mt-1">Name cannot be changed</p>}
          </div>
          <div>
            <label className="block text-sm font-medium text-theme-secondary mb-1">Display Name</label>
            <input
              type="text"
              value={formData.display_name}
              onChange={(e) => setFormData(prev => ({ ...prev, display_name: e.target.value }))}
              placeholder="e.g., Enterprise Customers"
              className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
            />
          </div>
        </div>

        <div className="border-t border-theme pt-4">
          <h4 className="text-sm font-medium text-theme-secondary mb-3 flex items-center gap-2">
            <Clock className="w-4 h-4" />
            Probe Timing
          </h4>
          <div className="grid grid-cols-3 gap-4">
            <div>
              <label className="block text-sm text-theme-muted mb-1">Interval (seconds)</label>
              <input
                type="number"
                min="1"
                value={formData.probe_interval_seconds}
                onChange={(e) => setFormData(prev => ({ ...prev, probe_interval_seconds: parseInt(e.target.value) || 30 }))}
                className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
              />
            </div>
            <div>
              <label className="block text-sm text-theme-muted mb-1">Timeout (seconds)</label>
              <input
                type="number"
                min="1"
                value={formData.probe_timeout_seconds}
                onChange={(e) => setFormData(prev => ({ ...prev, probe_timeout_seconds: parseInt(e.target.value) || 5 }))}
                className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
              />
            </div>
            <div>
              <label className="block text-sm text-theme-muted mb-1">Retries</label>
              <input
                type="number"
                min="0"
                value={formData.probe_retries}
                onChange={(e) => setFormData(prev => ({ ...prev, probe_retries: parseInt(e.target.value) || 0 }))}
                className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
              />
            </div>
          </div>
        </div>

        <div className="border-t border-theme pt-4">
          <h4 className="text-sm font-medium text-theme-secondary mb-3 flex items-center gap-2">
            <Users className="w-4 h-4" />
            Agent Selection
          </h4>
          <div className="space-y-3">
            <div>
              <label className="block text-sm text-theme-muted mb-1">Strategy</label>
              <select
                value={formData.agent_selection.strategy}
                onChange={(e) => setFormData(prev => ({
                  ...prev,
                  agent_selection: { ...prev.agent_selection, strategy: e.target.value }
                }))}
                className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
              >
                <option value="all">All Agents</option>
                <option value="distributed">Distributed (subset)</option>
              </select>
            </div>

            {formData.agent_selection.strategy === 'distributed' && (
              <div className="grid grid-cols-3 gap-4 pl-4 border-l-2 border-theme">
                <div>
                  <label className="block text-sm text-theme-muted mb-1">Agent Count</label>
                  <input
                    type="number"
                    min="1"
                    value={formData.agent_selection.count}
                    onChange={(e) => setFormData(prev => ({
                      ...prev,
                      agent_selection: { ...prev.agent_selection, count: parseInt(e.target.value) || 1 }
                    }))}
                    className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
                  />
                </div>
                <div>
                  <label className="block text-sm text-theme-muted mb-1">Min Regions</label>
                  <input
                    type="number"
                    min="0"
                    value={formData.agent_selection.diversity.min_regions}
                    onChange={(e) => setFormData(prev => ({
                      ...prev,
                      agent_selection: {
                        ...prev.agent_selection,
                        diversity: { ...prev.agent_selection.diversity, min_regions: parseInt(e.target.value) || 0 }
                      }
                    }))}
                    className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
                  />
                </div>
                <div>
                  <label className="block text-sm text-theme-muted mb-1">Min Providers</label>
                  <input
                    type="number"
                    min="0"
                    value={formData.agent_selection.diversity.min_providers}
                    onChange={(e) => setFormData(prev => ({
                      ...prev,
                      agent_selection: {
                        ...prev.agent_selection,
                        diversity: { ...prev.agent_selection.diversity, min_providers: parseInt(e.target.value) || 0 }
                      }
                    }))}
                    className="w-full px-3 py-2 bg-surface-primary border border-theme rounded-lg text-theme-primary focus:outline-none focus:ring-2 focus:ring-pilot-cyan"
                  />
                </div>
              </div>
            )}
          </div>
        </div>
      </div>

      <ModalFooter>
        <Button variant="ghost" onClick={onClose} disabled={saving}>Cancel</Button>
        <Button onClick={handleSubmit} disabled={saving || !formData.name}>
          {saving ? 'Saving...' : (isEditing ? 'Update' : 'Create')}
        </Button>
      </ModalFooter>
    </Modal>
  );
}

function formatDuration(nanoseconds) {
  const seconds = nanoseconds / 1000000000;
  if (seconds < 60) return `${seconds}s`;
  const minutes = seconds / 60;
  return `${minutes}m`;
}

export function Settings() {
  const [activeSection, setActiveSection] = useState('tiers');
  const [tiers, setTiers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showTierModal, setShowTierModal] = useState(false);
  const [editingTier, setEditingTier] = useState(null);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [deletingTier, setDeletingTier] = useState(null);
  const [deleteLoading, setDeleteLoading] = useState(false);

  const fetchTiers = async () => {
    try {
      setLoading(true);
      const res = await endpoints.listTiers();
      setTiers(res.tiers || []);
    } catch (err) {
      console.error('Failed to fetch tiers:', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchTiers();
  }, []);

  const handleDeleteTier = async () => {
    if (!deletingTier) return;
    setDeleteLoading(true);
    try {
      await endpoints.deleteTier(deletingTier.name);
      setShowDeleteConfirm(false);
      setDeletingTier(null);
      fetchTiers();
    } catch (err) {
      alert('Failed to delete tier: ' + err.message);
    } finally {
      setDeleteLoading(false);
    }
  };

  return (
    <>
      <PageHeader
        title="Settings"
        description="Configure monitoring tiers, alerts, and system settings"
      />

      <PageContent>
        <div className="grid grid-cols-1 lg:grid-cols-4 gap-6">
          {/* Sidebar Navigation */}
          <div className="lg:col-span-1">
            <Card className="p-2">
              <nav className="space-y-1">
                {settingsSections.map((section) => (
                  <button
                    key={section.id}
                    onClick={() => setActiveSection(section.id)}
                    className={`
                      w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-left
                      transition-colors
                      ${activeSection === section.id
                        ? 'bg-pilot-yellow text-neutral-900'
                        : 'text-theme-secondary hover:bg-surface-tertiary'
                      }
                    `}
                  >
                    <section.icon className="w-5 h-5" />
                    <span className="font-medium">{section.name}</span>
                  </button>
                ))}
              </nav>
            </Card>
          </div>

          {/* Main Content */}
          <div className="lg:col-span-3">
            {activeSection === 'tiers' && (
              <div className="space-y-6">
                <Card>
                  <div className="flex items-center justify-between">
                    <div>
                      <CardTitle>Monitoring Tiers</CardTitle>
                      <CardDescription>
                        Tiers control monitoring intensity including probe interval, agent selection,
                        and alerting behavior.
                      </CardDescription>
                    </div>
                    <Button onClick={() => { setEditingTier(null); setShowTierModal(true); }} className="gap-2">
                      <Plus className="w-4 h-4" />
                      Add Tier
                    </Button>
                  </div>
                </Card>

                {loading ? (
                  <Card>
                    <div className="text-center py-8 text-theme-muted">Loading tiers...</div>
                  </Card>
                ) : tiers.length === 0 ? (
                  <Card>
                    <div className="text-center py-8 text-theme-muted">
                      No tiers configured. Create your first tier to get started.
                    </div>
                  </Card>
                ) : (
                  tiers.map((tier) => (
                    <Card key={tier.name} className="group">
                      <div className="flex items-start justify-between">
                        <div className="flex-1">
                          <div className="flex items-center gap-3">
                            <h3 className="text-lg font-semibold text-theme-primary">{tier.display_name || tier.name}</h3>
                            <span className="text-xs px-2 py-0.5 bg-surface-tertiary text-theme-muted rounded font-mono">
                              {tier.name}
                            </span>
                          </div>
                          <div className="flex gap-6 mt-3 text-sm">
                            <div className="text-theme-muted">
                              <span className="text-theme-muted">Interval:</span>{' '}
                              <span className="text-theme-primary font-medium">{formatDuration(tier.probe_interval)}</span>
                            </div>
                            <div className="text-theme-muted">
                              <span className="text-theme-muted">Timeout:</span>{' '}
                              <span className="text-theme-primary font-medium">{formatDuration(tier.probe_timeout)}</span>
                            </div>
                            <div className="text-theme-muted">
                              <span className="text-theme-muted">Retries:</span>{' '}
                              <span className="text-theme-primary font-medium">{tier.probe_retries}</span>
                            </div>
                          </div>
                          <div className="mt-2 text-sm text-theme-muted">
                            <span className="text-theme-muted">Agent Selection:</span>{' '}
                            <span className="text-theme-primary capitalize">{tier.agent_selection?.strategy || 'all'}</span>
                            {tier.agent_selection?.count > 0 && (
                              <span className="text-theme-primary"> ({tier.agent_selection.count} agents)</span>
                            )}
                            {tier.agent_selection?.diversity?.min_regions > 0 && (
                              <span className="text-theme-muted">
                                {' '}• Min {tier.agent_selection.diversity.min_regions} regions
                              </span>
                            )}
                            {tier.agent_selection?.diversity?.min_providers > 0 && (
                              <span className="text-theme-muted">
                                , {tier.agent_selection.diversity.min_providers} providers
                              </span>
                            )}
                          </div>
                        </div>
                        <div className="flex gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => { setEditingTier(tier); setShowTierModal(true); }}
                            className="gap-1"
                          >
                            <Edit2 className="w-3 h-3" />
                            Edit
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => { setDeletingTier(tier); setShowDeleteConfirm(true); }}
                            className="gap-1 text-pilot-red hover:text-pilot-red hover:bg-pilot-red/10"
                          >
                            <Trash2 className="w-3 h-3" />
                            Delete
                          </Button>
                        </div>
                      </div>
                    </Card>
                  ))
                )}
              </div>
            )}

            {activeSection === 'alerts' && (
              <div className="space-y-6">
                <Card>
                  <CardTitle>Alert Configuration</CardTitle>
                  <CardDescription>
                    Configure alert thresholds and notification handlers for different tiers and severities.
                  </CardDescription>
                </Card>

                <Card>
                  <h3 className="text-lg font-semibold text-theme-primary mb-4">Default Thresholds</h3>
                  <div className="grid grid-cols-2 gap-4">
                    <Input
                      label="Latency Warning (ms)"
                      type="number"
                      defaultValue={100}
                    />
                    <Input
                      label="Latency Critical (ms)"
                      type="number"
                      defaultValue={500}
                    />
                    <Input
                      label="Packet Loss Warning (%)"
                      type="number"
                      defaultValue={5}
                    />
                    <Input
                      label="Packet Loss Critical (%)"
                      type="number"
                      defaultValue={25}
                    />
                  </div>
                </Card>

                <Card>
                  <h3 className="text-lg font-semibold text-theme-primary mb-4">Notification Handlers</h3>
                  <div className="space-y-3">
                    <div className="flex items-center justify-between p-3 bg-surface-primary rounded-lg">
                      <div>
                        <span className="font-medium text-theme-primary">Slack</span>
                        <span className="text-sm text-theme-muted ml-2">#noc-alerts</span>
                      </div>
                      <span className="text-xs px-2 py-1 bg-status-healthy/20 text-status-healthy rounded">Active</span>
                    </div>
                    <div className="flex items-center justify-between p-3 bg-surface-primary rounded-lg">
                      <div>
                        <span className="font-medium text-theme-primary">PagerDuty</span>
                        <span className="text-sm text-theme-muted ml-2">Critical alerts only</span>
                      </div>
                      <span className="text-xs px-2 py-1 bg-status-healthy/20 text-status-healthy rounded">Active</span>
                    </div>
                  </div>
                  <Button variant="secondary" size="sm" className="mt-4">
                    Add Handler
                  </Button>
                </Card>
              </div>
            )}

            {activeSection === 'retention' && (
              <div className="space-y-6">
                <Card>
                  <CardTitle>Data Retention</CardTitle>
                  <CardDescription>
                    Configure how long probe results and metrics are retained.
                  </CardDescription>
                </Card>

                <Card>
                  <h3 className="text-lg font-semibold text-theme-primary mb-4">Retention Policies</h3>
                  <div className="space-y-4">
                    <div className="flex items-center justify-between">
                      <div>
                        <span className="font-medium text-theme-primary">Raw Probe Results</span>
                        <p className="text-sm text-theme-muted">Individual probe measurements</p>
                      </div>
                      <Select
                        options={[
                          { value: '7', label: '7 days' },
                          { value: '14', label: '14 days' },
                          { value: '30', label: '30 days' },
                          { value: '90', label: '90 days' },
                        ]}
                        value="30"
                        onChange={() => {}}
                        className="w-32"
                      />
                    </div>
                    <div className="flex items-center justify-between">
                      <div>
                        <span className="font-medium text-theme-primary">Hourly Aggregates</span>
                        <p className="text-sm text-theme-muted">Rolled up metrics per hour</p>
                      </div>
                      <Select
                        options={[
                          { value: '365', label: '1 year' },
                          { value: '730', label: '2 years' },
                          { value: '1825', label: '5 years' },
                        ]}
                        value="730"
                        onChange={() => {}}
                        className="w-32"
                      />
                    </div>
                  </div>
                </Card>
              </div>
            )}

            {activeSection === 'security' && (
              <div className="space-y-6">
                <Card>
                  <CardTitle>Security Settings</CardTitle>
                  <CardDescription>
                    Manage API keys and authentication for agents and external integrations.
                  </CardDescription>
                </Card>

                <Card>
                  <h3 className="text-lg font-semibold text-theme-primary mb-4">API Keys</h3>
                  <div className="space-y-3">
                    <div className="flex items-center justify-between p-3 bg-surface-primary rounded-lg">
                      <div>
                        <span className="font-medium text-theme-primary">Production Agents</span>
                        <p className="text-xs text-theme-muted mt-0.5">Last used: 2 minutes ago</p>
                      </div>
                      <Button variant="ghost" size="sm">Rotate</Button>
                    </div>
                    <div className="flex items-center justify-between p-3 bg-surface-primary rounded-lg">
                      <div>
                        <span className="font-medium text-theme-primary">Grafana Integration</span>
                        <p className="text-xs text-theme-muted mt-0.5">Last used: 1 hour ago</p>
                      </div>
                      <Button variant="ghost" size="sm">Rotate</Button>
                    </div>
                  </div>
                  <Button variant="secondary" size="sm" className="mt-4">
                    Create New Key
                  </Button>
                </Card>
              </div>
            )}
          </div>
        </div>
      </PageContent>

      {/* Tier Create/Edit Modal */}
      <TierModal
        isOpen={showTierModal}
        onClose={() => { setShowTierModal(false); setEditingTier(null); }}
        tier={editingTier}
        onSave={fetchTiers}
      />

      {/* Delete Confirmation */}
      <ConfirmModal
        isOpen={showDeleteConfirm}
        onClose={() => { setShowDeleteConfirm(false); setDeletingTier(null); }}
        onConfirm={handleDeleteTier}
        title="Delete Tier"
        message={`Are you sure you want to delete the "${deletingTier?.display_name || deletingTier?.name}" tier? This cannot be undone. Note: You cannot delete a tier if any targets are using it.`}
        confirmText="Delete"
        confirmVariant="danger"
        loading={deleteLoading}
      />
    </>
  );
}
