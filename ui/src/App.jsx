import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { ThemeProvider } from './context/ThemeContext';
import { Layout } from './components/Layout';
import { Dashboard, Agents, AgentDetail, Targets, TargetDetail, Incidents, Snapshots, Alerts, Settings, Fleet, MetricsExplorer, Subnets, SubnetDetail, ReviewQueue, LatencyMatrix, Infrastructure } from './pages';

function App() {
  return (
    <ThemeProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<Layout />}>
            <Route index element={<Dashboard />} />
            <Route path="agents" element={<Agents />} />
            <Route path="agents/:id" element={<AgentDetail />} />
            <Route path="infrastructure" element={<Infrastructure />} />
            <Route path="fleet" element={<Fleet />} />
            <Route path="targets" element={<Targets />} />
            <Route path="targets/:id" element={<TargetDetail />} />
            <Route path="subnets" element={<Subnets />} />
            <Route path="subnets/:id" element={<SubnetDetail />} />
            <Route path="review" element={<ReviewQueue />} />
            <Route path="metrics" element={<MetricsExplorer />} />
            <Route path="latency-matrix" element={<LatencyMatrix />} />
            <Route path="incidents" element={<Incidents />} />
            <Route path="snapshots" element={<Snapshots />} />
            <Route path="alerts" element={<Alerts />} />
            <Route path="settings" element={<Settings />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </ThemeProvider>
  );
}

export default App;
