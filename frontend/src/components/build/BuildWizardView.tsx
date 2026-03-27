import type { ReactNode } from 'react';

const BUILD_STEPS = [
  { id: 'chat',     label: 'Chat' },
  { id: 'artifact', label: 'Build Plan' },
  { id: 'skills',   label: 'Skills' },
  { id: 'generate', label: 'Generate Beads' },
  { id: 'execute',  label: 'Implement' },
];

interface Props {
  activeStep: string;
  onStepChange: (id: string) => void;
  canAdvance: Record<string, boolean>;
  children: ReactNode;
}

export function BuildWizardView({ activeStep, onStepChange, canAdvance, children }: Props) {
  const stepIndex = BUILD_STEPS.findIndex(s => s.id === activeStep);

  const handlePrev = () => {
    if (stepIndex > 0) onStepChange(BUILD_STEPS[stepIndex - 1].id);
  };

  const handleNext = () => {
    if (stepIndex < BUILD_STEPS.length - 1) onStepChange(BUILD_STEPS[stepIndex + 1].id);
  };

  const canGoNext = canAdvance[activeStep] !== false && stepIndex < BUILD_STEPS.length - 1;
  const canGoPrev = stepIndex > 0;

  return (
    <div className="build-wizard">
      <div className="wizard-steps">
        {BUILD_STEPS.map((step, i) => {
          const isCompleted = i < stepIndex;
          const isActive = i === stepIndex;
          return (
            <div key={step.id} className="wizard-step-item">
              <div className={`wizard-step-dot${isActive ? ' wizard-step-active' : isCompleted ? ' wizard-step-completed' : ''}`}>
                {isCompleted ? '✓' : i + 1}
              </div>
              <span className={`wizard-step-label${isActive ? ' wizard-step-label-active' : ''}`}>{step.label}</span>
              {i < BUILD_STEPS.length - 1 && (
                <div className={`wizard-step-connector${isCompleted ? ' wizard-connector-done' : ''}`} />
              )}
            </div>
          );
        })}
      </div>

      <div className="wizard-content">{children}</div>

      <div className="wizard-nav">
        <button
          className="wizard-nav-btn"
          onClick={handlePrev}
          disabled={!canGoPrev}
        >
          ← Previous
        </button>
        <button
          className="wizard-nav-btn wizard-nav-next"
          onClick={handleNext}
          disabled={!canGoNext}
          title={!canAdvance[activeStep] && stepIndex < BUILD_STEPS.length - 1
            ? activeStep === 'chat'
              ? 'Generate a build plan first'
              : activeStep === 'generate'
                ? 'Generate beads first'
                : undefined
            : undefined}
        >
          Next →
        </button>
      </div>
    </div>
  );
}
