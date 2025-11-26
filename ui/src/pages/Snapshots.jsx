import { useState } from 'react';
import {
  Camera,
  Plus,
  Clock,
  GitCompare,
  ChevronRight,
  CheckCircle,
  XCircle,
  AlertTriangle,
} from 'lucide-react';

import { PageHeader, PageContent } from '../components/Layout';
import { Card, CardTitle, CardContent } from '../components/Card';
import { Button } from '../components/Button';
import { StatusBadge } from '../components/StatusBadge';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '../components/Table';
import { formatRelativeTime } from '../lib/utils';

// Demo data
const mockSnapshots = [
  {
    id: 'snap-001',
    name: 'Pre-maintenance Chicago POP',
    description: 'Baseline before router upgrade CHI-01',
    createdAt: new Date(Date.now() - 3600000 * 2),
    createdBy: 'jsmith',
    targetCount: 1024,
    status: 'completed',
    results: { healthy: 1020, degraded: 3, down: 1 },
  },
  {
    id: 'snap-002',
    name: 'Post-maintenance Chicago POP',
    description: 'After router upgrade CHI-01',
    createdAt: new Date(Date.now() - 3600000),
    createdBy: 'jsmith',
    targetCount: 1024,
    status: 'completed',
    results: { healthy: 1022, degraded: 2, down: 0 },
    comparedTo: 'snap-001',
    comparison: { improved: 2, regressed: 0, unchanged: 1022 },
  },
  {
    id: 'snap-003',
    name: 'Daily baseline - All targets',
    description: 'Automated daily snapshot',
    createdAt: new Date(Date.now() - 3600000 * 24),
    createdBy: 'system',
    targetCount: 102847,
    status: 'completed',
    results: { healthy: 101923, degraded: 712, down: 212 },
  },
  {
    id: 'snap-004',
    name: 'NYC fiber cut investigation',
    description: 'Snapshot during reported outage',
    createdAt: new Date(Date.now() - 3600000 * 48),
    createdBy: 'mwilson',
    targetCount: 5234,
    status: 'completed',
    results: { healthy: 4892, degraded: 234, down: 108 },
  },
];

export function Snapshots() {
  const [snapshots] = useState(mockSnapshots);

  return (
    <>
      <PageHeader
        title="Snapshots"
        description="Point-in-time network state captures for maintenance and troubleshooting"
        actions={
          <Button className="gap-2">
            <Camera className="w-4 h-4" />
            Create Snapshot
          </Button>
        }
      />

      <PageContent>
        {/* Info Card */}
        <Card className="mb-6" accent="cyan">
          <div className="flex items-start gap-4">
            <div className="p-3 bg-pilot-cyan/20 rounded-lg">
              <Camera className="w-6 h-6 text-pilot-cyan" />
            </div>
            <div>
              <h3 className="font-medium text-theme-primary mb-1">Maintenance Snapshots</h3>
              <p className="text-sm text-theme-muted">
                Create snapshots before and after maintenance windows to compare network state.
                Snapshots capture the current probe results for all or selected targets,
                allowing you to detect regressions and verify improvements.
              </p>
            </div>
          </div>
        </Card>

        {/* Snapshots List */}
        <Card>
          <CardTitle>Recent Snapshots</CardTitle>
          <CardContent className="mt-4">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Snapshot</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="text-right">Targets</TableHead>
                  <TableHead>Results</TableHead>
                  <TableHead>Comparison</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {snapshots.map((snapshot) => (
                  <TableRow key={snapshot.id} className="cursor-pointer">
                    <TableCell>
                      <div>
                        <div className="font-medium text-theme-primary">{snapshot.name}</div>
                        <div className="text-xs text-theme-muted">{snapshot.description}</div>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2 text-sm">
                        <Clock className="w-4 h-4 text-theme-muted" />
                        <div>
                          <div className="text-theme-primary">{formatRelativeTime(snapshot.createdAt)}</div>
                          <div className="text-xs text-theme-muted">by {snapshot.createdBy}</div>
                        </div>
                      </div>
                    </TableCell>
                    <TableCell className="text-right font-mono">
                      {snapshot.targetCount.toLocaleString()}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-3 text-sm">
                        <span className="flex items-center gap-1 text-status-healthy">
                          <CheckCircle className="w-3 h-3" />
                          {snapshot.results.healthy.toLocaleString()}
                        </span>
                        {snapshot.results.degraded > 0 && (
                          <span className="flex items-center gap-1 text-warning">
                            <AlertTriangle className="w-3 h-3" />
                            {snapshot.results.degraded}
                          </span>
                        )}
                        {snapshot.results.down > 0 && (
                          <span className="flex items-center gap-1 text-status-down">
                            <XCircle className="w-3 h-3" />
                            {snapshot.results.down}
                          </span>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      {snapshot.comparison ? (
                        <div className="flex items-center gap-2 text-sm">
                          <GitCompare className="w-4 h-4 text-pilot-cyan" />
                          <span className="text-status-healthy">+{snapshot.comparison.improved}</span>
                          <span className="text-status-down">-{snapshot.comparison.regressed}</span>
                        </div>
                      ) : (
                        <Button variant="ghost" size="sm" className="gap-1">
                          <GitCompare className="w-3 h-3" />
                          Compare
                        </Button>
                      )}
                    </TableCell>
                    <TableCell>
                      <ChevronRight className="w-4 h-4 text-theme-muted" />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </PageContent>
    </>
  );
}
