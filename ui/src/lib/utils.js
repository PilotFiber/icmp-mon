// Format bytes to human readable
export function formatBytes(bytes, decimals = 2) {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}

// Format duration in milliseconds to human readable
export function formatDuration(ms) {
  if (ms < 1) return '<1ms';
  if (ms < 1000) return `${Math.round(ms)}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  if (ms < 3600000) return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`;
  return `${Math.floor(ms / 3600000)}h ${Math.floor((ms % 3600000) / 60000)}m`;
}

// Format relative time
export function formatRelativeTime(date) {
  const now = new Date();
  const then = new Date(date);
  const diff = now - then;

  const seconds = Math.floor(diff / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);

  if (seconds < 60) return 'just now';
  if (minutes < 60) return `${minutes}m ago`;
  if (hours < 24) return `${hours}h ago`;
  if (days < 7) return `${days}d ago`;

  return then.toLocaleDateString();
}

// Format timestamp
export function formatTimestamp(date) {
  return new Date(date).toLocaleString();
}

// Calculate uptime percentage
export function calculateUptime(successCount, totalCount) {
  if (totalCount === 0) return 100;
  return ((successCount / totalCount) * 100).toFixed(2);
}

// Get status from metrics
export function getStatusFromMetrics(latency, packetLoss, threshold = {}) {
  const { maxLatency = 100, maxPacketLoss = 5 } = threshold;

  if (packetLoss >= 100) return 'down';
  if (packetLoss > maxPacketLoss || latency > maxLatency) return 'degraded';
  return 'healthy';
}

// Group targets by tag
export function groupByTag(targets, tagKey) {
  return targets.reduce((acc, target) => {
    const value = target.tags?.[tagKey] || 'untagged';
    if (!acc[value]) acc[value] = [];
    acc[value].push(target);
    return acc;
  }, {});
}

// Classnames utility
export function cn(...classes) {
  return classes.filter(Boolean).join(' ');
}

// Format seconds to human readable uptime
export function formatUptime(seconds) {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  if (seconds < 86400) {
    const hours = Math.floor(seconds / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    return mins > 0 ? `${hours}h ${mins}m` : `${hours}h`;
  }
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  return hours > 0 ? `${days}d ${hours}h` : `${days}d`;
}
