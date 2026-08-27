import Modal from './Modal'
import TokenProbabilities from './TokenProbabilities'

interface TokenProbabilitiesModalProps {
  modelName: string
  onClose: () => void
}

// TokenProbabilitiesModal is a thin Modal wrapper around TokenProbabilities
// — kept as a separate component (rather than folding a "size" prop into
// TokenProbabilities itself) so the analysis tool has no idea whether it's
// being shown in a modal or inline; today it's a modal, matching the chat
// tool's presentation and freeing the Models detail page's card grid from
// having to reserve room for an open-ended list of generated tokens.
function TokenProbabilitiesModal({ modelName, onClose }: TokenProbabilitiesModalProps) {
  return (
    <Modal title={`Token probabilities — ${modelName}`} onClose={onClose} size="lg">
      <TokenProbabilities modelName={modelName} />
    </Modal>
  )
}

export default TokenProbabilitiesModal
