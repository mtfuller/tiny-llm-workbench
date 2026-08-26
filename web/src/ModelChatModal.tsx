import { Bot, Loader2, Send, User } from 'lucide-react'
import { useEffect, useRef, useState, type FormEvent } from 'react'
import { chatWithModel, type ChatTurn } from './api'
import Modal from './Modal'

interface ModelChatModalProps {
  modelName: string
  onClose: () => void
}

// ModelChatModal is a small multi-turn chat window for trying out a
// registry model directly, without setting up a full Agent. Each send
// resends the whole conversation so far — mlx_lm.server itself keeps no
// memory between requests, so TLW has to.
function ModelChatModal({ modelName, onClose }: ModelChatModalProps) {
  const [messages, setMessages] = useState<ChatTurn[]>([])
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const logEndRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, sending])

  const sendMessage = async () => {
    if (!input.trim() || sending) return

    const next = [...messages, { role: 'user', content: input.trim() } satisfies ChatTurn]
    setMessages(next)
    setInput('')
    setSending(true)
    setError(null)
    try {
      const res = await chatWithModel(modelName, next)
      setMessages([...next, { role: 'assistant', content: res.completion }])
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSending(false)
    }
  }

  return (
    <Modal title={`Chat with ${modelName}`} onClose={onClose} size="lg">
      <div className="chat-modal-body">
        <div className="chat-modal-log">
          {messages.length === 0 && <p className="chat-modal-empty">Say hello to try this model.</p>}
          {messages.map((m, i) => (
            <div className={`chat-modal-message chat-modal-message-${m.role}`} key={i}>
              <div className="chat-modal-avatar">{m.role === 'user' ? <User size={16} /> : <Bot size={16} />}</div>
              <div className="chat-modal-bubble">{m.content}</div>
            </div>
          ))}
          {sending && (
            <div className="chat-modal-message chat-modal-message-assistant">
              <div className="chat-modal-avatar">
                <Bot size={16} />
              </div>
              <div className="chat-modal-bubble chat-modal-bubble-pending">
                <Loader2 size={15} className="chat-modal-spinner" />
              </div>
            </div>
          )}
          <div ref={logEndRef} />
        </div>

        {error && <p className="error">{error}</p>}

        <form
          className="chat-modal-form"
          onSubmit={(e: FormEvent) => {
            e.preventDefault()
            sendMessage()
          }}
        >
          <textarea
            className="chat-modal-input"
            rows={1}
            placeholder="Message the model…"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                sendMessage()
              }
            }}
            disabled={sending}
            autoFocus
          />
          <button type="submit" className="chat-modal-send" disabled={sending || !input.trim()} aria-label="Send">
            <Send size={16} />
          </button>
        </form>
      </div>
    </Modal>
  )
}

export default ModelChatModal
