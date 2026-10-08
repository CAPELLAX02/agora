import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { App } from './App'

describe('App', () => {
  it('uygulama adını gösterir', () => {
    render(<App />)
    expect(screen.getByRole('heading', { name: 'Agora' })).toBeInTheDocument()
  })
})
