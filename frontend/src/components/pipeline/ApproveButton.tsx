import { approveStage } from '../../api/pipeline';

interface Props {
  projectId: string;
  disabled: boolean;
  onApproved: () => void;
}

export function ApproveButton({ projectId, disabled, onApproved }: Props) {
  const handleApprove = async () => {
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
