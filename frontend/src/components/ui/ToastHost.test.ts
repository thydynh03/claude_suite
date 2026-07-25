import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import { get } from 'svelte/store'
import ToastHost from './ToastHost.svelte'
import { addToast, toasts } from '../../lib/stores/appState'

beforeEach(() => {
  toasts.set([])
})

describe('ToastHost', () => {
  it('shows nothing until a toast arrives', () => {
    render(ToastHost)
    expect(screen.queryByText('task finished')).not.toBeInTheDocument()
  })

  it('renders a toast pushed onto the store', async () => {
    render(ToastHost)

    vi.useFakeTimers()
    try {
      addToast('task finished', 'SUCCESS')
    } finally {
      vi.useRealTimers()
    }

    expect(await screen.findByText('task finished')).toBeInTheDocument()
  })

  it('removes the toast when its close button is used', async () => {
    const user = userEvent.setup()
    render(ToastHost)

    vi.useFakeTimers()
    try {
      addToast('dismiss me')
    } finally {
      vi.useRealTimers()
    }

    expect(await screen.findByText('dismiss me')).toBeInTheDocument()

    await user.click(screen.getByRole('button'))

    expect(get(toasts)).toHaveLength(0)
  })

  it('shows several toasts at once', async () => {
    render(ToastHost)

    vi.useFakeTimers()
    try {
      addToast('first')
      addToast('second')
    } finally {
      vi.useRealTimers()
    }

    expect(await screen.findByText('first')).toBeInTheDocument()
    expect(screen.getByText('second')).toBeInTheDocument()
  })
})
