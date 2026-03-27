import { approveStage } from '../../api/pipeline';

interface Props {
  projectId: string;
  disabled: boolean;
  onApproved: () => void;
  warning?: string;
}

export function ApproveButton({ projectId, disabled, onApproved, warning }: Readonly<Props>) {
  const handleApprove = async () => {
    if (warning && !window.confirm(warning)) return;
    try {
      await approveStage(projectId);
      onApproved();
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to approve');
    }
  };

  return (
    <button
      className="approve-button"
      onClick={handleApprove}
      disabled={disabled}
    >
      Approve &amp; Continue
    </button>
  );
}
