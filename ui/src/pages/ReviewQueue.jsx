import { useState, useEffect, useMemo } from 'react';
import {
  AlertCircle,
  RefreshCw,
  CheckCircle,
  XCircle,
  Clock,
  Network,
  MapPin,
  Building2,
  History,
  ChevronDown,
  ChevronUp,
} from 'lucide-react';

import { PageHeader, PageContent } from '../components/Layout';
import { Card } from '../components/Card';
import { MetricCard } from '../components/MetricCard';
import { StatusBadge, StatusDot } from '../components/StatusBadge';
import { Button } from '../components/Button';
import { SearchInput, Select } from '../components/Input';
import { Modal, ModalFooter } from '../components/Modal';
import { formatRelativeTime } from '../lib/utils';
import { endpoints } from '../lib/api';

export function ReviewQueue() {
  const [targets, setTargets] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [search, setSearch] = useState('');
  const [popFilter, setPopFilter] = useState('');
  const [selectedTarget, setSelectedTarget] = useState(null);
  const [acknowledging, setAcknowledging] = useState(false);
  const [showAckModal, setShowAckModal] = useState(false);
  const [expandedTargets, setExpandedTargets] = useState(new Set());

  const fetchData = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await endpoints.getReviewQueue();
      setTargets(res.targets || []);
    } catch (err) {
      console.error('Failed to fetch review queue:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
    // Refresh every 30 seconds
    const interval = setInterval(fetchData, 30000);
    return () => clearInterval(interval);
  }, []);

  // Get unique POPs for filter
  const pops = useMemo(() => {
    const uniquePops = [...new Set(targets.map(t => t.pop_name).filter(Boolean))];
    return [
      { value: '', label: 'All POPs' },
      ...uniquePops.map(p => ({ value: p, label: p })),
    ];
  }, [targets]);

  const filteredTargets = useMemo(() => {
    return targets.filter(target => {
      if (search) {
        const searchLower = search.toLowerCase();
        const matchesIP = target.ip?.toLowerCase().includes(searchLower);
        const matchesSubscriber = target.subscriber_name?.toLowerCase().includes(searchLower);
        const matchesPop = target.pop_name?.toLowerCase().includes(searchLower);
        const matchesNetwork = target.network_address?.toLowerCase().includes(searchLower);
        if (!matchesIP && !matchesSubscriber && !matchesPop && !matchesNetwork) {
          return false;
        }
      }
      if (popFilter && target.pop_name !== popFilter) return false;
      return true;
    });
  }, [targets, search, popFilter]);

  // Group by POP
  const groupedByPop = useMemo(() => {
    const grouped = {};
    filteredTargets.forEach(t => {
      const pop = t.pop_name || 'Unknown';
      if (!grouped[pop]) grouped[pop] = [];
      grouped[pop].push(t);
    });
    return grouped;
  }, [filteredTargets]);

  const handleAcknowledge = async (markInactive = false) => {
    if (!selectedTarget) return;
    setAcknowledging(true);
    try {
      await endpoints.acknowledgeTarget(selectedTarget.id, markInactive);
      setShowAckModal(false);
      setSelectedTarget(null);
      fetchData();
    } catch (err) {
      console.error('Failed to acknowledge target:', err);
      alert('Failed to acknowledge: ' + err.message);
    } finally {
      setAcknowledging(false);
    }
  };

  const toggleExpanded = (targetId) => {
    setExpandedTargets(prev => {
      const next = new Set(prev);
      if (next.has(targetId)) {
        next.delete(targetId);
      } else {
        next.add(targetId);
      }
      return next;
    });
  };

  if (error) {
    return (
      <>
        <PageHeader title="Review Queue" />
        <PageContent>
          <Card accent="red">
            <div className="flex items-center gap-3">
              <AlertCircle className="w-6 h-6 text-pilot-red" />
              <div>
                <h3 className="font-medium text-theme-primary">Failed to load review queue</h3>
                <p className="text-sm text-theme-muted">{error}</p>
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
        title="Review Queue"
        description={`${targets.length} targets need review`}
        actions={
          <Button variant="secondary" onClick={fetchData} className="gap-2" size="sm">
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
            <span className="hidden sm:inline">Refresh</span>
          </Button>
        }
      />

      <PageContent>
        {/* Summary Cards */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 md:gap-4 mb-4 md:mb-6">
          <MetricCard
            title="Needs Review"
            value={targets.length.toLocaleString()}
            icon={AlertCircle}
            status={targets.length > 0 ? 'degraded' : 'healthy'}
            size="sm"
          />
          <MetricCard
            title="POPs Affected"
            value={Object.keys(groupedByPop).length.toLocaleString()}
            icon={MapPin}
            size="sm"
          />
          <MetricCard
            title="Oldest Item"
            value={targets.length > 0
              ? formatRelativeTime(targets.reduce((oldest, t) =>
                  !oldest || new Date(t.state_changed_at) < new Date(oldest)
                    ? t.state_changed_at
                    : oldest, null))
              : '—'
            }
            icon={Clock}
            size="sm"
          />
          <MetricCard
            title="Subscribers"
            value={new Set(targets.map(t => t.subscriber_name).filter(Boolean)).size.toLocaleString()}
            icon={Building2}
            size="sm"
          />
        </div>

        {/* Filters */}
        <Card className="mb-4 md:mb-6">
          <div className="flex flex-col sm:flex-row sm:flex-wrap gap-2 sm:gap-3 md:gap-4 items-stretch sm:items-center">
            <SearchInput
              value={search}
              onChange={setSearch}
              placeholder="Search IP, subscriber..."
              className="w-full sm:w-56 md:w-72"
            />
            <Select
              options={pops}
              value={popFilter}
              onChange={setPopFilter}
              className="w-full sm:w-32 md:w-40"
            />
          </div>
        </Card>

        {/* Queue List */}
        {loading ? (
          <Card>
            <div className="flex items-center justify-center py-8 md:py-12">
              <RefreshCw className="w-5 h-5 md:w-6 md:h-6 animate-spin text-theme-muted" />
            </div>
          </Card>
        ) : filteredTargets.length === 0 ? (
          <Card>
            <div className="text-center py-8 md:py-12 text-theme-muted">
              <CheckCircle className="w-10 h-10 md:w-12 md:h-12 mx-auto mb-3 md:mb-4 text-status-healthy opacity-50" />
              <p className="text-base md:text-lg">All clear!</p>
              <p className="text-xs md:text-sm mt-1">No targets currently need review</p>
            </div>
          </Card>
        ) : (
          <div className="space-y-3 md:space-y-4">
            {Object.entries(groupedByPop).sort((a, b) => b[1].length - a[1].length).map(([pop, popTargets]) => (
              <Card key={pop} accent="warning">
                <div className="flex items-center justify-between mb-3 md:mb-4">
                  <div className="flex items-center gap-2 md:gap-3">
                    <MapPin className="w-4 h-4 md:w-5 md:h-5 text-accent" />
                    <h3 className="font-semibold text-theme-primary text-sm md:text-base">{pop}</h3>
                    <span className="px-1.5 md:px-2 py-0.5 bg-accent/20 text-accent rounded text-xs md:text-sm font-medium">
                      {popTargets.length}
                    </span>
                  </div>
                </div>

                <div className="space-y-2 md:space-y-3">
                  {popTargets.map(target => {
                    const isExpanded = expandedTargets.has(target.id);
                    return (
                      <div
                        key={target.id}
                        className="bg-surface-primary rounded-lg border border-theme overflow-hidden"
                      >
                        {/* Header Row */}
                        <div
                          className="flex flex-col sm:flex-row sm:items-center sm:justify-between p-2.5 md:p-3 cursor-pointer hover:bg-surface-tertiary active:bg-surface-tertiary transition-colors gap-2 sm:gap-0"
                          onClick={() => toggleExpanded(target.id)}
                        >
                          <div className="flex items-center gap-2 md:gap-4">
                            <StatusDot status="down" pulse />
                            <div className="min-w-0">
                              <div className="flex items-center gap-2 flex-wrap">
                                <span className="font-mono font-medium text-theme-primary text-sm truncate">{target.ip}</span>
                                <StatusBadge status="down" label="Excluded" size="sm" />
                              </div>
                              <div className="text-[10px] md:text-xs text-theme-muted mt-0.5 truncate">
                                {target.subscriber_name || 'Unknown'} | {target.network_address}/{target.network_size}
                              </div>
                            </div>
                          </div>
                          <div className="flex items-center justify-between sm:justify-end gap-2 md:gap-4 pl-5 sm:pl-0">
                            <div className="text-right text-xs md:text-sm">
                              <div className="text-theme-muted hidden sm:block">Excluded</div>
                              <div className="text-theme-secondary">
                                {formatRelativeTime(target.state_changed_at)}
                              </div>
                            </div>
                            <Button
                              variant="secondary"
                              size="sm"
                              onClick={(e) => {
                                e.stopPropagation();
                                setSelectedTarget(target);
                                setShowAckModal(true);
                              }}
                            >
                              Review
                            </Button>
                            {isExpanded ? (
                              <ChevronUp className="w-4 h-4 text-theme-muted hidden sm:block" />
                            ) : (
                              <ChevronDown className="w-4 h-4 text-theme-muted hidden sm:block" />
                            )}
                          </div>
                        </div>

                        {/* Expanded Details */}
                        {isExpanded && (
                          <div className="px-2.5 md:px-3 pb-2.5 md:pb-3 border-t border-theme">
                            <div className="grid grid-cols-2 gap-2 md:gap-3 mt-2.5 md:mt-3">
                              <div>
                                <div className="text-[10px] md:text-xs text-theme-muted">IP Type</div>
                                <div className="text-xs md:text-sm text-theme-primary">{target.ip_type || 'customer'}</div>
                              </div>
                              <div>
                                <div className="text-[10px] md:text-xs text-theme-muted">Ownership</div>
                                <div className="text-xs md:text-sm text-theme-primary">{target.ownership || 'auto'}</div>
                              </div>
                              <div>
                                <div className="text-[10px] md:text-xs text-theme-muted">Gateway</div>
                                <div className="text-xs md:text-sm font-mono text-theme-primary truncate">{target.gateway_address || '—'}</div>
                              </div>
                              <div>
                                <div className="text-[10px] md:text-xs text-theme-muted">Last Response</div>
                                <div className="text-xs md:text-sm text-theme-primary">
                                  {target.last_response_at
                                    ? formatRelativeTime(target.last_response_at)
                                    : 'Never'}
                                </div>
                              </div>
                            </div>
                            {target.location_address && (
                              <div className="mt-2 md:mt-3">
                                <div className="text-[10px] md:text-xs text-theme-muted">Location</div>
                                <div className="text-xs md:text-sm text-theme-primary truncate">
                                  {target.location_address}, {target.city}
                                </div>
                              </div>
                            )}
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              </Card>
            ))}
          </div>
        )}

        {/* Acknowledge Modal */}
        <Modal
          isOpen={showAckModal}
          onClose={() => {
            setShowAckModal(false);
            setSelectedTarget(null);
          }}
          title="Review Target"
          size="md"
        >
          {selectedTarget && (
            <div className="space-y-4">
              <div className="bg-surface-primary rounded-lg p-4">
                <div className="flex items-center gap-3 mb-3">
                  <StatusDot status="down" />
                  <span className="font-mono font-medium text-theme-primary text-lg">
                    {selectedTarget.ip}
                  </span>
                </div>
                <div className="grid grid-cols-2 gap-3 text-sm">
                  <div>
                    <span className="text-theme-muted">Subscriber:</span>{' '}
                    <span className="text-theme-primary">{selectedTarget.subscriber_name || '—'}</span>
                  </div>
                  <div>
                    <span className="text-theme-muted">Network:</span>{' '}
                    <span className="text-theme-primary font-mono">
                      {selectedTarget.network_address}/{selectedTarget.network_size}
                    </span>
                  </div>
                  <div>
                    <span className="text-theme-muted">POP:</span>{' '}
                    <span className="text-theme-primary">{selectedTarget.pop_name || '—'}</span>
                  </div>
                  <div>
                    <span className="text-theme-muted">State changed:</span>{' '}
                    <span className="text-theme-primary">
                      {formatRelativeTime(selectedTarget.state_changed_at)}
                    </span>
                  </div>
                </div>
              </div>

              <div className="text-sm text-theme-muted">
                <p className="mb-2">
                  This target has been marked as <span className="text-pilot-red font-medium">Excluded</span> because
                  it hasn't responded to probes for an extended period.
                </p>
                <p>Choose an action:</p>
              </div>

              <div className="space-y-3">
                <button
                  onClick={() => handleAcknowledge(false)}
                  disabled={acknowledging}
                  className="w-full p-3 bg-surface-primary hover:bg-surface-tertiary border border-theme rounded-lg text-left transition-colors"
                >
                  <div className="flex items-center gap-3">
                    <CheckCircle className="w-5 h-5 text-status-healthy" />
                    <div>
                      <div className="font-medium text-theme-primary">Acknowledge</div>
                      <div className="text-sm text-theme-muted">
                        Keep monitoring at reduced frequency (smart re-check tier)
                      </div>
                    </div>
                  </div>
                </button>

                <button
                  onClick={() => handleAcknowledge(true)}
                  disabled={acknowledging}
                  className="w-full p-3 bg-surface-primary hover:bg-surface-tertiary border border-theme rounded-lg text-left transition-colors"
                >
                  <div className="flex items-center gap-3">
                    <XCircle className="w-5 h-5 text-accent" />
                    <div>
                      <div className="font-medium text-theme-primary">Mark Inactive</div>
                      <div className="text-sm text-theme-muted">
                        Target is intentionally offline; monitor at 1-hour intervals
                      </div>
                    </div>
                  </div>
                </button>
              </div>
            </div>
          )}

          <ModalFooter>
            <Button
              variant="ghost"
              onClick={() => {
                setShowAckModal(false);
                setSelectedTarget(null);
              }}
              disabled={acknowledging}
            >
              Cancel
            </Button>
          </ModalFooter>
        </Modal>
      </PageContent>
    </>
  );
}
