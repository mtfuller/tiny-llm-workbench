import type { Model } from '../api'

// modelSourceLabel renders a registry model's provenance for display — the
// same mapping the Models list page uses, shared so the model picker matches
// it. "huggingface" → "Hugging Face"; anything with a baseModel is a model
// trained here; otherwise the raw source string.
export function modelSourceLabel(model: Model): string {
  if (model.source === 'huggingface') return 'Hugging Face'
  if (model.baseModel) return 'Trained'
  return model.source
}
