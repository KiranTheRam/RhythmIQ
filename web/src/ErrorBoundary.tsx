import { Component, type ErrorInfo, type ReactNode } from 'react'

interface Props {
  children: ReactNode
}

interface State {
  message: string | null
}

/**
 * Without this, any render-time error unmounts the whole tree and leaves the
 * viewer looking at a blank page with no way to recover.
 */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { message: null }

  static getDerivedStateFromError(error: unknown): State {
    return { message: error instanceof Error ? error.message : 'Unknown error' }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('RhythmIQ render error', error, info.componentStack)
  }

  render() {
    if (this.state.message === null) {
      return this.props.children
    }

    return (
      <div className="stage">
        <section className="gate">
          <h1 className="gate-title">This page stopped rendering</h1>
          <p className="gate-body">
            {this.state.message}. Reloading usually clears it; if it comes back, refresh your data.
          </p>
          <button className="button" onClick={() => window.location.reload()}>
            Reload
          </button>
        </section>
      </div>
    )
  }
}
