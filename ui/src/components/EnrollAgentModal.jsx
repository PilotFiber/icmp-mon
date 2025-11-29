import { useState, useCallback, useRef, useEffect } from 'react';
import {
  Server,
  Check,
  X,
  Loader2,
  AlertCircle,
  CheckCircle2,
  Circle,
  RefreshCw,
  Terminal,
  Play,
} from 'lucide-react';

import { Modal, ModalFooter } from './Modal';
import { Button } from './Button';
import { endpoints } from '../lib/api';

const ENROLLMENT_STEPS = [
  { id: 'connecting', label: 'Connecting via SSH' },
  { id: 'detecting', label: 'Detecting system' },
  { id: 'key_installing', label: 'Installing SSH key' },
  { id: 'hardening', label: 'Hardening SSH' },
  { id: 'tailscale', label: 'Installing Tailscale' },
  { id: 'dependencies', label: 'Installing dependencies' },
  { id: 'agent_installing', label: 'Installing agent' },
  { id: 'starting', label: 'Starting agent' },
  { id: 'registering', label: 'Verifying registration' },
];

function StepIndicator({ step, status }) {
  const getIcon = () => {
    switch (status) {
      case 'completed':
        return <CheckCircle2 className="w-5 h-5 text-pilot-green" />;
      case 'in_progress':
        return <Loader2 className="w-5 h-5 text-pilot-cyan animate-spin" />;
      case 'failed':
        return <AlertCircle className="w-5 h-5 text-pilot-red" />;
      default:
        return <Circle className="w-5 h-5 text-gray-600" />;
    }
  };

  return (
    <div className={`flex items-center gap-3 py-2 ${status === 'pending' ? 'opacity-50' : ''}`}>
      {getIcon()}
      <span className={`text-sm ${status === 'in_progress' ? 'text-theme-primary' : status === 'completed' ? 'text-theme-secondary' : 'text-theme-muted'}`}>
        {step.label}
      </span>
    </div>
  );
}

function ProgressBar({ progress }) {
  return (
    <div className="w-full h-2 bg-surface-tertiary rounded-full overflow-hidden">
      <div
        className="h-full bg-gradient-to-r from-pilot-cyan to-pilot-green transition-all duration-500 ease-out"
        style={{ width: `${progress}%` }}
      />
    </div>
  );
}

function LogOutput({ logs }) {
  const containerRef = useRef(null);

  useEffect(() => {
    if (containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [logs]);

  if (logs.length === 0) return null;

  return (
    <div className="mt-4">
      <div className="flex items-center gap-2 text-sm text-theme-muted mb-2">
        <Terminal className="w-4 h-4" />
        <span>Output</span>
      </div>
      <div
        ref={containerRef}
        className="bg-black/40 rounded-lg p-3 h-32 overflow-y-auto font-mono text-xs"
      >
        {logs.map((log, i) => (
          <div key={i} className={`${log.type === 'error' ? 'text-pilot-red' : 'text-theme-muted'}`}>
            {log.message}
          </div>
        ))}
      </div>
    </div>
  );
}

// Strip CIDR notation from IP address (e.g., "192.168.1.1/32" -> "192.168.1.1")
function stripCIDR(ip) {
  if (!ip) return '';
  return ip.split('/')[0];
}

export function EnrollAgentModal({ isOpen, onClose, onSuccess, reEnrollAgent = null }) {
  // Determine if this is a re-enrollment
  const isReEnroll = !!reEnrollAgent;

  // Form state - pre-fill from reEnrollAgent if provided
  const [formData, setFormData] = useState({
    target_ip: stripCIDR(reEnrollAgent?.public_ip) || stripCIDR(reEnrollAgent?.tailscale_ip) || '',
    target_port: 22,
    username: 'root',
    password: '',
    agent_name: reEnrollAgent?.name || '',
    region: reEnrollAgent?.region || '',
    location: reEnrollAgent?.location || '',
    provider: reEnrollAgent?.provider || '',
  });

  // Reset form when reEnrollAgent changes
  useEffect(() => {
    if (reEnrollAgent) {
      setFormData({
        target_ip: stripCIDR(reEnrollAgent.public_ip) || stripCIDR(reEnrollAgent.tailscale_ip) || '',
        target_port: 22,
        username: 'root',
        password: '',
        agent_name: reEnrollAgent.name || '',
        region: reEnrollAgent.region || '',
        location: reEnrollAgent.location || '',
        provider: reEnrollAgent.provider || '',
      });
    } else {
      setFormData({
        target_ip: '',
        target_port: 22,
        username: 'root',
        password: '',
        agent_name: '',
        region: '',
        location: '',
        provider: '',
      });
    }
  }, [reEnrollAgent]);

  // Enrollment state
  const [phase, setPhase] = useState('form'); // form, enrolling, success, failed
  const [progress, setProgress] = useState(0);
  const [currentStep, setCurrentStep] = useState(null);
  const [completedSteps, setCompletedSteps] = useState([]);
  const [logs, setLogs] = useState([]);
  const [error, setError] = useState(null);
  const [enrollmentId, setEnrollmentId] = useState(null);
  const [agentId, setAgentId] = useState(null);
  const [canResume, setCanResume] = useState(false); // Can resume without password (SSH key installed)

  const abortControllerRef = useRef(null);

  const resetState = useCallback(() => {
    setPhase('form');
    setProgress(0);
    setCurrentStep(null);
    setCompletedSteps([]);
    setLogs([]);
    setError(null);
    setEnrollmentId(null);
    setAgentId(null);
    setCanResume(false);
  }, []);

  const handleClose = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    resetState();
    onClose();
  }, [onClose, resetState]);

  const handleInputChange = (field) => (e) => {
    setFormData((prev) => ({
      ...prev,
      [field]: field === 'target_port' ? parseInt(e.target.value) || 22 : e.target.value,
    }));
  };

  const handleEnroll = async () => {
    setPhase('enrolling');
    setError(null);
    setLogs([]);

    abortControllerRef.current = new AbortController();

    try {
      // Always try SSH key first - handles retries where key is installed but server is hardened
      const enrollData = { ...formData, try_key_first: true };
      const response = await endpoints.enrollAgent(enrollData);

      if (!response.ok) {
        const errorData = await response.json();
        throw new Error(errorData.error || 'Enrollment failed');
      }

      // Handle SSE stream
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();

        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        let eventType = '';
        let eventData = '';

        for (const line of lines) {
          if (line.startsWith('event:')) {
            eventType = line.slice(6).trim();
          } else if (line.startsWith('data:')) {
            eventData = line.slice(5).trim();
          } else if (line === '' && eventData) {
            // End of event
            try {
              const data = JSON.parse(eventData);
              handleEvent(eventType, data);
            } catch (e) {
              console.error('Failed to parse event:', e);
            }
            eventType = '';
            eventData = '';
          }
        }
      }
    } catch (err) {
      if (err.name === 'AbortError') return;

      setError(err.message);
      setPhase('failed');
      setLogs((prev) => [...prev, { type: 'error', message: err.message }]);
    }
  };

  const handleEvent = (type, data) => {
    switch (type) {
      case 'step':
        setCurrentStep(data.step);
        setProgress(data.progress || 0);
        if (data.enrollment_id) {
          setEnrollmentId(data.enrollment_id);
        }
        setLogs((prev) => [...prev, { type: 'info', message: data.message }]);
        break;

      case 'log':
        if (data.step && !completedSteps.includes(data.step)) {
          setCompletedSteps((prev) => [...prev, data.step]);
        }
        setLogs((prev) => [...prev, { type: 'info', message: data.message }]);
        break;

      case 'complete':
        setPhase('success');
        setProgress(100);
        setCurrentStep(null);
        setCompletedSteps(ENROLLMENT_STEPS.map((s) => s.id));
        if (data.details?.agent_id) {
          setAgentId(data.details.agent_id);
        }
        setLogs((prev) => [...prev, { type: 'success', message: data.message }]);
        break;

      case 'error':
        setPhase('failed');
        setError(data.message);
        // Check if SSH key was installed - if so, we can resume without password
        setCanResume(completedSteps.includes('key_installing'));
        setLogs((prev) => [...prev, { type: 'error', message: data.message }]);
        break;

      default:
        console.log('Unknown event:', type, data);
    }
  };

  const getStepStatus = (stepId) => {
    if (completedSteps.includes(stepId)) return 'completed';
    if (currentStep === stepId) return 'in_progress';
    if (phase === 'failed' && currentStep === stepId) return 'failed';
    return 'pending';
  };

  const handleRetry = () => {
    setCompletedSteps([]);
    setCurrentStep(null);
    setProgress(0);
    setError(null);
    setLogs([]);
    setCanResume(false);
    handleEnroll();
  };

  // Resume enrollment using SSH key (no password required)
  const handleResume = async () => {
    if (!enrollmentId) {
      setError('No enrollment ID available for resume');
      return;
    }

    setPhase('enrolling');
    setError(null);
    setLogs((prev) => [...prev, { type: 'info', message: 'Resuming enrollment via SSH key...' }]);

    abortControllerRef.current = new AbortController();

    try {
      const response = await endpoints.resumeEnrollment(enrollmentId);

      if (!response.ok) {
        const errorData = await response.json();
        throw new Error(errorData.error || 'Resume failed');
      }

      // Handle SSE stream (same as enrollment)
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();

        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        let eventType = '';
        let eventData = '';

        for (const line of lines) {
          if (line.startsWith('event:')) {
            eventType = line.slice(6).trim();
          } else if (line.startsWith('data:')) {
            eventData = line.slice(5).trim();
          } else if (line === '' && eventData) {
            try {
              const data = JSON.parse(eventData);
              handleEvent(eventType, data);
            } catch (e) {
              console.error('Failed to parse event:', e);
            }
            eventType = '';
            eventData = '';
          }
        }
      }
    } catch (err) {
      if (err.name === 'AbortError') return;

      setError(err.message);
      setPhase('failed');
      setLogs((prev) => [...prev, { type: 'error', message: err.message }]);
    }
  };

  const handleSuccessClose = () => {
    if (onSuccess) onSuccess(agentId);
    handleClose();
  };

  // Form phase
  if (phase === 'form') {
    return (
      <Modal isOpen={isOpen} onClose={handleClose} title={isReEnroll ? `Re-enroll Agent: ${reEnrollAgent?.name}` : "Enroll New Agent"} size="md">
        <div className="space-y-4">
          <p className="text-sm text-theme-muted mb-4">
            {isReEnroll
              ? "Re-enroll this agent to reinstall the agent software and refresh its configuration. SSH password is required."
              : "Enter the SSH credentials for the server. The password is used once and never stored."
            }
          </p>

          <div className="grid grid-cols-3 gap-4">
            <div className="col-span-2">
              <label className="block text-sm font-medium text-theme-secondary mb-1">
                Server IP / Hostname *
              </label>
              <input
                type="text"
                value={formData.target_ip}
                onChange={handleInputChange('target_ip')}
                placeholder="192.168.1.100"
                className="w-full px-3 py-2 bg-surface-tertiary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan focus:border-transparent"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-theme-secondary mb-1">Port</label>
              <input
                type="number"
                value={formData.target_port}
                onChange={handleInputChange('target_port')}
                className="w-full px-3 py-2 bg-surface-tertiary border border-theme rounded-lg text-theme-primary focus:outline-none focus:ring-2 focus:ring-pilot-cyan focus:border-transparent"
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-theme-secondary mb-1">
                SSH Username *
              </label>
              <input
                type="text"
                value={formData.username}
                onChange={handleInputChange('username')}
                placeholder="root"
                className="w-full px-3 py-2 bg-surface-tertiary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan focus:border-transparent"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-theme-secondary mb-1">Password *</label>
              <input
                type="password"
                value={formData.password}
                onChange={handleInputChange('password')}
                placeholder="SSH password"
                className="w-full px-3 py-2 bg-surface-tertiary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan focus:border-transparent"
              />
            </div>
          </div>

          <div className="pt-2 border-t border-theme">
            <p className="text-xs text-theme-muted mb-3">Optional configuration</p>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-theme-secondary mb-1">Agent Name</label>
                <input
                  type="text"
                  value={formData.agent_name}
                  onChange={handleInputChange('agent_name')}
                  placeholder="Auto-generated if empty"
                  className="w-full px-3 py-2 bg-surface-tertiary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan focus:border-transparent"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-theme-secondary mb-1">Region</label>
                <input
                  type="text"
                  value={formData.region}
                  onChange={handleInputChange('region')}
                  placeholder="us-east-1"
                  className="w-full px-3 py-2 bg-surface-tertiary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan focus:border-transparent"
                />
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4 mt-3">
              <div>
                <label className="block text-sm font-medium text-theme-secondary mb-1">Location</label>
                <input
                  type="text"
                  value={formData.location}
                  onChange={handleInputChange('location')}
                  placeholder="New York, NY"
                  className="w-full px-3 py-2 bg-surface-tertiary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan focus:border-transparent"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-theme-secondary mb-1">Provider</label>
                <input
                  type="text"
                  value={formData.provider}
                  onChange={handleInputChange('provider')}
                  placeholder="AWS, GCP, etc."
                  className="w-full px-3 py-2 bg-surface-tertiary border border-theme rounded-lg text-theme-primary placeholder-theme-muted focus:outline-none focus:ring-2 focus:ring-pilot-cyan focus:border-transparent"
                />
              </div>
            </div>
          </div>
        </div>

        <ModalFooter>
          <Button variant="ghost" onClick={handleClose}>
            Cancel
          </Button>
          <Button
            onClick={handleEnroll}
            disabled={!formData.target_ip || !formData.username || !formData.password}
            className="gap-2"
          >
            <Server className="w-4 h-4" />
            {isReEnroll ? 'Re-enroll Agent' : 'Enroll Agent'}
          </Button>
        </ModalFooter>
      </Modal>
    );
  }

  // Enrolling / Success / Failed phases
  return (
    <Modal
      isOpen={isOpen}
      onClose={phase === 'enrolling' ? undefined : handleClose}
      title={
        phase === 'success'
          ? 'Agent Enrolled Successfully'
          : phase === 'failed'
          ? 'Enrollment Failed'
          : 'Enrolling Agent'
      }
      size="md"
    >
      <div className="space-y-6">
        {/* Progress Bar */}
        <div>
          <div className="flex justify-between text-sm mb-2">
            <span className="text-theme-muted">Progress</span>
            <span className="text-theme-primary font-medium">{progress}%</span>
          </div>
          <ProgressBar progress={progress} />
        </div>

        {/* Server Info */}
        <div className="flex items-center gap-3 p-3 bg-surface-tertiary rounded-lg">
          <Server className="w-5 h-5 text-pilot-cyan" />
          <div>
            <div className="text-theme-primary font-medium">{formData.target_ip}</div>
            <div className="text-xs text-theme-muted">
              {formData.username}@{formData.target_ip}:{formData.target_port}
            </div>
          </div>
        </div>

        {/* Steps */}
        <div className="space-y-1">
          {ENROLLMENT_STEPS.map((step) => (
            <StepIndicator key={step.id} step={step} status={getStepStatus(step.id)} />
          ))}
        </div>

        {/* Error Display */}
        {error && (
          <div className="p-3 bg-pilot-red/10 border border-pilot-red/30 rounded-lg">
            <div className="flex items-start gap-2">
              <AlertCircle className="w-5 h-5 text-pilot-red flex-shrink-0 mt-0.5" />
              <div>
                <div className="text-pilot-red font-medium">Error</div>
                <div className="text-sm text-theme-secondary mt-1">{error}</div>
              </div>
            </div>
          </div>
        )}

        {/* Success Message */}
        {phase === 'success' && (
          <div className="p-3 bg-pilot-green/10 border border-pilot-green/30 rounded-lg">
            <div className="flex items-start gap-2">
              <CheckCircle2 className="w-5 h-5 text-pilot-green flex-shrink-0 mt-0.5" />
              <div>
                <div className="text-pilot-green font-medium">Agent enrolled successfully!</div>
                <div className="text-sm text-theme-secondary mt-1">
                  The agent is now running and reporting to the control plane.
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Log Output */}
        <LogOutput logs={logs} />
      </div>

      <ModalFooter>
        {phase === 'enrolling' && (
          <Button variant="ghost" onClick={handleClose}>
            Cancel
          </Button>
        )}
        {phase === 'failed' && (
          <>
            <Button variant="ghost" onClick={handleClose}>
              Close
            </Button>
            {canResume ? (
              <Button onClick={handleResume} className="gap-2">
                <Play className="w-4 h-4" />
                Resume
              </Button>
            ) : (
              <Button onClick={handleRetry} className="gap-2">
                <RefreshCw className="w-4 h-4" />
                Retry
              </Button>
            )}
          </>
        )}
        {phase === 'success' && (
          <Button onClick={handleSuccessClose} className="gap-2">
            <Check className="w-4 h-4" />
            Done
          </Button>
        )}
      </ModalFooter>
    </Modal>
  );
}
