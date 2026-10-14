import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { RichTextEditor } from './rich-text-editor'

describe('zengin metin editörü', () => {
  it('editör oluşunca araç çubuğu ve içerik görünür', async () => {
    render(<RichTextEditor value="<p>Merhaba <strong>dünya</strong></p>" onChange={() => {}} label="Özet" />)
    const toolbar = await screen.findByRole('toolbar', { name: 'Biçimlendirme' })
    expect(toolbar).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Kalın' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Geri al' })).toBeDisabled()
    expect(screen.getByRole('textbox', { name: 'Özet' })).toHaveTextContent('Merhaba dünya')
  })
})
