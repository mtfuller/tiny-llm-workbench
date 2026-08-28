import { ArrowUp, Folder, RefreshCw } from 'lucide-react'
import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { listDirectory, type DirectoryListing } from './api'
import Modal from './Modal'
import ModalActions from './ModalActions'

interface DirectoryPickerProps {
  // The directory to open initially; defaults to the user's home directory.
  initialPath?: string
  onSelect: (path: string) => void
  onClose: () => void
}

// DirectoryPicker browses directories on the host (via GET /api/fs/list) so a
// user can point a real workspace at a folder on their machine. Directories
// only — file contents are never listed.
function DirectoryPicker({ initialPath, onSelect, onClose }: DirectoryPickerProps) {
  const [listing, setListing] = useState<DirectoryListing | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const load = useCallback((path?: string) => {
    setLoading(true)
    setError(null)
    listDirectory(path)
      .then((l) => setListing(l))
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    load(initialPath)
  }, [load, initialPath])

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (listing) onSelect(listing.path)
  }

  return (
    <Modal title="Choose a folder" onClose={onClose} size="lg">
      <form className="stacked-form" onSubmit={handleSubmit}>
        <div className="dir-picker-bar">
          <button
            type="button"
            className="icon-button"
            title="Up one level"
            aria-label="Up one level"
            disabled={!listing?.parent || loading}
            onClick={() => listing?.parent && load(listing.parent)}
          >
            <ArrowUp size={15} />
          </button>
          <code className="dir-picker-path">{listing?.path ?? '…'}</code>
          <button
            type="button"
            className="icon-button"
            title="Refresh"
            aria-label="Refresh"
            disabled={loading}
            onClick={() => load(listing?.path)}
          >
            <RefreshCw size={15} />
          </button>
        </div>

        {error && <p className="error">{error}</p>}

        <div className="dir-picker-list panel panel-flush">
          {listing === null && <p className="empty-state">Loading…</p>}
          {listing !== null && listing.entries.length === 0 && (
            <p className="empty-state">No subfolders here.</p>
          )}
          {listing?.entries.map((e) => (
            <button key={e.path} type="button" className="dir-picker-row" onClick={() => load(e.path)}>
              <Folder size={15} />
              <span>{e.name}</span>
            </button>
          ))}
        </div>

        <ModalActions onCancel={onClose} submitLabel="Select this folder" disabled={!listing} />
      </form>
    </Modal>
  )
}

export default DirectoryPicker
